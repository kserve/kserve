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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func (r *KernelCacheReconciler) updateStatus(
	ctx context.Context,
	kernelCache *v1alpha1.KernelCache,
	state v1alpha1.KernelCacheState,
	nodeCount int,
	reason string,
	message string,
	mountType v1alpha1.KernelCacheMountType,
) error {
	return r.updateStatusWithCounts(ctx, kernelCache, state, nodeCount, 0, 0, 0, reason, message, mountType)
}

func (r *KernelCacheReconciler) updateStatusWithCounts(
	ctx context.Context,
	kernelCache *v1alpha1.KernelCache,
	state v1alpha1.KernelCacheState,
	nodeCount int,
	nodesReady int,
	nodesPreparing int,
	nodesError int,
	reason string,
	message string,
	mountType v1alpha1.KernelCacheMountType,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.KernelCache{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(kernelCache), current); err != nil {
			return err
		}

		desiredStatus := current.Status.DeepCopy()
		if desiredStatus == nil {
			desiredStatus = &v1alpha1.KernelCacheStatus{}
		}
		desiredStatus.State = state
		desiredStatus.MountType = mountType
		desiredStatus.Counts = &v1alpha1.KernelCacheCounts{
			NodeCount:      nodeCount,
			NodesReady:     nodesReady,
			NodesPreparing: nodesPreparing,
			NodesError:     nodesError,
		}
		meta.SetStatusCondition(&desiredStatus.Conditions, metav1.Condition{
			Type:               kernelCacheReadyConditionType,
			Status:             conditionStatusForKernelCacheState(state),
			ObservedGeneration: current.Generation,
			Reason:             reason,
			Message:            message,
		})

		if reflect.DeepEqual(current.Status, *desiredStatus) {
			return nil
		}
		current.Status = *desiredStatus
		return r.Status().Update(ctx, current)
	})
}

type kernelCacheAggregate struct {
	State          v1alpha1.KernelCacheState
	NodeCount      int
	NodesReady     int
	NodesPreparing int
	NodesError     int
	Reason         string
	Message        string
}

func (r *KernelCacheReconciler) aggregateKernelCacheStatus(
	ctx context.Context,
	kernelCache *v1alpha1.KernelCache,
	readyNodes *corev1.NodeList,
) (kernelCacheAggregate, error) {
	expectedNodes := make([]string, 0, len(readyNodes.Items))
	for i := range readyNodes.Items {
		expectedNodes = append(expectedNodes, readyNodes.Items[i].Name)
	}
	kernelCacheNodes := &v1alpha1.KernelCacheNodeList{}
	if err := r.List(ctx, kernelCacheNodes); err != nil {
		return kernelCacheAggregate{}, err
	}
	return aggregateKernelCacheNodeStatuses(kernelCache, expectedNodes, kernelCacheNodes), nil
}

func aggregateKernelCacheNodeStatuses(
	kernelCache *v1alpha1.KernelCache,
	expectedNodes []string,
	kernelCacheNodes *v1alpha1.KernelCacheNodeList,
) kernelCacheAggregate {
	cacheKey := kernelCache.Namespace + "/" + kernelCache.Name
	nodeStatuses := make(map[string]v1alpha1.KernelCacheNodeCacheInfo, len(kernelCacheNodes.Items))
	for i := range kernelCacheNodes.Items {
		if cacheInfo, ok := kernelCacheNodes.Items[i].Status.CacheStatus[cacheKey]; ok {
			nodeStatuses[kernelCacheNodes.Items[i].Name] = cacheInfo
		}
	}

	aggregate := kernelCacheAggregate{
		State:     v1alpha1.KernelCacheStatePending,
		NodeCount: len(expectedNodes),
		Reason:    reasonWaitingForPreparation,
		Message:   "waiting for node cache preparation status",
	}
	for _, nodeName := range expectedNodes {
		cacheInfo, exists := nodeStatuses[nodeName]
		if !exists {
			aggregate.NodesPreparing++
			continue
		}
		switch cacheInfo.State {
		case v1alpha1.KernelCacheNodePreparationStateReady:
			aggregate.NodesReady++
		case v1alpha1.KernelCacheNodePreparationStateError:
			aggregate.NodesError++
			if aggregate.Message == "waiting for node cache preparation status" && cacheInfo.Message != "" {
				aggregate.Message = cacheInfo.Message
			}
		default:
			aggregate.NodesPreparing++
		}
	}

	switch {
	case aggregate.NodesError > 0:
		aggregate.State = v1alpha1.KernelCacheStateError
		aggregate.Reason = reasonPreparationFailed
	case aggregate.NodeCount > 0 && aggregate.NodesReady == aggregate.NodeCount:
		aggregate.State = v1alpha1.KernelCacheStateReady
		aggregate.Reason = reasonCacheReady
		aggregate.Message = "cache is ready on all selected nodes"
	case aggregate.NodesPreparing > 0:
		aggregate.State = v1alpha1.KernelCacheStatePreparing
		aggregate.Reason = reasonPreparing
		aggregate.Message = "cache preparation is in progress"
	}
	return aggregate
}

func conditionStatusForKernelCacheState(state v1alpha1.KernelCacheState) metav1.ConditionStatus {
	if state == v1alpha1.KernelCacheStateReady {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}
