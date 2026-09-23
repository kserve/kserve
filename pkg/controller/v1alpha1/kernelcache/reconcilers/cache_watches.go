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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	kernelcacheconfig "github.com/kserve/kserve/pkg/kernelcache/config"
	"github.com/kserve/kserve/pkg/kernelcache/nodegroup"
)

// SetupWithManager registers KC and the resources that can change its status.
func (r *KernelCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KernelCache{}).
		Watches(&v1alpha1.KernelCacheNodeGroup{}, handler.EnqueueRequestsFromMapFunc(r.enqueueKCsOnNodeGroupChange)).
		Watches(&v1alpha1.KernelCacheNode{}, handler.EnqueueRequestsFromMapFunc(r.enqueueKCsOnKCNChange)).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(r.enqueueKCsOnNodeChange), builder.WithPredicates(kernelCacheNodePredicate())).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.enqueueKCsOnConfigChange), builder.WithPredicates(predicate.NewPredicateFuncs(isInferenceServiceConfigMap))).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.enqueueKCOnPodUsageChange), builder.WithPredicates(consumerPodPredicate())).
		Complete(r)
}

func isInferenceServiceConfigMap(obj client.Object) bool {
	return obj != nil && obj.GetNamespace() == constants.KServeNamespace && obj.GetName() == constants.InferenceServiceConfigMapName
}

// kernelCacheNodePredicate forwards only Node events that can change placement.
func kernelCacheNodePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return e.Object != nil },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, oldOK := e.ObjectOld.(*corev1.Node)
			newNode, newOK := e.ObjectNew.(*corev1.Node)
			if !oldOK || !newOK {
				return false
			}
			return !reflect.DeepEqual(oldNode.Labels, newNode.Labels) || nodegroup.IsNodeReady(*oldNode) != nodegroup.IsNodeReady(*newNode)
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return e.Object != nil },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// enqueueKCsOnNodeGroupChange selects KCs whose effective NodeGroup changed.
func (r *KernelCacheReconciler) enqueueKCsOnNodeGroupChange(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.kcRequestsForNodeGroup(ctx, obj.GetName())
}

// enqueueKCsOnNodeChange selects KCs whose effective NodeGroup matches the Node.
func (r *KernelCacheReconciler) enqueueKCsOnNodeChange(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.kcRequestsForMatchingNodeGroups(ctx, obj)
}

// enqueueKCsOnConfigChange requeues all KCs because config values are global.
func (r *KernelCacheReconciler) enqueueKCsOnConfigChange(ctx context.Context, obj client.Object) []reconcile.Request {
	if !isInferenceServiceConfigMap(obj) {
		return nil
	}
	return r.kcRequestsForNodeGroup(ctx, "")
}

// kcRequestsForMatchingNodeGroups finds KCs affected by the supplied Node.
func (r *KernelCacheReconciler) kcRequestsForMatchingNodeGroups(ctx context.Context, obj client.Object) []reconcile.Request {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil
	}
	nodeGroups := &v1alpha1.KernelCacheNodeGroupList{}
	if err := r.List(ctx, nodeGroups); err != nil {
		return nil
	}
	matchingGroups := nodegroup.MatchingGroups(node, nodeGroups.Items)
	if len(matchingGroups) == 0 {
		return nil
	}
	matchingGroupNames := make(map[string]struct{}, len(matchingGroups))
	for i := range matchingGroups {
		matchingGroupNames[matchingGroups[i].Name] = struct{}{}
	}
	defaultNodeGroup := ""
	if config, err := kernelcacheconfig.Load(ctx, r.Client); err == nil {
		defaultNodeGroup = config.DefaultNodeGroup
	}

	kernelCaches := &v1alpha1.KernelCacheList{}
	if err := r.List(ctx, kernelCaches); err != nil {
		return nil
	}
	requests := make([]reconcile.Request, 0, len(kernelCaches.Items))
	for i := range kernelCaches.Items {
		cache := &kernelCaches.Items[i]
		groupName := kernelCacheNodeGroupName(cache, defaultNodeGroup)
		if _, matches := matchingGroupNames[groupName]; matches {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cache)})
		}
	}
	return requests
}

// kcRequestsForNodeGroup returns KCs using nodeGroupName. An empty name means all KCs.
func (r *KernelCacheReconciler) kcRequestsForNodeGroup(ctx context.Context, nodeGroupName string) []reconcile.Request {
	kernelCaches := &v1alpha1.KernelCacheList{}
	if err := r.List(ctx, kernelCaches); err != nil {
		return nil
	}
	defaultNodeGroup := ""
	if nodeGroupName != "" {
		if config, err := kernelcacheconfig.Load(ctx, r.Client); err == nil {
			defaultNodeGroup = config.DefaultNodeGroup
		}
	}
	requests := make([]reconcile.Request, 0, len(kernelCaches.Items))
	for i := range kernelCaches.Items {
		cache := &kernelCaches.Items[i]
		if nodeGroupName != "" && kernelCacheNodeGroupName(cache, defaultNodeGroup) != nodeGroupName {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cache)})
	}
	return requests
}

// enqueueKCsOnKCNChange propagates per-node preparation status to referenced KCs.
func (r *KernelCacheReconciler) enqueueKCsOnKCNChange(_ context.Context, obj client.Object) []reconcile.Request {
	kernelCacheNode, ok := obj.(*v1alpha1.KernelCacheNode)
	if !ok {
		return nil
	}
	seen := make(map[types.NamespacedName]struct{})
	requests := make([]reconcile.Request, 0, len(kernelCacheNode.Status.CacheStatus))
	for _, cacheInfo := range kernelCacheNode.Status.CacheStatus {
		key := types.NamespacedName{Namespace: cacheInfo.KernelCacheRef.Namespace, Name: cacheInfo.KernelCacheRef.Name}
		if key.Namespace == "" || key.Name == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}
	return requests
}
