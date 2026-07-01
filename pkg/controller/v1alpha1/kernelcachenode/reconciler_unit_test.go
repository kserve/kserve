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
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestDiscoverCaches(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	t.Run("adds new KernelCache to CacheStatus", func(t *testing.T) {
		storageSize := resource.MustParse("1Gi")
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image:       "ghcr.io/test/kernels:latest",
				StorageSize: &storageSize,
			},
			Status: v1alpha1.KernelCacheStatus{
				ResolvedDigest: "sha256:test123",
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		kcNode := &v1alpha1.KernelCacheNode{
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName:    "test-node",
				CacheStatus: make(map[string]v1alpha1.CacheNodeCacheInfo),
			},
		}

		err := reconciler.discoverCaches(context.Background(), kcNode)
		if err != nil {
			t.Fatalf("discoverCaches failed: %v", err)
		}

		cacheKey := "default/test-cache"
		cacheInfo, exists := kcNode.Status.CacheStatus[cacheKey]
		if !exists {
			t.Fatalf("expected cache %s to be added to CacheStatus", cacheKey)
		}

		if cacheInfo.Name != "test-cache" {
			t.Errorf("expected cache name 'test-cache', got %s", cacheInfo.Name)
		}
		if cacheInfo.Namespace != "default" {
			t.Errorf("expected cache namespace 'default', got %s", cacheInfo.Namespace)
		}
		if cacheInfo.Image != "ghcr.io/test/kernels:latest" {
			t.Errorf("expected image 'ghcr.io/test/kernels:latest', got %s", cacheInfo.Image)
		}
		if cacheInfo.Digest != "sha256:test123" {
			t.Errorf("expected digest 'sha256:test123', got %s", cacheInfo.Digest)
		}
		if cacheInfo.State != v1alpha1.NodeCacheStatePending {
			t.Errorf("expected state Pending, got %s", cacheInfo.State)
		}
	})

	t.Run("removes deleted KernelCache from CacheStatus", func(t *testing.T) {
		// No KernelCache exists in cluster
		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		kcNode := &v1alpha1.KernelCacheNode{
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "test-node",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/deleted-cache": {
						Name:      "deleted-cache",
						Namespace: "default",
						Image:     "test:latest",
						State:     v1alpha1.NodeCacheStateExtracted,
					},
				},
			},
		}

		err := reconciler.discoverCaches(context.Background(), kcNode)
		if err != nil {
			t.Fatalf("discoverCaches failed: %v", err)
		}

		// Cache should be removed from CacheStatus
		if _, exists := kcNode.Status.CacheStatus["default/deleted-cache"]; exists {
			t.Errorf("expected deleted cache to be removed from CacheStatus")
		}
	})

	t.Run("handles empty cluster", func(t *testing.T) {
		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		kcNode := &v1alpha1.KernelCacheNode{
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName:    "test-node",
				CacheStatus: make(map[string]v1alpha1.CacheNodeCacheInfo),
			},
		}

		err := reconciler.discoverCaches(context.Background(), kcNode)
		if err != nil {
			t.Fatalf("discoverCaches failed: %v", err)
		}

		if len(kcNode.Status.CacheStatus) != 0 {
			t.Errorf("expected CacheStatus to be empty, got %d entries", len(kcNode.Status.CacheStatus))
		}
	})

	t.Run("updates existing cache entry", func(t *testing.T) {
		storageSize := resource.MustParse("1Gi")
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image:       "ghcr.io/test/kernels:v2",
				StorageSize: &storageSize,
			},
			Status: v1alpha1.KernelCacheStatus{
				ResolvedDigest: "sha256:newdigest",
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		kcNode := &v1alpha1.KernelCacheNode{
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "test-node",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						Image:     "ghcr.io/test/kernels:v1",
						Digest:    "sha256:olddigest",
						State:     v1alpha1.NodeCacheStateExtracted,
					},
				},
			},
		}

		err := reconciler.discoverCaches(context.Background(), kcNode)
		if err != nil {
			t.Fatalf("discoverCaches failed: %v", err)
		}

		cacheInfo := kcNode.Status.CacheStatus["default/test-cache"]
		if cacheInfo.Image != "ghcr.io/test/kernels:v2" {
			t.Errorf("expected image to be updated to v2, got %s", cacheInfo.Image)
		}
		if cacheInfo.Digest != "sha256:newdigest" {
			t.Errorf("expected digest to be updated, got %s", cacheInfo.Digest)
		}
		// State should be preserved
		if cacheInfo.State != v1alpha1.NodeCacheStateExtracted {
			t.Errorf("expected state to be preserved as Extracted, got %s", cacheInfo.State)
		}
	})
}

func TestGetExtractionJob(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	t.Run("finds Job by labels", func(t *testing.T) {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extract-test-cache-abc",
				Namespace: "kserve",
				Labels: map[string]string{
					"cache":           "test-cache",
					"cache-namespace": "default",
					"app":             "kernel-cache-extract",
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(job).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		cacheInfo := v1alpha1.CacheNodeCacheInfo{
			Name:      "test-cache",
			Namespace: "default",
		}

		foundJob, err := reconciler.getExtractionJob(context.Background(), cacheInfo, "kserve")
		if err != nil {
			t.Fatalf("getExtractionJob failed: %v", err)
		}

		if foundJob == nil {
			t.Fatal("expected to find Job, got nil")
		}

		if foundJob.Name != job.Name {
			t.Errorf("expected Job name %s, got %s", job.Name, foundJob.Name)
		}
	})

	t.Run("returns nil when Job not found", func(t *testing.T) {
		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		cacheInfo := v1alpha1.CacheNodeCacheInfo{
			Name:      "nonexistent-cache",
			Namespace: "default",
		}

		foundJob, err := reconciler.getExtractionJob(context.Background(), cacheInfo, "kserve")
		if err != nil {
			t.Fatalf("getExtractionJob failed: %v", err)
		}

		if foundJob != nil {
			t.Errorf("expected nil Job for nonexistent cache, got %v", foundJob)
		}
	})
}

func TestJobHelpers(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = batchv1.AddToScheme(scheme)

	reconciler := &KernelCacheNodeReconciler{
		Log:    logr.Discard(),
		Scheme: scheme,
	}

	t.Run("jobCompleted returns true for completed job", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		if !reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return true for completed job")
		}
	})

	t.Run("jobCompleted returns false for running job", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Active: 1,
			},
		}

		if reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return false for running job")
		}
	})

	t.Run("jobFailed returns true for failed job", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		if !reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return true for failed job")
		}
	})

	t.Run("jobFailed returns false for successful job", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		if reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return false for successful job")
		}
	})
}

func TestUpdateServingCounts(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	t.Run("counts pods mounting cache PVC", func(t *testing.T) {
		pod1 := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node",
				Volumes: []corev1.Volume{
					{
						Name: "cache-vol",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-cache-serving",
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pod1).
			WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
				pod := o.(*corev1.Pod)
				return []string{pod.Spec.NodeName}
			}).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		kcNode := &v1alpha1.KernelCacheNode{
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "test-node",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateExtracted,
					},
				},
			},
		}

		err := reconciler.updateServingCounts(context.Background(), kcNode)
		if err != nil {
			t.Fatalf("updateServingCounts failed: %v", err)
		}

		cacheInfo := kcNode.Status.CacheStatus["default/test-cache"]

		// Should count default namespace
		if len(cacheInfo.ServingNamespaces) != 1 {
			t.Errorf("expected 1 namespace, got %d", len(cacheInfo.ServingNamespaces))
		}

		counts, ok := cacheInfo.ServingNamespaces["default"]
		if !ok {
			t.Fatal("expected default namespace in ServingNamespaces")
		}

		if counts.PodsUsing != 1 {
			t.Errorf("expected 1 pod using cache, got %d", counts.PodsUsing)
		}
		if counts.PodsReady != 1 {
			t.Errorf("expected 1 ready pod, got %d", counts.PodsReady)
		}
	})

	t.Run("handles terminating pods", func(t *testing.T) {
		now := metav1.Now()
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "terminating-pod",
				Namespace:         "default",
				DeletionTimestamp: &now,
				Finalizers:        []string{"test-finalizer"}, // Need finalizer for DeletionTimestamp
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node",
				Volumes: []corev1.Volume{
					{
						Name: "cache-vol",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-cache",
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pod).
			WithIndex(&corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
				pod := o.(*corev1.Pod)
				return []string{pod.Spec.NodeName}
			}).
			Build()

		reconciler := &KernelCacheNodeReconciler{
			Client: k8sClient,
			Log:    logr.Discard(),
			Scheme: scheme,
		}

		kcNode := &v1alpha1.KernelCacheNode{
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "test-node",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateExtracted,
					},
				},
			},
		}

		err := reconciler.updateServingCounts(context.Background(), kcNode)
		if err != nil {
			t.Fatalf("updateServingCounts failed: %v", err)
		}

		cacheInfo := kcNode.Status.CacheStatus["default/test-cache"]
		counts := cacheInfo.ServingNamespaces["default"]

		if counts.PodsTerminating != 1 {
			t.Errorf("expected 1 terminating pod, got %d", counts.PodsTerminating)
		}
	})
}
