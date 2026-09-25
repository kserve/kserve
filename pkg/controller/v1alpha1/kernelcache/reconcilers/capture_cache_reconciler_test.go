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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestCaptureRejectsKernelCacheWithDifferentImage(t *testing.T) {
	capture := &v1alpha1.KernelCacheCapture{}
	capture.Status.Artifact = &v1alpha1.KernelCacheArtifact{ImageReference: "registry.example.com/cache@sha256:aaa"}
	cache := &v1alpha1.KernelCache{}
	cache.Spec.Artifact.ImageReference = "registry.example.com/cache@sha256:bbb"
	r := &KernelCacheReconciler{}
	if err := r.setCaptureKernelCacheRef(context.Background(), capture, cache); err == nil {
		t.Fatal("expected mismatched images to be rejected")
	}
	if capture.Status.KernelCacheRef != nil {
		t.Fatal("mismatched cache must not be linked")
	}
}

func TestCaptureSigningAllowsCache(t *testing.T) {
	tests := []struct {
		name    string
		status  *v1alpha1.KernelCacheSigningStatus
		allowed bool
	}{
		{name: "missing status"},
		{name: "none skipped", status: &v1alpha1.KernelCacheSigningStatus{Mode: "none", State: v1alpha1.KernelCacheArtifactSecurityStateSkipped}, allowed: true},
		{name: "cert succeeded", status: &v1alpha1.KernelCacheSigningStatus{Mode: "cert", State: v1alpha1.KernelCacheArtifactSecurityStateSucceeded, Signed: true}, allowed: true},
		{name: "cert failed", status: &v1alpha1.KernelCacheSigningStatus{Mode: "cert", State: v1alpha1.KernelCacheArtifactSecurityStateFailed}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := captureSigningAllowsCache(test.status); got != test.allowed {
				t.Fatalf("expected %t, got %t", test.allowed, got)
			}
		})
	}
}

func TestUnchangedCaptureKeepsExistingKernelCache(t *testing.T) {
	capture := &v1alpha1.KernelCacheCapture{
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseComplete,
			RuntimeResult: map[string]string{
				runtimeResultStateKey: runtimeResultUnchangedState,
			},
			Artifact:       &v1alpha1.KernelCacheArtifact{ImageReference: "registry.example.com/cache@sha256:aaa"},
			KernelCacheRef: &v1alpha1.NamespacedName{Namespace: "team", Name: "cache"},
			Signing: &v1alpha1.KernelCacheSigningStatus{
				Mode:  "none",
				State: v1alpha1.KernelCacheArtifactSecurityStateSkipped,
			},
			Conditions: []metav1.Condition{{Type: kernelCacheCaptureReadyConditionType, Status: metav1.ConditionTrue}},
		},
	}

	if !isCaptureComplete(capture) {
		t.Fatal("unchanged capture with an existing cache must remain complete")
	}
	capture.Status.KernelCacheRef = nil
	if isCaptureComplete(capture) {
		t.Fatal("unchanged capture without an existing cache must not be complete")
	}
}

func TestCaptureCompletionRequiresCurrentGeneration(t *testing.T) {
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseComplete,
			RuntimeResult: map[string]string{
				runtimeResultStateKey:          runtimeResultSucceededState,
				runtimeResultImageReferenceKey: "registry.example.com/cache@sha256:aaa",
			},
			Artifact: &v1alpha1.KernelCacheArtifact{ImageReference: "registry.example.com/cache@sha256:aaa"},
			Signing: &v1alpha1.KernelCacheSigningStatus{
				Mode:  "none",
				State: v1alpha1.KernelCacheArtifactSecurityStateSkipped,
			},
			Conditions: []metav1.Condition{{
				Type:               kernelCacheCaptureReadyConditionType,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
			}},
		},
	}

	require.False(t, isCaptureComplete(capture))
	capture.Status.Conditions[0].ObservedGeneration = capture.Generation
	require.True(t, isCaptureComplete(capture))
}

func TestKernelCacheReconcilerCreatesKernelCacheFromCompletedCapture(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "qwen-kernelcache-capture",
			Namespace: "team",
			UID:       types.UID("capture-uid"),
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			SourceRef: v1alpha1.KernelCacheSourceRef{
				Kind: "InferenceService",
				Name: "qwen",
			},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseComplete,
			RuntimeResult: map[string]string{
				runtimeResultStateKey: runtimeResultSucceededState,
			},
			Artifact: &v1alpha1.KernelCacheArtifact{
				ImageReference: "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				CachePaths: []v1alpha1.KernelCachePath{{
					ContainerName: "kserve-container",
					ContainerPath: "/tmp/vllm",
					OCIPath:       "io.vllm.cache",
				}},
				Identity: v1alpha1.KernelCacheIdentity{
					Footprints: v1alpha1.KernelCacheFootprints{
						WorkloadFootprint:      "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						CompatibilityFootprint: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					},
				},
			},
			Signing: &v1alpha1.KernelCacheSigningStatus{
				Mode:  "none",
				State: v1alpha1.KernelCacheArtifactSecurityStateSkipped,
			},
		},
	}
	capture.Status.RuntimeResult[runtimeResultImageReferenceKey] = capture.Status.Artifact.ImageReference
	capture.Status.RuntimeResult[runtimeResultCaptureSessionIDKey] = "session"
	capture.Status.RuntimeResult[runtimeResultSourcePodNameKey] = "model-pod"
	capture.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}
	capture.Status.ActiveSession = &v1alpha1.KernelCacheCaptureSession{
		ID: "session", PodName: "model-pod", RequestedNodeGroup: "annotation-group",
	}
	secondCapture := capture.DeepCopy()
	secondCapture.Name = "qwen-kernelcache-capture-second"
	secondCapture.UID = types.UID("capture-uid-second")
	secondCapture.Status.Artifact.ImageReference = "registry.example.com/cache@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	secondCapture.Status.RuntimeResult[runtimeResultImageReferenceKey] = secondCapture.Status.Artifact.ImageReference
	secondCapture.Status.KernelCacheRef = nil
	inferenceService := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "qwen",
			Namespace: "team",
		},
	}
	annotationGroup := &v1alpha1.KernelCacheNodeGroup{ObjectMeta: metav1.ObjectMeta{Name: "annotation-group"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}}}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			"kernelcache": `{"enabled":true,"defaultMountType":"oci","defaultNodeGroup":"config-group"}`,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(capture, secondCapture, inferenceService, configMap, annotationGroup).
		WithStatusSubresource(capture, secondCapture).
		Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}
	request := ctrl.Request{NamespacedName: client.ObjectKey{Namespace: "team", Name: generatedKernelCacheName(capture.Name, capture.Status.Artifact.ImageReference)}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	kernelCache := &v1alpha1.KernelCache{}
	if err := k8sClient.Get(context.Background(), request.NamespacedName, kernelCache); err != nil {
		t.Fatal(err)
	}
	if kernelCache.Spec.NodeGroupRef.Name != "annotation-group" {
		t.Fatalf("expected annotation node group, got %q", kernelCache.Spec.NodeGroupRef.Name)
	}
	if kernelCache.Annotations[constants.KernelCacheNodeGroupSelectionSourceAnnotationKey] != "annotation" {
		t.Fatalf("expected annotation selection source, got %q", kernelCache.Annotations[constants.KernelCacheNodeGroupSelectionSourceAnnotationKey])
	}
	if kernelCache.Spec.MountType != v1alpha1.KernelCacheMountTypeOCI {
		t.Fatalf("expected oci mount type, got %q", kernelCache.Spec.MountType)
	}
	if kernelCache.Spec.Artifact.ImageReference != capture.Status.Artifact.ImageReference {
		t.Fatalf("expected artifact image %q, got %q", capture.Status.Artifact.ImageReference, kernelCache.Spec.Artifact.ImageReference)
	}
	require.Equal(t, capture.Status.Artifact.CachePaths, kernelCache.Spec.Artifact.CachePaths)

	updatedCapture := &v1alpha1.KernelCacheCapture{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(capture), updatedCapture); err != nil {
		t.Fatal(err)
	}
	if updatedCapture.Status.KernelCacheRef == nil || updatedCapture.Status.KernelCacheRef.Name != kernelCache.Name {
		t.Fatalf("expected KernelCacheRef to point to %q, got %#v", kernelCache.Name, updatedCapture.Status.KernelCacheRef)
	}

	// A result from another capture must create a new cache without changing the first capture.
	requests := reconciler.enqueueKCForCompletedKCC(context.Background(), secondCapture)
	if len(requests) != 1 || requests[0].Name == kernelCache.Name {
		t.Fatal("new artifact must enqueue a different cache")
	}
	if _, err := reconciler.Reconcile(context.Background(), requests[0]); err != nil {
		t.Fatal(err)
	}
	updatedSecondCapture := &v1alpha1.KernelCacheCapture{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(secondCapture), updatedSecondCapture); err != nil {
		t.Fatal(err)
	}
	if updatedSecondCapture.Status.KernelCacheRef == nil || updatedSecondCapture.Status.KernelCacheRef.Name != requests[0].Name {
		t.Fatal("second capture must reference the new cache")
	}
	if err := reconciler.reconcileCaptureKernelCacheRef(context.Background(), kernelCache); err != nil {
		t.Fatalf("old cache reconciliation must remain independent: %v", err)
	}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(capture), updatedCapture); err != nil {
		t.Fatal(err)
	}
	if updatedCapture.Status.KernelCacheRef == nil || updatedCapture.Status.KernelCacheRef.Name != kernelCache.Name {
		t.Fatal("first capture reference must remain unchanged")
	}
	if err := reconciler.reconcileCaptureKernelCache(context.Background(), requests[0]); err != nil {
		t.Fatal(err)
	}
	caches := &v1alpha1.KernelCacheList{}
	if err := k8sClient.List(context.Background(), caches); err != nil {
		t.Fatal(err)
	}
	if len(caches.Items) != 2 {
		t.Fatalf("expected two retained caches, got %d", len(caches.Items))
	}
}

func TestKernelCacheReconcilerAutomaticallySelectsUniqueNodeGroup(t *testing.T) {
	reconciler, k8sClient, capture, request := newCaptureNodeGroupHarness(t,
		"gpu-node",
		"",
		"fallback",
		[]v1alpha1.KernelCacheNodeGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "gpu"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "cpu"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "cpu"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "fallback"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "other"}}},
		},
		map[string]string{"role": "gpu"},
	)

	_, err := reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	cache := &v1alpha1.KernelCache{}
	require.NoError(t, k8sClient.Get(t.Context(), request.NamespacedName, cache))
	require.Equal(t, "gpu", cache.Spec.NodeGroupRef.Name)
	require.Equal(t, "auto", cache.Annotations[constants.KernelCacheNodeGroupSelectionSourceAnnotationKey])
	require.NotNil(t, capture.Status.Artifact)
}

func TestCaptureMountTypeDefaultsToOCI(t *testing.T) {
	mountType, err := captureMountType(&v1beta1.KernelCacheConfig{})
	require.NoError(t, err)
	require.Equal(t, v1alpha1.KernelCacheMountTypeOCI, mountType)

	_, err = captureMountType(&v1beta1.KernelCacheConfig{DefaultMountType: "pvc"})
	require.EqualError(t, err, `invalid kernelcache.defaultMountType "pvc"`)
}

func TestKernelCacheReconcilerUsesDefaultWhenNoNodeGroupMatches(t *testing.T) {
	reconciler, k8sClient, _, request := newCaptureNodeGroupHarness(t,
		"gpu-node",
		"",
		"fallback",
		[]v1alpha1.KernelCacheNodeGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "fallback"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "other"}}},
		},
		map[string]string{"role": "gpu"},
	)

	_, err := reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	cache := &v1alpha1.KernelCache{}
	require.NoError(t, k8sClient.Get(t.Context(), request.NamespacedName, cache))
	require.Equal(t, "fallback", cache.Spec.NodeGroupRef.Name)
	require.Equal(t, "default", cache.Annotations[constants.KernelCacheNodeGroupSelectionSourceAnnotationKey])
}

func TestKernelCacheReconcilerWaitsForProducerNodeAssignment(t *testing.T) {
	reconciler, k8sClient, capture, request := newCaptureNodeGroupHarness(t,
		"",
		"",
		"fallback",
		[]v1alpha1.KernelCacheNodeGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "fallback"}},
		},
		nil,
	)
	recorder := events.NewFakeRecorder(1)
	reconciler.Recorder = recorder

	_, err := reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	err = k8sClient.Get(t.Context(), request.NamespacedName, &v1alpha1.KernelCache{})
	require.True(t, apierrors.IsNotFound(err))
	require.Nil(t, capture.Status.KernelCacheRef)
	event := <-recorder.Events
	require.True(t, strings.Contains(event, "ProducerNodePending"), event)
}

func TestKernelCacheReconcilerSkipsAmbiguousNodeGroups(t *testing.T) {
	reconciler, k8sClient, capture, request := newCaptureNodeGroupHarness(t,
		"gpu-node",
		"",
		"fallback",
		[]v1alpha1.KernelCacheNodeGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "gpu-a"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "gpu-b"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"zone": "east"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "fallback"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "other"}}},
		},
		map[string]string{"role": "gpu", "zone": "east"},
	)
	recorder := events.NewFakeRecorder(1)
	reconciler.Recorder = recorder

	result, err := reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	require.Zero(t, result.RequeueAfter)
	err = k8sClient.Get(t.Context(), request.NamespacedName, &v1alpha1.KernelCache{})
	require.True(t, apierrors.IsNotFound(err))
	updatedCapture := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updatedCapture))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseComplete, updatedCapture.Status.Phase)
	event := <-recorder.Events
	require.True(t, strings.Contains(event, "AmbiguousNodeGroup"), event)
	require.True(t, strings.Contains(event, "gpu-a, gpu-b"), event)
}

func TestKernelCacheReconcilerDoesNotFallbackForInvalidRequestedNodeGroup(t *testing.T) {
	reconciler, k8sClient, _, request := newCaptureNodeGroupHarness(t,
		"gpu-node",
		"missing",
		"fallback",
		[]v1alpha1.KernelCacheNodeGroup{
			{ObjectMeta: metav1.ObjectMeta{Name: "fallback"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}}},
		},
		map[string]string{"role": "gpu"},
	)
	recorder := events.NewFakeRecorder(1)
	reconciler.Recorder = recorder

	_, err := reconciler.Reconcile(t.Context(), request)
	require.NoError(t, err)
	err = k8sClient.Get(t.Context(), request.NamespacedName, &v1alpha1.KernelCache{})
	require.True(t, apierrors.IsNotFound(err))
	event := <-recorder.Events
	require.True(t, strings.Contains(event, "InvalidRequestedNodeGroup"), event)
}

func TestNodeGroupChangeRequeuesUnboundCompletedCapture(t *testing.T) {
	reconciler, _, capture, request := newCaptureNodeGroupHarness(t,
		"gpu-node",
		"",
		"",
		nil,
		map[string]string{"role": "gpu"},
	)

	requests := reconciler.enqueueKCsOnNodeGroupChange(t.Context(), &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
	})
	require.Contains(t, requests, request)
	require.Nil(t, capture.Status.KernelCacheRef)
}

func TestNodeChangeRequeuesCaptureAssignedToThatNode(t *testing.T) {
	reconciler, _, _, request := newCaptureNodeGroupHarness(t,
		"gpu-node",
		"",
		"",
		nil,
		map[string]string{"role": "gpu"},
	)

	requests := reconciler.enqueueKCsOnNodeChange(t.Context(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-node"},
	})
	require.Contains(t, requests, request)

	requests = reconciler.enqueueKCsOnNodeChange(t.Context(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "other-node"},
	})
	require.NotContains(t, requests, request)
}

func newCaptureNodeGroupHarness(
	t *testing.T,
	nodeName string,
	requestedNodeGroup string,
	defaultNodeGroup string,
	groups []v1alpha1.KernelCacheNodeGroup,
	nodeLabels map[string]string,
) (*KernelCacheReconciler, client.Client, *v1alpha1.KernelCacheCapture, ctrl.Request) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	image := "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "model-capture", Namespace: "team"},
		Spec:       v1alpha1.KernelCacheCaptureSpec{SourceRef: v1alpha1.KernelCacheSourceRef{Kind: "InferenceService", Name: "model"}},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase:         v1alpha1.KernelCacheCapturePhaseComplete,
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod", NodeName: nodeName, RequestedNodeGroup: requestedNodeGroup},
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            runtimeResultSucceededState,
				runtimeResultImageReferenceKey:   image,
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "model-pod",
			},
			Artifact: &v1alpha1.KernelCacheArtifact{
				ImageReference: image,
				CachePaths: []v1alpha1.KernelCachePath{{
					ContainerName: "kserve-container", ContainerPath: "/tmp/vllm", OCIPath: "io.vllm.cache",
				}},
			},
			Signing:    &v1alpha1.KernelCacheSigningStatus{Mode: "none", State: v1alpha1.KernelCacheArtifactSecurityStateSkipped},
			Conditions: []metav1.Condition{{Type: kernelCacheCaptureReadyConditionType, Status: metav1.ConditionTrue}},
		},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true,"defaultMountType":"oci","defaultNodeGroup":"` + defaultNodeGroup + `"}`},
	}
	objects := []client.Object{capture, configMap}
	if nodeName != "" {
		objects = append(objects, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: nodeLabels}})
	}
	for index := range groups {
		objects = append(objects, &groups[index])
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(capture).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient, Reader: k8sClient}
	request := ctrl.Request{NamespacedName: client.ObjectKey{Namespace: capture.Namespace, Name: generatedKernelCacheName(capture.Name, image)}}
	return reconciler, k8sClient, capture, request
}
