/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconcilers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	kernelcacheconfig "github.com/kserve/kserve/pkg/kernelcache/config"
	"github.com/kserve/kserve/pkg/kernelcache/nodegroup"
)

const (
	kernelCacheNodeAgentDaemonSetName   = "kserve-kernelcachenode-agent"
	kernelCacheNodeAgentDisabledKey     = "serving.kserve.io/kernelcache-agent"
	kernelCacheNodeAgentDisabledValue   = "disabled"
	kernelCacheNodeReconcileRequestName = "kernelcache-node-reconcile"
)

// KernelCacheNodeReconciler creates KernelCacheNode resources for nodes selected by KernelCacheNodeGroup.
type KernelCacheNodeReconciler struct {
	client.Client
	Reader client.Reader
	Log    logr.Logger
}

func (r *KernelCacheNodeReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	config, err := kernelcacheconfig.Load(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("load KernelCache configuration: %w", err)
	}
	if !config.Enabled {
		return ctrl.Result{}, nil
	}

	groups, err := r.listAndValidateNodeGroups(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileAgentDaemonSet(ctx, groups); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileKernelCacheNodes(ctx, groups); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KernelCacheNodeReconciler) listAndValidateNodeGroups(ctx context.Context) ([]v1alpha1.KernelCacheNodeGroup, error) {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}

	groups := &v1alpha1.KernelCacheNodeGroupList{}
	if err := reader.List(ctx, groups); err != nil {
		return nil, err
	}
	if err := validateKernelCacheNodeGroups(groups.Items); err != nil {
		return nil, err
	}
	return groups.Items, nil
}

func (r *KernelCacheNodeReconciler) reconcileKernelCacheNodes(ctx context.Context, groups []v1alpha1.KernelCacheNodeGroup) error {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}

	nodes := &corev1.NodeList{}
	if err := reader.List(ctx, nodes); err != nil {
		return err
	}
	kernelCacheNodes := &v1alpha1.KernelCacheNodeList{}
	if err := reader.List(ctx, kernelCacheNodes); err != nil {
		return err
	}

	activeNodes := make(map[string]struct{})
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if len(nodegroup.MatchingGroups(node, groups)) == 0 {
			continue
		}
		activeNodes[node.Name] = struct{}{}
	}

	existingNodes := make(map[string]struct{}, len(kernelCacheNodes.Items))
	for i := range kernelCacheNodes.Items {
		existingNodes[kernelCacheNodes.Items[i].Name] = struct{}{}
	}
	for nodeName := range activeNodes {
		if _, exists := existingNodes[nodeName]; exists {
			continue
		}
		kernelCacheNode := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
		if err := r.Create(ctx, kernelCacheNode); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	for i := range kernelCacheNodes.Items {
		kernelCacheNode := &kernelCacheNodes.Items[i]
		if _, exists := activeNodes[kernelCacheNode.Name]; exists {
			continue
		}
		if err := r.Delete(ctx, kernelCacheNode); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func validateKernelCacheNodeGroups(groups []v1alpha1.KernelCacheNodeGroup) error {
	for i := range groups {
		if len(groups[i].Spec.NodeSelector) == 0 {
			return fmt.Errorf("KernelCacheNodeGroup %q requires a non-empty nodeSelector", groups[i].Name)
		}
	}
	return nil
}

func (r *KernelCacheNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KernelCacheNodeGroup{}).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.nodeToNodeGroupRequests)).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.nodeToNodeGroupRequests), builder.WithPredicates(r.inferenceServiceConfigMapPredicate())).
		Complete(r)
}

func (r *KernelCacheNodeReconciler) inferenceServiceConfigMapPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetNamespace() == constants.KServeNamespace && obj.GetName() == constants.InferenceServiceConfigMapName
	})
}

func (r *KernelCacheNodeReconciler) nodeToNodeGroupRequests(context.Context, client.Object) []reconcile.Request {
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{Name: kernelCacheNodeReconcileRequestName},
	}}
}

func (r *KernelCacheNodeReconciler) reconcileAgentDaemonSet(ctx context.Context, groups []v1alpha1.KernelCacheNodeGroup) error {
	daemonSet := &appsv1.DaemonSet{}
	key := client.ObjectKey{Namespace: constants.KServeNamespace, Name: kernelCacheNodeAgentDaemonSetName}
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	if err := reader.Get(ctx, key, daemonSet); err != nil {
		return fmt.Errorf("get KernelCacheNode agent DaemonSet: %w", err)
	}

	original := daemonSet.DeepCopy()
	daemonSet.Spec.Template.Spec.NodeSelector = nil
	daemonSet.Spec.Template.Spec.Affinity = agentAffinity(groups, daemonSet.Spec.Template.Spec.Affinity)
	daemonSet.Spec.Template.Spec.Tolerations = agentTolerations(groups)

	if original.Spec.Template.Spec.NodeSelector == nil &&
		reflect.DeepEqual(original.Spec.Template.Spec.Affinity, daemonSet.Spec.Template.Spec.Affinity) &&
		reflect.DeepEqual(original.Spec.Template.Spec.Tolerations, daemonSet.Spec.Template.Spec.Tolerations) {
		return nil
	}

	return r.Patch(ctx, daemonSet, client.MergeFrom(original))
}

func agentAffinity(groups []v1alpha1.KernelCacheNodeGroup, existing *corev1.Affinity) *corev1.Affinity {
	affinity := existing.DeepCopy()
	if affinity == nil {
		affinity = &corev1.Affinity{}
	}
	if affinity.NodeAffinity == nil {
		affinity.NodeAffinity = &corev1.NodeAffinity{}
	}

	orderedGroups := append([]v1alpha1.KernelCacheNodeGroup(nil), groups...)
	sort.Slice(orderedGroups, func(i, j int) bool {
		return orderedGroups[i].Name < orderedGroups[j].Name
	})

	terms := make([]corev1.NodeSelectorTerm, 0, len(orderedGroups))
	for i := range orderedGroups {
		keys := make([]string, 0, len(orderedGroups[i].Spec.NodeSelector))
		for key := range orderedGroups[i].Spec.NodeSelector {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		expressions := make([]corev1.NodeSelectorRequirement, 0, len(keys))
		for _, key := range keys {
			expressions = append(expressions, corev1.NodeSelectorRequirement{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{orderedGroups[i].Spec.NodeSelector[key]},
			})
		}
		terms = append(terms, corev1.NodeSelectorTerm{MatchExpressions: expressions})
	}

	if len(terms) == 0 {
		terms = []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      kernelCacheNodeAgentDisabledKey,
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{kernelCacheNodeAgentDisabledValue},
			}},
		}}
	}

	affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{NodeSelectorTerms: terms}
	return affinity
}

func agentTolerations(groups []v1alpha1.KernelCacheNodeGroup) []corev1.Toleration {
	unique := make(map[string]corev1.Toleration)
	for i := range groups {
		for _, toleration := range groups[i].Spec.Tolerations {
			encoded, err := json.Marshal(toleration)
			if err != nil {
				continue
			}
			unique[string(encoded)] = toleration
		}
	}

	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	tolerations := make([]corev1.Toleration, 0, len(keys))
	for _, key := range keys {
		tolerations = append(tolerations, unique[key])
	}
	if len(tolerations) == 0 {
		return nil
	}
	return tolerations
}
