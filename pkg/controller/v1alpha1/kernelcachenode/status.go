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

package kernelcachenode

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/kernelcache/nodegroup"
)

// Status reconciliation and conflict-safe updates.
func (r *KernelCacheNodeReconciler) reconcileStatus(
	ctx context.Context,
	config *v1beta1.KernelCacheConfig,
	validateImages bool,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		kernelCacheNode := &v1alpha1.KernelCacheNode{}
		if err := r.Get(ctx, client.ObjectKey{Name: r.NodeName}, kernelCacheNode); err != nil {
			return fmt.Errorf("get KernelCacheNode %q: %w", r.NodeName, err)
		}

		oldStatus := kernelCacheNode.Status.DeepCopy()
		podsUsing, needsImageValidation, err := r.discoverCaches(ctx, kernelCacheNode, config.DefaultNodeGroup)
		if err != nil {
			return fmt.Errorf("discover KernelCaches for node %q: %w", r.NodeName, err)
		}
		if err := r.updateCacheStatuses(ctx, kernelCacheNode, config, validateImages || needsImageValidation); err != nil {
			return fmt.Errorf("update KernelCache statuses for node %q: %w", r.NodeName, err)
		}
		r.updateCounts(kernelCacheNode, podsUsing)

		if !reflect.DeepEqual(*oldStatus, kernelCacheNode.Status) {
			if err := r.Status().Update(ctx, kernelCacheNode); err != nil {
				return fmt.Errorf("update KernelCacheNode status for %q: %w", r.NodeName, err)
			}
			r.logStatusChanges(oldStatus, &kernelCacheNode.Status)
		}
		return nil
	})
}

func (r *KernelCacheNodeReconciler) logStatusChanges(
	oldStatus *v1alpha1.KernelCacheNodeStatus,
	newStatus *v1alpha1.KernelCacheNodeStatus,
) {
	oldCacheStatus := map[string]v1alpha1.KernelCacheNodeCacheInfo{}
	if oldStatus != nil {
		oldCacheStatus = oldStatus.CacheStatus
	}

	for cacheKey, newInfo := range newStatus.CacheStatus {
		oldInfo, exists := oldCacheStatus[cacheKey]
		if exists && oldInfo.State == newInfo.State && oldInfo.Message == newInfo.Message {
			continue
		}

		oldState := ""
		if exists {
			oldState = string(oldInfo.State)
		}
		r.Log.Info("KernelCache node cache status changed",
			"node", r.NodeName,
			"cache", cacheKey,
			"oldState", oldState,
			"state", newInfo.State,
			"message", newInfo.Message,
		)
	}

	for cacheKey, oldInfo := range oldCacheStatus {
		if _, exists := newStatus.CacheStatus[cacheKey]; exists {
			continue
		}
		r.Log.Info("KernelCache node cache status removed",
			"node", r.NodeName,
			"cache", cacheKey,
			"oldState", oldInfo.State,
		)
	}
}

func (r *KernelCacheNodeReconciler) discoverCaches(ctx context.Context, kernelCacheNode *v1alpha1.KernelCacheNode, defaultNodeGroup string) (int, bool, error) {
	caches := &v1alpha1.KernelCacheList{}
	if err := r.List(ctx, caches); err != nil {
		return 0, false, fmt.Errorf("list KernelCaches: %w", err)
	}

	if kernelCacheNode.Status.CacheStatus == nil {
		kernelCacheNode.Status.CacheStatus = make(map[string]v1alpha1.KernelCacheNodeCacheInfo)
	}

	activeCaches := make(map[string]struct{}, len(caches.Items))
	podsUsing := make(map[types.UID]struct{})
	needsImageValidation := false
	matchingGroupNames, err := r.matchingNodeGroups(ctx, caches.Items, defaultNodeGroup)
	if err != nil {
		return 0, false, err
	}
	for i := range caches.Items {
		kernelCache := &caches.Items[i]
		if !matchesNodeGroup(kernelCache, matchingGroupNames, defaultNodeGroup) {
			continue
		}
		if usage := kernelCache.Status.Usage; usage != nil {
			for _, podUsage := range usage.Pods {
				if podUsage.NodeName == r.NodeName && podUsage.PodUID != "" {
					podsUsing[podUsage.PodUID] = struct{}{}
				}
			}
		}

		cacheKey := kernelCacheKey(kernelCache.Namespace, kernelCache.Name)
		activeCaches[cacheKey] = struct{}{}
		cacheInfo, exists := kernelCacheNode.Status.CacheStatus[cacheKey]
		if !exists {
			needsImageValidation = true
			cacheInfo = v1alpha1.KernelCacheNodeCacheInfo{
				KernelCacheRef: v1alpha1.NamespacedName{
					Namespace: kernelCache.Namespace,
					Name:      kernelCache.Name,
				},
				State:      v1alpha1.KernelCacheNodePreparationStatePending,
				LastUpdate: metav1.Now(),
			}
		}

		cacheInfo.KernelCacheRef = v1alpha1.NamespacedName{
			Namespace: kernelCache.Namespace,
			Name:      kernelCache.Name,
		}
		if cacheInfo.ImageReference != kernelCache.Spec.Artifact.ImageReference || cacheInfo.Footprints != kernelCache.Spec.Artifact.Identity.Footprints {
			needsImageValidation = true
			cacheInfo.ImageReference = kernelCache.Spec.Artifact.ImageReference
			cacheInfo.Footprints = kernelCache.Spec.Artifact.Identity.Footprints
			cacheInfo.State = v1alpha1.KernelCacheNodePreparationStatePending
			cacheInfo.Message = "waiting for the preparation Job"
			cacheInfo.LastUpdate = metav1.Now()
		}
		kernelCacheNode.Status.CacheStatus[cacheKey] = cacheInfo
	}

	for cacheKey := range kernelCacheNode.Status.CacheStatus {
		if _, exists := activeCaches[cacheKey]; !exists {
			delete(kernelCacheNode.Status.CacheStatus, cacheKey)
		}
	}
	return len(podsUsing), needsImageValidation, nil
}

func (r *KernelCacheNodeReconciler) matchingNodeGroups(
	ctx context.Context,
	caches []v1alpha1.KernelCache,
	defaultNodeGroup string,
) (map[string]struct{}, error) {
	matchingGroups := make(map[string]struct{})
	if len(caches) == 0 {
		return matchingGroups, nil
	}

	node := &corev1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: r.NodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			return matchingGroups, nil
		}
		return nil, fmt.Errorf("get Node %q while matching KernelCaches: %w", r.NodeName, err)
	}
	if !nodegroup.IsNodeReady(*node) {
		return matchingGroups, nil
	}

	nodeGroups := &v1alpha1.KernelCacheNodeGroupList{}
	if err := r.List(ctx, nodeGroups); err != nil {
		return nil, fmt.Errorf("list KernelCacheNodeGroups: %w", err)
	}
	referencedGroups := make(map[string]struct{})
	for i := range caches {
		if name := nodegroup.ResolveKernelCacheNodeGroupName(&caches[i], defaultNodeGroup); name != "" {
			referencedGroups[name] = struct{}{}
		}
	}
	for i := range nodeGroups.Items {
		group := &nodeGroups.Items[i]
		if _, referenced := referencedGroups[group.Name]; referenced && len(group.Spec.NodeSelector) == 0 {
			return nil, fmt.Errorf("KernelCacheNodeGroup %q requires a non-empty nodeSelector", group.Name)
		}
	}

	for _, group := range nodegroup.MatchingGroups(node, nodeGroups.Items) {
		if _, referenced := referencedGroups[group.Name]; referenced {
			matchingGroups[group.Name] = struct{}{}
		}
	}
	return matchingGroups, nil
}

func matchesNodeGroup(kernelCache *v1alpha1.KernelCache, matchingGroups map[string]struct{}, defaultNodeGroup string) bool {
	name := nodegroup.ResolveKernelCacheNodeGroupName(kernelCache, defaultNodeGroup)
	if name == "" {
		return false
	}
	_, matched := matchingGroups[name]
	return matched
}

func kernelCacheKey(namespace, name string) string {
	return namespace + "/" + name
}

func (r *KernelCacheNodeReconciler) updateCounts(kernelCacheNode *v1alpha1.KernelCacheNode, podsUsing int) {
	counts := &v1alpha1.KernelCacheNodeCounts{}
	for _, cacheInfo := range kernelCacheNode.Status.CacheStatus {
		switch cacheInfo.State {
		case v1alpha1.KernelCacheNodePreparationStateReady:
			counts.CachesReady++
		case v1alpha1.KernelCacheNodePreparationStateError:
			counts.CachesError++
		default:
			counts.CachesPreparing++
		}
	}
	counts.TotalPodsUsing = podsUsing
	kernelCacheNode.Status.Counts = counts
}
