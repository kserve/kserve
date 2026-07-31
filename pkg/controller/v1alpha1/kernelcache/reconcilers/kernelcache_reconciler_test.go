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
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestJobFailed(t *testing.T) {
	reconciler := &KernelCacheReconciler{
		Log: logr.Discard(),
	}

	t.Run("returns true when job has failed condition", func(t *testing.T) {
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

	t.Run("returns false when job has not failed", func(t *testing.T) {
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
			t.Error("expected jobFailed to return false for completed job")
		}
	})

	t.Run("returns false when job has no conditions", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{},
			},
		}

		if reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return false for job with no conditions")
		}
	})

	t.Run("returns false when job failed condition is false", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		if reconciler.jobFailed(job) {
			t.Error("expected jobFailed to return false when failed condition status is false")
		}
	})
}

func TestJobCompleted(t *testing.T) {
	reconciler := &KernelCacheReconciler{
		Log: logr.Discard(),
	}

	t.Run("returns true when job has complete condition", func(t *testing.T) {
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

	t.Run("returns false when job has not completed", func(t *testing.T) {
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

		if reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return false for failed job")
		}
	})

	t.Run("returns false when job has no conditions", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{},
			},
		}

		if reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return false for job with no conditions")
		}
	})

	t.Run("returns false when job complete condition is false", func(t *testing.T) {
		job := &batchv1.Job{
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		if reconciler.jobCompleted(job) {
			t.Error("expected jobCompleted to return false when complete condition status is false")
		}
	})
}

func TestResolveStorageSize(t *testing.T) {
	reconciler := &KernelCacheReconciler{
		Log: logr.Discard(),
	}

	t.Run("uses spec.storageSize when provided", func(t *testing.T) {
		size := resource.MustParse("5Gi")
		kc := &v1alpha1.KernelCache{
			Spec: v1alpha1.KernelCacheSpec{
				StorageSize: &size,
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					v1alpha1.AnnotationCacheSizeBytes: "2147483648", // 2Gi in bytes
				},
			},
		}

		result := reconciler.resolveStorageSize(kc)
		if !result.Equal(size) {
			t.Errorf("expected %s, got %s", size.String(), result.String())
		}
	})

	t.Run("uses annotation when spec.storageSize is nil", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			Spec: v1alpha1.KernelCacheSpec{
				StorageSize: nil,
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					v1alpha1.AnnotationCacheSizeBytes: "3221225472", // 3Gi in bytes
				},
			},
		}

		result := reconciler.resolveStorageSize(kc)
		expected := resource.MustParse("3Gi")
		// Use Cmp to handle binary vs decimal differences
		if result.Cmp(expected) != 0 {
			t.Errorf("expected ~%s, got %s", expected.String(), result.String())
		}
	})

	t.Run("uses 10Gi default when both are missing", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			Spec: v1alpha1.KernelCacheSpec{
				StorageSize: nil,
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		}

		result := reconciler.resolveStorageSize(kc)
		expected := resource.MustParse("10Gi")
		if !result.Equal(expected) {
			t.Errorf("expected %s, got %s", expected.String(), result.String())
		}
	})

	t.Run("uses 10Gi default when annotation is invalid", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			Spec: v1alpha1.KernelCacheSpec{
				StorageSize: nil,
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					v1alpha1.AnnotationCacheSizeBytes: "not-a-number",
				},
			},
		}

		result := reconciler.resolveStorageSize(kc)
		expected := resource.MustParse("10Gi")
		if !result.Equal(expected) {
			t.Errorf("expected %s, got %s", expected.String(), result.String())
		}
	})

	t.Run("uses 10Gi default when annotation is zero", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			Spec: v1alpha1.KernelCacheSpec{
				StorageSize: nil,
			},
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					v1alpha1.AnnotationCacheSizeBytes: "0",
				},
			},
		}

		result := reconciler.resolveStorageSize(kc)
		expected := resource.MustParse("10Gi")
		if !result.Equal(expected) {
			t.Errorf("expected %s, got %s", expected.String(), result.String())
		}
	})
}

func TestExtractionComplete(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	t.Run("returns true when extraction job is completed", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extract-test-cache-abc123",
				Namespace: "kserve",
				Labels: map[string]string{
					"kernelcache.kserve.io/cache":     "test-cache",
					"kernelcache.kserve.io/namespace": "default",
					"app.kubernetes.io/component":     "extract",
				},
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, job).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		if !reconciler.extractionComplete(context.TODO(), kc, "kserve") {
			t.Error("expected extractionComplete to return true for completed job")
		}
	})

	t.Run("returns false when extraction job is not completed", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "extract-test-cache-abc123",
				Namespace: "kserve",
				Labels: map[string]string{
					"kernelcache.kserve.io/cache":     "test-cache",
					"kernelcache.kserve.io/namespace": "default",
					"app.kubernetes.io/component":     "extract",
				},
			},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, job).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		if reconciler.extractionComplete(context.TODO(), kc, "kserve") {
			t.Error("expected extractionComplete to return false for non-completed job")
		}
	})

	t.Run("returns false when extraction job does not exist", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		if reconciler.extractionComplete(context.TODO(), kc, "kserve") {
			t.Error("expected extractionComplete to return false when job does not exist")
		}
	})
}

func TestReconcileImageVolume(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	t.Run("sets status.mountType to imageVolume on first call", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image:     "test-image:latest",
				MountType: v1alpha1.KernelCacheMountTypeImageVolume,
			},
			Status: v1alpha1.KernelCacheStatus{
				MountType: "", // Not yet set
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		_, err := reconciler.reconcileImageVolume(context.TODO(), kc, nil, v1alpha1.KernelCacheMountTypeImageVolume)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Retrieve updated object
		updated := &v1alpha1.KernelCache{}
		err = k8sClient.Get(context.TODO(), client.ObjectKey{Name: "test-cache", Namespace: "default"}, updated)
		if err != nil {
			t.Fatalf("failed to get updated cache: %v", err)
		}

		if updated.Status.MountType != v1alpha1.KernelCacheMountTypeImageVolume {
			t.Errorf("expected mountType to be %s, got %s",
				v1alpha1.KernelCacheMountTypeImageVolume, updated.Status.MountType)
		}
	})

	t.Run("does not update status.mountType if already set", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
				// Track resource version to detect status updates
				ResourceVersion: "1",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image:     "test-image:latest",
				MountType: v1alpha1.KernelCacheMountTypeImageVolume,
			},
			Status: v1alpha1.KernelCacheStatus{
				MountType: v1alpha1.KernelCacheMountTypeImageVolume, // Already set
				State:     v1alpha1.CacheStatePending,
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		_, err := reconciler.reconcileImageVolume(context.TODO(), kc, nil, v1alpha1.KernelCacheMountTypeImageVolume)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify no PVCs were created
		pvcList := &corev1.PersistentVolumeClaimList{}
		err = k8sClient.List(context.TODO(), pvcList)
		if err != nil {
			t.Fatalf("failed to list PVCs: %v", err)
		}
		if len(pvcList.Items) > 0 {
			t.Errorf("expected no PVCs to be created, found %d", len(pvcList.Items))
		}

		// Verify no Jobs were created
		jobList := &batchv1.JobList{}
		err = k8sClient.List(context.TODO(), jobList)
		if err != nil {
			t.Fatalf("failed to list Jobs: %v", err)
		}
		if len(jobList.Items) > 0 {
			t.Errorf("expected no Jobs to be created, found %d", len(jobList.Items))
		}
	})

	t.Run("returns no error and no requeue for successful reconciliation", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image:     "test-image:latest",
				MountType: v1alpha1.KernelCacheMountTypeImageVolume,
			},
			Status: v1alpha1.KernelCacheStatus{
				MountType: v1alpha1.KernelCacheMountTypeImageVolume,
				State:     v1alpha1.CacheStatePending,
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		result, err := reconciler.reconcileImageVolume(context.TODO(), kc, nil, v1alpha1.KernelCacheMountTypeImageVolume)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if result.Requeue {
			t.Error("expected no requeue")
		}
		if result.RequeueAfter != 0 {
			t.Errorf("expected no requeue after, got %v", result.RequeueAfter)
		}
	})

	t.Run("handles missing KernelCacheNodes gracefully", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image:     "test-image:latest",
				MountType: v1alpha1.KernelCacheMountTypeImageVolume,
			},
			Status: v1alpha1.KernelCacheStatus{
				MountType: v1alpha1.KernelCacheMountTypeImageVolume,
			},
		}

		// No KernelCacheNodes created
		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		result, err := reconciler.reconcileImageVolume(context.TODO(), kc, nil, v1alpha1.KernelCacheMountTypeImageVolume)
		if err != nil {
			t.Fatalf("expected no error with missing nodes, got %v", err)
		}
		if result.Requeue {
			t.Error("expected no requeue")
		}

		// Verify status is still Pending (no nodes = no state change)
		updated := &v1alpha1.KernelCache{}
		err = k8sClient.Get(context.TODO(), client.ObjectKey{Name: "test-cache", Namespace: "default"}, updated)
		if err != nil {
			t.Fatalf("failed to get updated cache: %v", err)
		}
		if updated.Status.State != v1alpha1.CacheStatePending {
			t.Errorf("expected state Pending, got %s", updated.Status.State)
		}
	})
}

func TestUpdateAggregateStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	t.Run("transitions from Pending to Extracted when KernelCacheNodes report extracted", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
			Status: v1alpha1.KernelCacheStatus{
				State: v1alpha1.CacheStatePending,
			},
		}

		// Create KernelCacheNode with extracted state
		kcNode := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node1",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateExtracted,
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, kcNode).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kc.Status.State != v1alpha1.CacheStateExtracted {
			t.Errorf("expected state Extracted, got %s", kc.Status.State)
		}
		if kc.Status.Counts.NodeCnt != 1 {
			t.Errorf("expected NodeCnt=1, got %d", kc.Status.Counts.NodeCnt)
		}
		if kc.Status.Counts.NodeNotInUseCnt != 1 {
			t.Errorf("expected NodeNotInUseCnt=1, got %d", kc.Status.Counts.NodeNotInUseCnt)
		}
	})

	t.Run("transitions from Extracted to Running when pods mount the cache", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
			Status: v1alpha1.KernelCacheStatus{
				State: v1alpha1.CacheStateExtracted,
			},
		}

		// KernelCacheNode with Running state (pods using cache)
		kcNode := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node1",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateRunning,
						ServingNamespaces: map[string]v1alpha1.NamespaceServingCounts{
							"user-ns": {
								PodsUsing: 2,
								PodsReady: 2,
							},
						},
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, kcNode).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kc.Status.State != v1alpha1.CacheStateRunning {
			t.Errorf("expected state Running, got %s", kc.Status.State)
		}
		if kc.Status.Counts.NodeInUseCnt != 1 {
			t.Errorf("expected NodeInUseCnt=1, got %d", kc.Status.Counts.NodeInUseCnt)
		}
		if kc.Status.ServingStatus.TotalPodsUsing != 2 {
			t.Errorf("expected TotalPodsUsing=2, got %d", kc.Status.ServingStatus.TotalPodsUsing)
		}
	})

	t.Run("transitions from Running to Extracted when all pods are deleted", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
			Status: v1alpha1.KernelCacheStatus{
				State: v1alpha1.CacheStateRunning,
			},
		}

		// KernelCacheNode back to Extracted (no pods using cache)
		kcNode := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node1",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:              "test-cache",
						Namespace:         "default",
						State:             v1alpha1.NodeCacheStateExtracted,
						ServingNamespaces: map[string]v1alpha1.NamespaceServingCounts{
							// No pods using cache anymore
						},
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, kcNode).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kc.Status.State != v1alpha1.CacheStateExtracted {
			t.Errorf("expected state Extracted, got %s", kc.Status.State)
		}
		if kc.Status.Counts.NodeNotInUseCnt != 1 {
			t.Errorf("expected NodeNotInUseCnt=1, got %d", kc.Status.Counts.NodeNotInUseCnt)
		}
		if kc.Status.ServingStatus.TotalPodsUsing != 0 {
			t.Errorf("expected TotalPodsUsing=0, got %d", kc.Status.ServingStatus.TotalPodsUsing)
		}
	})

	t.Run("prioritizes Error state over Running", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		// Multiple nodes: one in error, one running
		kcNode1 := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node1",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateError,
						Message:   "Extraction failed",
					},
				},
			},
		}

		kcNode2 := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node2",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateRunning,
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, kcNode1, kcNode2).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Error takes precedence
		if kc.Status.State != v1alpha1.CacheStateError {
			t.Errorf("expected state Error, got %s", kc.Status.State)
		}
		if kc.Status.Counts.NodeErrorCnt != 1 {
			t.Errorf("expected NodeErrorCnt=1, got %d", kc.Status.Counts.NodeErrorCnt)
		}
		if kc.Status.Counts.NodeInUseCnt != 1 {
			t.Errorf("expected NodeInUseCnt=1, got %d", kc.Status.Counts.NodeInUseCnt)
		}
	})

	t.Run("aggregates GPU compatibility from multiple nodes", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		kcNode1 := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node1",
				GPUInfo: []v1alpha1.GPUTypeInfo{
					{
						GPUType: "nvidia-a100",
						IDs:     []int{0, 1},
					},
				},
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:           "test-cache",
						Namespace:      "default",
						State:          v1alpha1.NodeCacheStateExtracted,
						CompatibleGPUs: []int{0, 1},
					},
				},
			},
		}

		kcNode2 := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node2",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node2",
				GPUInfo: []v1alpha1.GPUTypeInfo{
					{
						GPUType: "Aldebaran/MI200",
						IDs:     []int{0},
					},
				},
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:             "test-cache",
						Namespace:        "default",
						State:            v1alpha1.NodeCacheStateExtracted,
						IncompatibleGPUs: []int{0},
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, kcNode1, kcNode2).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kc.Status.GPUCompatibility.TotalCompatibleGPUs != 2 {
			t.Errorf("expected TotalCompatibleGPUs=2, got %d", kc.Status.GPUCompatibility.TotalCompatibleGPUs)
		}
		if kc.Status.GPUCompatibility.TotalIncompatibleGPUs != 1 {
			t.Errorf("expected TotalIncompatibleGPUs=1, got %d", kc.Status.GPUCompatibility.TotalIncompatibleGPUs)
		}

		// Check GPU types are aggregated
		hasCompatibleA100 := false
		for _, gpuType := range kc.Status.GPUCompatibility.CompatibleTypes {
			if gpuType == "nvidia-a100" {
				hasCompatibleA100 = true
			}
		}
		if !hasCompatibleA100 {
			t.Error("expected nvidia-a100 in compatible types")
		}

		hasIncompatibleMI200 := false
		for _, gpuType := range kc.Status.GPUCompatibility.IncompatibleTypes {
			if gpuType == "Aldebaran/MI200" {
				hasIncompatibleMI200 = true
			}
		}
		if !hasIncompatibleMI200 {
			t.Error("expected Aldebaran/MI200 in incompatible types")
		}
	})

	t.Run("aggregates serving status across multiple namespaces", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		kcNode := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node1",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateRunning,
						ServingNamespaces: map[string]v1alpha1.NamespaceServingCounts{
							"ns1": {
								PodsUsing: 3,
								PodsReady: 2,
							},
							"ns2": {
								PodsUsing:       2,
								PodsReady:       1,
								PodsTerminating: 1,
							},
						},
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, kcNode).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kc.Status.ServingStatus.TotalNamespaces != 2 {
			t.Errorf("expected TotalNamespaces=2, got %d", kc.Status.ServingStatus.TotalNamespaces)
		}
		if kc.Status.ServingStatus.TotalPodsUsing != 5 {
			t.Errorf("expected TotalPodsUsing=5, got %d", kc.Status.ServingStatus.TotalPodsUsing)
		}
		if kc.Status.ServingStatus.TotalPodsReady != 3 {
			t.Errorf("expected TotalPodsReady=3, got %d", kc.Status.ServingStatus.TotalPodsReady)
		}
		if kc.Status.ServingStatus.TotalPodsTerminating != 1 {
			t.Errorf("expected TotalPodsTerminating=1, got %d", kc.Status.ServingStatus.TotalPodsTerminating)
		}
	})

	t.Run("handles no KernelCacheNodes without error", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("expected no error with missing nodes, got %v", err)
		}

		// Should remain in Pending state
		if kc.Status.State != v1alpha1.CacheStatePending {
			t.Errorf("expected state Pending, got %s", kc.Status.State)
		}
		if kc.Status.Counts.NodeCnt != 0 {
			t.Errorf("expected NodeCnt=0, got %d", kc.Status.Counts.NodeCnt)
		}
	})

	t.Run("transitions to Downloading when nodes exist but no extraction complete", func(t *testing.T) {
		kc := &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cache",
				Namespace: "default",
			},
			Spec: v1alpha1.KernelCacheSpec{
				Image: "test-image:latest",
			},
		}

		kcNode := &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
			Status: v1alpha1.KernelCacheNodeStatus{
				NodeName: "node1",
				CacheStatus: map[string]v1alpha1.CacheNodeCacheInfo{
					"default/test-cache": {
						Name:      "test-cache",
						Namespace: "default",
						State:     v1alpha1.NodeCacheStateDownloading,
					},
				},
			},
		}

		k8sClient := fakeclient.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(kc, kcNode).
			WithStatusSubresource(kc).
			Build()

		reconciler := &KernelCacheReconciler{
			Client:    k8sClient,
			Clientset: fake.NewSimpleClientset(),
			Log:       logr.Discard(),
			Scheme:    scheme,
		}

		err := reconciler.updateAggregateStatus(context.TODO(), kc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kc.Status.State != v1alpha1.CacheStateDownloading {
			t.Errorf("expected state Downloading, got %s", kc.Status.State)
		}
		if kc.Status.Counts.NodeCnt != 1 {
			t.Errorf("expected NodeCnt=1, got %d", kc.Status.Counts.NodeCnt)
		}
	})
}
