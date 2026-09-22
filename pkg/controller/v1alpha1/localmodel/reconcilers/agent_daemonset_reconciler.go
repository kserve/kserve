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
	"reflect"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	// AgentDaemonSetName is the name of the localmodelnode agent DaemonSet deployed alongside the controller.
	AgentDaemonSetName = "kserve-localmodelnode-agent"
	// ManagedTolerationsAnnotation records the tolerations this controller last copied from the
	// LocalModelNodeGroup CRs, so tolerations set at install time (Helm values, kustomize patches)
	// are never mistaken for ours and removed.
	ManagedTolerationsAnnotation = "serving.kserve.io/managed-tolerations"
)

// AgentDaemonSetReconciler keeps the tolerations of the localmodelnode agent DaemonSet in sync with
// the tolerations declared on LocalModelNodeGroup CRs. Download jobs inherit their node group's
// tolerations, but a job only ever gets created by the agent running on that node, so the agent has
// to tolerate the same taints or the models are never downloaded at all.
type AgentDaemonSetReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	// Namespace the agent DaemonSet lives in. Defaults to constants.KServeNamespace when empty.
	Namespace string
}

func (c *AgentDaemonSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	namespace := c.Namespace
	if namespace == "" {
		namespace = constants.KServeNamespace
	}

	daemonSet := &appsv1.DaemonSet{}
	daemonSetName := types.NamespacedName{Name: AgentDaemonSetName, Namespace: namespace}
	if err := c.Get(ctx, daemonSetName, daemonSet); err != nil {
		if apierr.IsNotFound(err) {
			// The agent is an optional component; nothing to sync when it is not installed.
			c.Log.V(1).Info("Agent DaemonSet not found, skipping toleration sync", "name", daemonSetName)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	nodeGroups := &v1alpha1.LocalModelNodeGroupList{}
	if err := c.List(ctx, nodeGroups); err != nil {
		c.Log.Error(err, "Failed to list node groups while syncing agent tolerations")
		return reconcile.Result{}, err
	}

	// List results come back sorted by name, so the union below is stable across reconciles.
	var nodeGroupTolerations []corev1.Toleration
	for _, nodeGroup := range nodeGroups.Items {
		nodeGroupTolerations = appendMissingTolerations(nodeGroupTolerations, nodeGroup.Spec.Tolerations)
	}

	previouslyManaged := managedTolerations(daemonSet, c.Log)
	// Anything on the DaemonSet we did not put there came from the install and must be preserved.
	var tolerations []corev1.Toleration
	for _, toleration := range daemonSet.Spec.Template.Spec.Tolerations {
		if !containsToleration(previouslyManaged, toleration) {
			tolerations = append(tolerations, toleration)
		}
	}
	tolerations = appendMissingTolerations(tolerations, nodeGroupTolerations)

	annotation := ""
	if len(nodeGroupTolerations) > 0 {
		marshalled, err := json.Marshal(nodeGroupTolerations)
		if err != nil {
			return reconcile.Result{}, err
		}
		annotation = string(marshalled)
	}

	if equalTolerations(daemonSet.Spec.Template.Spec.Tolerations, tolerations) &&
		daemonSet.Annotations[ManagedTolerationsAnnotation] == annotation {
		return reconcile.Result{}, nil
	}

	patch := client.MergeFrom(daemonSet.DeepCopy())
	daemonSet.Spec.Template.Spec.Tolerations = tolerations
	if annotation == "" {
		delete(daemonSet.Annotations, ManagedTolerationsAnnotation)
	} else {
		if daemonSet.Annotations == nil {
			daemonSet.Annotations = map[string]string{}
		}
		daemonSet.Annotations[ManagedTolerationsAnnotation] = annotation
	}

	c.Log.Info("Updating agent DaemonSet tolerations", "name", daemonSetName, "tolerations", tolerations)
	if err := c.Patch(ctx, daemonSet, patch); err != nil {
		c.Log.Error(err, "Failed to update agent DaemonSet tolerations", "name", daemonSetName)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

// managedTolerations returns the tolerations this controller applied on a previous reconcile.
func managedTolerations(daemonSet *appsv1.DaemonSet, log logr.Logger) []corev1.Toleration {
	annotation, ok := daemonSet.Annotations[ManagedTolerationsAnnotation]
	if !ok || annotation == "" {
		return nil
	}
	var tolerations []corev1.Toleration
	if err := json.Unmarshal([]byte(annotation), &tolerations); err != nil {
		// A malformed annotation would make us drop install-time tolerations, so keep them all.
		log.Error(err, "Failed to parse managed tolerations annotation", "annotation", ManagedTolerationsAnnotation)
		return nil
	}
	return tolerations
}

func appendMissingTolerations(tolerations []corev1.Toleration, toAppend []corev1.Toleration) []corev1.Toleration {
	for _, toleration := range toAppend {
		if !containsToleration(tolerations, toleration) {
			tolerations = append(tolerations, toleration)
		}
	}
	return tolerations
}

// equalTolerations compares two toleration lists, treating a nil list and an empty one as equal.
func equalTolerations(a, b []corev1.Toleration) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func containsToleration(tolerations []corev1.Toleration, toleration corev1.Toleration) bool {
	for _, existing := range tolerations {
		if reflect.DeepEqual(existing, toleration) {
			return true
		}
	}
	return false
}

func (c *AgentDaemonSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	namespace := c.Namespace
	if namespace == "" {
		namespace = constants.KServeNamespace
	}
	agentDaemonSet := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == AgentDaemonSetName && object.GetNamespace() == namespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		Named("localmodelnodegroup-agent-daemonset").
		For(&v1alpha1.LocalModelNodeGroup{}).
		// Re-apply the tolerations if the DaemonSet is redeployed or edited out from under us.
		Watches(&appsv1.DaemonSet{}, handler.EnqueueRequestsFromMapFunc(c.daemonSetFunc), builder.WithPredicates(agentDaemonSet)).
		Complete(c)
}

// Given the agent DaemonSet, enqueue every node group so the tolerations are recomputed.
func (c *AgentDaemonSetReconciler) daemonSetFunc(ctx context.Context, obj client.Object) []reconcile.Request {
	nodeGroups := &v1alpha1.LocalModelNodeGroupList{}
	if err := c.List(ctx, nodeGroups); err != nil {
		c.Log.Error(err, "list node groups error when reconciling agent DaemonSet")
		return []reconcile.Request{}
	}
	requests := make([]reconcile.Request, 0, len(nodeGroups.Items))
	for _, nodeGroup := range nodeGroups.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: nodeGroup.Name}})
	}
	return requests
}
