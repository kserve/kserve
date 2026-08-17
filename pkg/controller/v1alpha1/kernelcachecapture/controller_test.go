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

package kernelcachecapture

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	_ = v1beta1.AddToScheme(s)
	return s
}

func newTestReconciler(objs ...runtime.Object) *KernelCacheCaptureReconciler {
	scheme := newTestScheme()
	clientObjs := make([]runtime.Object, 0, len(objs))
	clientObjs = append(clientObjs, objs...)
	fakeClient := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(clientObjs...).
		WithStatusSubresource(&v1alpha1.KernelCacheCapture{}).
		Build()

	return &KernelCacheCaptureReconciler{
		Client:   fakeClient,
		Log:      logr.Discard(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}
}

// --- extractDigestFromOutput tests ---

func TestExtractDigestFromOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name: "typical buildah push output",
			output: `Getting image source signatures
Copying blob sha256:abc123
Copying config sha256:def456
Writing manifest to image destination
sha256:7890abcdef1234567890abcdef1234567890abcdef1234567890abcdef123456`,
			expected: "sha256:7890abcdef1234567890abcdef1234567890abcdef1234567890abcdef123456",
		},
		{
			name:     "digest only",
			output:   "sha256:abc123",
			expected: "sha256:abc123",
		},
		{
			name:     "no digest in output",
			output:   "some random output\nno digest here",
			expected: "",
		},
		{
			name:     "empty output",
			output:   "",
			expected: "",
		},
		{
			name: "digest in middle of output",
			output: `line 1
sha256:middle123
line 3`,
			expected: "sha256:middle123",
		},
		{
			name: "multiple sha256 lines takes last",
			output: `sha256:first
sha256:second`,
			expected: "sha256:second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDigestFromOutput(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- containsString / removeString tests ---

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"a", "b", "c"}, "b"))
	assert.False(t, containsString([]string{"a", "b", "c"}, "d"))
	assert.False(t, containsString(nil, "a"))
	assert.False(t, containsString([]string{}, "a"))
}

func TestRemoveString(t *testing.T) {
	result := removeString([]string{"a", "b", "c"}, "b")
	assert.Equal(t, []string{"a", "c"}, result)

	result = removeString([]string{"a", "b", "c"}, "d")
	assert.Equal(t, []string{"a", "b", "c"}, result)

	result = removeString([]string{"a"}, "a")
	assert.Empty(t, result)
}

// --- Reconcile tests ---

func TestReconcile_NotFound(t *testing.T) {
	r := newTestReconciler()

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcile_AddsFinalizer(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kcc", Namespace: "default"},
	})
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue after adding finalizer")

	// Verify finalizer was added
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.Contains(t, updated.Finalizers, KCCFinalizerName)
}

func TestReconcile_InitializesPhase(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kcc", Namespace: "default"},
	})
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue after initializing phase")

	// Verify phase was set to Pending
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.Equal(t, v1alpha1.KernelCacheCapturePhasePending, updated.Status.Phase)
}

func TestReconcile_NotTriggered(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
			Trigger:     false,
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhasePending,
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kcc", Namespace: "default"},
	})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should not requeue when not triggered")
}

func TestReconcile_CompletedPhaseNoOp(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
			Trigger:     true,
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseComplete,
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kcc", Namespace: "default"},
	})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Completed KCC should not requeue")
}

func TestReconcile_FailedPhaseNoOp(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
			Trigger:     true,
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseFailed,
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-kcc", Namespace: "default"},
	})
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Failed KCC should not requeue")
}

// --- handlePendingPhase tests ---

func TestHandlePendingPhase_NoPod(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
			Trigger:     true,
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhasePending,
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.handlePendingPhase(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when no pod found")
}

func TestHandlePendingPhase_PodNotRunning(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
			Trigger:     true,
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhasePending,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"internal.serving.kserve.io/kernelcachecapture": "test-kcc",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	r := newTestReconciler(kcc, pod)

	result, err := r.handlePendingPhase(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when pod is not running")
}

func TestHandlePendingPhase_ContainerNotReady(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
			Trigger:     true,
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhasePending,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"internal.serving.kserve.io/kernelcachecapture": "test-kcc",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "cache-capture",
					Ready: false,
				},
			},
		},
	}

	r := newTestReconciler(kcc, pod)

	result, err := r.handlePendingPhase(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when container not ready")
}

func TestHandlePendingPhase_TransitionsToCapturing(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CachePreset: "vllm",
			Trigger:     true,
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhasePending,
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"internal.serving.kserve.io/kernelcachecapture": "test-kcc",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "cache-capture",
					Ready: true,
				},
			},
		},
	}

	r := newTestReconciler(kcc, pod)

	result, err := r.handlePendingPhase(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should not requeue after transitioning to Capturing")

	// Verify status updated
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.Equal(t, v1alpha1.KernelCacheCapturePhaseCapturing, updated.Status.Phase)
	assert.Equal(t, "test-pod", updated.Status.PodName)
}

// --- createKernelCacheIfEnabled tests ---

func TestCreateKernelCacheIfEnabled_Disabled(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CreateKernelCache: &v1alpha1.CreateKernelCacheConfig{
				Enabled: ptr.To(false),
			},
		},
	}

	r := newTestReconciler(kcc)

	err := r.createKernelCacheIfEnabled(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)

	// Verify no KC was created
	kc := &v1alpha1.KernelCache{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, kc)
	assert.True(t, apierr.IsNotFound(err), "KC should not be created when disabled")
}

func TestCreateKernelCacheIfEnabled_NilConfig(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage:       "registry/cache:v1",
			CreateKernelCache: nil,
		},
	}

	r := newTestReconciler(kcc)

	err := r.createKernelCacheIfEnabled(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)

	// Verify no KC was created
	kc := &v1alpha1.KernelCache{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, kc)
	assert.True(t, apierr.IsNotFound(err), "KC should not be created with nil config")
}

func TestCreateKernelCacheIfEnabled_DefaultsEnabled(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
			UID:       "kcc-uid-123",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CreateKernelCache: &v1alpha1.CreateKernelCacheConfig{
				Enabled: ptr.To(true),
			},
		},
	}

	r := newTestReconciler(kcc)

	err := r.createKernelCacheIfEnabled(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)

	// Verify KC was created
	kc := &v1alpha1.KernelCache{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, kc)
	assert.NoError(t, err)
	assert.Equal(t, "registry/cache:v1", kc.Spec.Image)
	assert.Equal(t, v1alpha1.KernelCacheMountTypeImageVolume, kc.Spec.MountType)

	// Verify owner reference
	assert.Len(t, kc.OwnerReferences, 1)
	assert.Equal(t, "test-kcc", kc.OwnerReferences[0].Name)

	// Verify KCC status updated with KC reference
	assert.NotNil(t, kcc.Status.KernelCacheRef)
	assert.Equal(t, "test-kcc", kcc.Status.KernelCacheRef.Name)
}

func TestCreateKernelCacheIfEnabled_CustomName(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
			UID:       "kcc-uid-123",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CreateKernelCache: &v1alpha1.CreateKernelCacheConfig{
				Enabled: ptr.To(true),
				Name:    "custom-kc-name",
			},
		},
	}

	r := newTestReconciler(kcc)

	err := r.createKernelCacheIfEnabled(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)

	// Verify KC was created with custom name
	kc := &v1alpha1.KernelCache{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "custom-kc-name", Namespace: "default"}, kc)
	assert.NoError(t, err)
	assert.Equal(t, "custom-kc-name", kc.Name)
}

func TestCreateKernelCacheIfEnabled_AlreadyExistsOwned(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
			UID:       "kcc-uid-123",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CreateKernelCache: &v1alpha1.CreateKernelCacheConfig{
				Enabled: ptr.To(true),
			},
		},
	}

	existingKC := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					UID: "kcc-uid-123",
				},
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: "registry/cache:v1",
		},
	}

	r := newTestReconciler(kcc, existingKC)

	err := r.createKernelCacheIfEnabled(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err, "Should succeed when KC already exists and is owned by this KCC")
	assert.NotNil(t, kcc.Status.KernelCacheRef)
}

func TestCreateKernelCacheIfEnabled_AlreadyExistsNotOwned(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
			UID:       "kcc-uid-123",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CreateKernelCache: &v1alpha1.CreateKernelCacheConfig{
				Enabled: ptr.To(true),
			},
		},
	}

	existingKC := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					UID: "different-owner-uid",
				},
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image: "registry/other-cache:v1",
		},
	}

	r := newTestReconciler(kcc, existingKC)

	err := r.createKernelCacheIfEnabled(context.Background(), kcc, logr.Discard())
	assert.Error(t, err, "Should error when KC exists but is not owned by this KCC")
	assert.Contains(t, err.Error(), "not owned by this KCC")
}

func TestCreateKernelCacheIfEnabled_CustomMountType(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kcc",
			Namespace: "default",
			UID:       "kcc-uid-123",
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			TargetImage: "registry/cache:v1",
			CreateKernelCache: &v1alpha1.CreateKernelCacheConfig{
				Enabled:   ptr.To(true),
				MountType: v1alpha1.KernelCacheMountTypePVC,
			},
		},
	}

	r := newTestReconciler(kcc)

	err := r.createKernelCacheIfEnabled(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)

	kc := &v1alpha1.KernelCache{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, kc)
	assert.NoError(t, err)
	assert.Equal(t, v1alpha1.KernelCacheMountTypePVC, kc.Spec.MountType)
}

// --- handleDeletion tests ---

func TestHandleDeletion_NoFinalizer(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{},
		},
	}

	r := newTestReconciler(kcc)

	// Simulate calling handleDeletion on a KCC without our finalizer
	result, err := r.handleDeletion(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestHandleDeletion_NoKCRef(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			KernelCacheRef: nil,
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.handleDeletion(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer removed
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, KCCFinalizerName)
}

func TestHandleDeletion_KCAlreadyDeleted(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			Finalizers: []string{KCCFinalizerName},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			KernelCacheRef: &v1alpha1.NamespacedName{
				Name:      "deleted-kc",
				Namespace: "default",
			},
		},
	}

	r := newTestReconciler(kcc)

	result, err := r.handleDeletion(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer removed
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, KCCFinalizerName)
}

func TestHandleDeletion_KCNotOwnedByKCC(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			UID:        "kcc-uid-123",
			Finalizers: []string{KCCFinalizerName},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			KernelCacheRef: &v1alpha1.NamespacedName{
				Name:      "existing-kc",
				Namespace: "default",
			},
		},
	}

	kc := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-kc",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{UID: "different-uid"},
			},
		},
	}

	r := newTestReconciler(kcc, kc)

	result, err := r.handleDeletion(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer removed (KC not owned by us, safe to delete)
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, KCCFinalizerName)
}

func TestHandleDeletion_KCInUse_BlocksDeletion(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			UID:        "kcc-uid-123",
			Finalizers: []string{KCCFinalizerName},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			KernelCacheRef: &v1alpha1.NamespacedName{
				Name:      "my-kc",
				Namespace: "default",
			},
		},
	}

	kc := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-kc",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{UID: "kcc-uid-123"},
			},
		},
	}

	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-isvc",
			Namespace: "default",
			Labels: map[string]string{
				constants.KernelCacheLabel: "my-kc",
			},
		},
	}

	r := newTestReconciler(kcc, kc, isvc)

	result, err := r.handleDeletion(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when KC is in use")

	// Verify finalizer still present (deletion blocked)
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.Contains(t, updated.Finalizers, KCCFinalizerName)
}

func TestHandleDeletion_KCNotInUse_AllowsDeletion(t *testing.T) {
	kcc := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-kcc",
			Namespace:  "default",
			UID:        "kcc-uid-123",
			Finalizers: []string{KCCFinalizerName},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			KernelCacheRef: &v1alpha1.NamespacedName{
				Name:      "my-kc",
				Namespace: "default",
			},
		},
	}

	kc := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-kc",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{UID: "kcc-uid-123"},
			},
		},
	}

	// No ISVC using this KC
	r := newTestReconciler(kcc, kc)

	result, err := r.handleDeletion(context.Background(), kcc, logr.Discard())
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer removed
	updated := &v1alpha1.KernelCacheCapture{}
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-kcc", Namespace: "default"}, updated)
	assert.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, KCCFinalizerName)
}
