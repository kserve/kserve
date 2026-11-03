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
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// consumerPodPredicate limits usage events to Pods that declare a KernelCache.
func consumerPodPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return hasKernelCacheUsageAnnotation(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPod, oldOK := e.ObjectOld.(*corev1.Pod)
			newPod, newOK := e.ObjectNew.(*corev1.Pod)
			if !oldOK || !newOK || (!hasKernelCacheUsageAnnotation(oldPod) && !hasKernelCacheUsageAnnotation(newPod)) {
				return false
			}
			return oldPod.Annotations[constants.KernelCacheUsageAnnotationKey] != newPod.Annotations[constants.KernelCacheUsageAnnotationKey] ||
				oldPod.Spec.NodeName != newPod.Spec.NodeName ||
				oldPod.Status.Phase != newPod.Status.Phase ||
				(oldPod.DeletionTimestamp == nil) != (newPod.DeletionTimestamp == nil)
		},
		DeleteFunc:  func(e event.DeleteEvent) bool { return hasKernelCacheUsageAnnotation(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return hasKernelCacheUsageAnnotation(e.Object) },
	}
}

func hasKernelCacheUsageAnnotation(obj client.Object) bool {
	return obj != nil && obj.GetAnnotations()[constants.KernelCacheUsageAnnotationKey] != ""
}

// enqueueKCOnPodUsageChange maps a Pod's usage annotation to its namespaced KC.
func (r *KernelCacheReconciler) enqueueKCOnPodUsageChange(ctx context.Context, obj client.Object) []reconcile.Request {
	usageRef := obj.GetAnnotations()[constants.KernelCacheUsageAnnotationKey]
	if usageRef == "" {
		// An update that removes the annotation still needs to clear the old usage entry.
		// The map handler receives only the new object, so refresh KCs in this namespace.
		kernelCaches := &v1alpha1.KernelCacheList{}
		if err := r.List(ctx, kernelCaches, client.InNamespace(obj.GetNamespace())); err != nil {
			return nil
		}
		requests := make([]reconcile.Request, 0, len(kernelCaches.Items))
		for i := range kernelCaches.Items {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&kernelCaches.Items[i])})
		}
		return requests
	}
	parts := strings.SplitN(usageRef, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] != obj.GetNamespace() {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: parts[0], Name: parts[1]}}}
}

func (r *KernelCacheReconciler) reconcileKernelCacheUsage(ctx context.Context, kernelCache *v1alpha1.KernelCache) error {
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods, client.InNamespace(kernelCache.Namespace)); err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.KernelCache{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(kernelCache), current); err != nil {
			return err
		}
		desired := buildKernelCacheUsage(pods.Items, kernelCache.Namespace+"/"+kernelCache.Name, current.Status.Usage)
		if reflect.DeepEqual(current.Status.Usage, desired) {
			return nil
		}
		current.Status.Usage = desired
		return r.Status().Update(ctx, current)
	})
}

func buildKernelCacheUsage(pods []corev1.Pod, usageRef string, previous *v1alpha1.KernelCacheUsage) *v1alpha1.KernelCacheUsage {
	previousByUID := make(map[types.UID]v1alpha1.KernelCachePodUsage)
	if previous != nil {
		for _, usage := range previous.Pods {
			previousByUID[usage.PodUID] = usage
		}
	}

	entries := make([]v1alpha1.KernelCachePodUsage, 0)
	for i := range pods {
		pod := &pods[i]
		if pod.Annotations[constants.KernelCacheUsageAnnotationKey] != usageRef ||
			pod.Spec.NodeName == "" || pod.DeletionTimestamp != nil ||
			pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed || pod.UID == "" {
			continue
		}
		usage, exists := previousByUID[pod.UID]
		if !exists {
			usage.ObservedAt = metav1.Now()
		}
		usage.PodUID = pod.UID
		usage.PodRef = v1alpha1.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
		usage.NodeName = pod.Spec.NodeName
		entries = append(entries, usage)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].PodUID < entries[j].PodUID })
	if len(entries) == 0 && previous == nil {
		return nil
	}
	return &v1alpha1.KernelCacheUsage{Pods: entries, TotalPodsUsing: len(entries)}
}
