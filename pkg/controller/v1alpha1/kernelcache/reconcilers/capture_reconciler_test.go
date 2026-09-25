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
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	cacheidentity "github.com/kserve/kserve/pkg/kernelcache/identity"
	kernelcachetypes "github.com/kserve/kserve/pkg/kernelcache/types"
)

func TestCapturePodPredicate(t *testing.T) {
	pred := capturePodPredicate()
	relevant := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{constants.InferenceServicePodLabelKey: "model"},
	}}
	unrelated := &corev1.Pod{}

	if !pred.Create(event.CreateEvent{Object: relevant}) {
		t.Fatal("expected an InferenceService Pod to pass the predicate")
	}
	if pred.Create(event.CreateEvent{Object: unrelated}) {
		t.Fatal("did not expect an unrelated Pod to pass the predicate")
	}
	if !pred.Update(event.UpdateEvent{ObjectOld: relevant, ObjectNew: relevant.DeepCopy()}) {
		t.Fatal("expected an InferenceService Pod update to pass the predicate")
	}
	if pred.Update(event.UpdateEvent{ObjectOld: relevant, ObjectNew: unrelated}) {
		t.Fatal("did not expect an update whose new Pod has no InferenceService label to pass the predicate")
	}
	if pred.Generic(event.GenericEvent{Object: unrelated}) {
		t.Fatal("did not expect an unrelated generic event to pass the predicate")
	}
}

func newCaptureClientWithStatusConflict(
	scheme *runtime.Scheme,
	capture *v1alpha1.KernelCacheCapture,
	onConflict func(context.Context, client.Client, *v1alpha1.KernelCacheCapture) error,
) client.WithWatch {
	conflicted := false
	return fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(capture).WithStatusSubresource(capture).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, object client.Object, options ...client.SubResourceUpdateOption) error {
				if subResource != "status" || conflicted {
					return c.SubResource(subResource).Update(ctx, object, options...)
				}
				conflicted = true
				current := &v1alpha1.KernelCacheCapture{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(object), current); err != nil {
					return err
				}
				if err := onConflict(ctx, c, current); err != nil {
					return err
				}
				return apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.SchemeGroupVersion.Group, Resource: "kernelcachecaptures"}, object.GetName(), nil)
			},
		}).Build()
}

func TestCompleteCaptureStatusIncludesPodRuntimeConfigFactors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	modelHash := "sha256:" + strings.Repeat("a", 64)
	runtimeInfo, err := json.Marshal(map[string]string{
		runtimeInfoModelURIHashKey: modelHash,
		runtimeInfoCommandHashKey:  cacheidentity.HashStrings([]string{"vllm", "--dtype", "bfloat16"}),
		runtimeInfoArgsHashKey:     cacheidentity.HashStrings([]string{"--max-model-len=4096"}),
	})
	require.NoError(t, err)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-pod",
			Namespace: "team",
			Labels:    map[string]string{constants.InferenceServicePodLabelKey: "model"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    "kserve-container",
			Image:   "registry.example/vllm@sha256:" + strings.Repeat("b", 64),
			Command: []string{"vllm", "--dtype", "bfloat16"},
			Args:    []string{"--max-model-len=4096"},
			Env: []corev1.EnvVar{
				{Name: "VLLM_USE_AOT_COMPILE", Value: "true"},
				{Name: "VLLM_CACHE_ROOT", Value: "/runtime/vllm"},
			},
		}}},
	}
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			SourceRef: v1alpha1.KernelCacheSourceRef{Kind: "InferenceService", Name: "model"},
			CachePaths: []v1alpha1.KernelCachePath{{
				ContainerName: "kserve-container", OCIPath: "io.vllm.cache",
			}},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{RuntimeResult: map[string]string{
			runtimeResultImageReferenceKey: "registry.example/cache@sha256:" + strings.Repeat("c", 64),
			runtimeResultRuntimeInfoKey:    string(runtimeInfo),
			runtimeResultSourcePodNameKey:  pod.Name,
		}},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: client}
	status := v1alpha1.KernelCacheCaptureStatus{}
	err = reconciler.completeCaptureStatus(t.Context(), &status, capture)
	require.NoError(t, err)
	require.NotNil(t, status.Artifact)
	require.Equal(t, "/runtime/vllm", status.Artifact.CachePaths[0].ContainerPath)
	require.Equal(t, "bfloat16", status.Artifact.Identity.Factors[cacheidentity.DTypeFactor])
	require.Equal(t, "4096", status.Artifact.Identity.Factors[cacheidentity.MaxModelLenFactor])
	require.Equal(t, "true", status.Artifact.Identity.Factors["env.VLLM_USE_AOT_COMPILE"])
}

func TestGeneratedCapturePodRequiresRevisionLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "model-pod",
		Namespace: "team",
		Labels:    map[string]string{constants.InferenceServicePodLabelKey: "model"},
	}}
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-kcc-abc123",
			Namespace: "team",
			Labels:    map[string]string{constants.KernelCacheCaptureGeneratedLabelKey: "true"},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{SourceRef: v1alpha1.KernelCacheSourceRef{
			Kind: "InferenceService", Name: "model",
		}},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.capturePod(t.Context(), capture, map[string]string{
		runtimeResultSourcePodNameKey: pod.Name,
	})
	require.EqualError(t, err, "generated capture Pod revision label is required")
}

func TestSuccessfulCaptureWithMissingProducerFailsWithoutArtifact(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	modelHash := "sha256:" + strings.Repeat("a", 64)
	runtimeInfo, err := json.Marshal(map[string]string{runtimeInfoModelURIHashKey: modelHash})
	require.NoError(t, err)
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Spec: v1alpha1.KernelCacheCaptureSpec{
			SourceRef:  v1alpha1.KernelCacheSourceRef{Kind: "InferenceService", Name: "model"},
			CachePaths: []v1alpha1.KernelCachePath{{ContainerName: "kserve-container", ContainerPath: "/runtime/vllm", OCIPath: "io.vllm.cache"}},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "missing-pod"},
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            runtimeResultSucceededState,
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "missing-pod",
				runtimeResultImageReferenceKey:   "registry.example/cache@sha256:" + strings.Repeat("b", 64),
				runtimeResultRuntimeInfoKey:      string(runtimeInfo),
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(capture).WithStatusSubresource(capture).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err = reconciler.reconcileRuntimeResult(t.Context(), capture)
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseFailed, updated.Status.Phase)
	require.Nil(t, updated.Status.Artifact)
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, kernelCacheCaptureReasonProducerGone, condition.Reason)
}

func TestReopenProducerGoneCaptureClearsStaleState(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	capturedSize := int64(42)
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "capture",
			Namespace: "team",
			Labels:    map[string]string{constants.KernelCacheCaptureGeneratedLabelKey: "true"},
		},
		Status: v1alpha1.KernelCacheCaptureStatus{
			ActiveSession:          &v1alpha1.KernelCacheCaptureSession{ID: "old-session", PodName: "old-pod"},
			RuntimeResult:          map[string]string{runtimeResultStateKey: runtimeResultSucceededState},
			Phase:                  v1alpha1.KernelCacheCapturePhaseFailed,
			CapturedCacheSizeBytes: &capturedSize,
			Conditions: []metav1.Condition{{
				Type:   kernelCacheCaptureReadyConditionType,
				Status: metav1.ConditionFalse,
				Reason: kernelCacheCaptureReasonProducerGone,
			}},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(capture).WithStatusSubresource(capture).Build()

	reopened, err := reopenProducerGoneCapture(t.Context(), k8sClient, k8sClient, client.ObjectKeyFromObject(capture))
	require.NoError(t, err)
	require.NotNil(t, reopened)
	require.Equal(t, v1alpha1.KernelCacheCapturePhasePending, reopened.Status.Phase)
	require.Nil(t, reopened.Status.ActiveSession)
	require.Nil(t, reopened.Status.RuntimeResult)
	require.Nil(t, reopened.Status.Artifact)
	require.Nil(t, reopened.Status.KernelCacheRef)
	require.Nil(t, reopened.Status.Signing)
	require.Nil(t, reopened.Status.CapturedAt)
	require.Nil(t, reopened.Status.CapturedCacheSizeBytes)
	condition := meta.FindStatusCondition(reopened.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, kernelCacheCaptureReasonRetryingProducerGone, condition.Reason)
}

func TestKernelCacheCaptureReconcilerDoesNotCreateDefaultCapture(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))

	inferenceService := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "team", UID: "isvc-uid"},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true}`},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(inferenceService, configMap).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.NoError(t, err)

	captures := &v1alpha1.KernelCacheCaptureList{}
	require.NoError(t, k8sClient.List(t.Context(), captures, client.InNamespace(inferenceService.Namespace)))
	require.Empty(t, captures.Items)
}

func TestCaptureRuntimeResultSetsWaitingForWorkloadPhase(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Status: v1alpha1.KernelCacheCaptureStatus{
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod"},
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            runtimeResultWaitingForWorkloadState,
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "model-pod",
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(capture).WithStatusSubresource(capture).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient}

	_, err := reconciler.reconcileRuntimeResult(t.Context(), capture)
	require.NoError(t, err)
	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseWaitingForWorkload, updated.Status.Phase)
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, metav1.ConditionFalse, condition.Status)
	require.Equal(t, kernelCacheCaptureReasonWaitingForWorkload, condition.Reason)
}

func TestUnknownRuntimeResultFailsCapture(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Status: v1alpha1.KernelCacheCaptureStatus{
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod"},
			Phase:         v1alpha1.KernelCacheCapturePhaseCapturing,
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            "Unknown",
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "model-pod",
			},
			Artifact:       &v1alpha1.KernelCacheArtifact{ImageReference: "registry.example/cache:stale"},
			KernelCacheRef: &v1alpha1.NamespacedName{Namespace: "team", Name: "stale-cache"},
			Signing:        &v1alpha1.KernelCacheSigningStatus{},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(capture).WithStatusSubresource(capture).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient}

	_, err := reconciler.reconcileRuntimeResult(t.Context(), capture)
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseFailed, updated.Status.Phase)
	require.Nil(t, updated.Status.Artifact)
	require.Nil(t, updated.Status.KernelCacheRef)
	require.Nil(t, updated.Status.Signing)
	require.Equal(t, "Unknown", updated.Status.RuntimeResult[runtimeResultStateKey])
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, metav1.ConditionFalse, condition.Status)
	require.Equal(t, kernelCacheCaptureReasonInvalidResult, condition.Reason)
	require.Equal(t, `unsupported runtime result state "Unknown"`, condition.Message)
}

func TestUnknownRuntimeResultWithMissingSourcePodFailsCapture(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseCapturing,
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            "Unknown",
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "missing-pod",
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(capture).WithStatusSubresource(capture).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient}

	_, err := reconciler.reconcileRuntimeResult(t.Context(), capture)
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseFailed, updated.Status.Phase)
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, kernelCacheCaptureReasonInvalidResult, condition.Reason)
}

func TestTerminalCaptureIgnoresLateRuntimeResult(t *testing.T) {
	for _, phase := range []v1alpha1.KernelCacheCapturePhase{
		v1alpha1.KernelCacheCapturePhaseComplete,
		v1alpha1.KernelCacheCapturePhaseUnchanged,
		v1alpha1.KernelCacheCapturePhaseFailed,
	} {
		t.Run(string(phase), func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			capture := &v1alpha1.KernelCacheCapture{
				ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
				Status: v1alpha1.KernelCacheCaptureStatus{
					Phase:         phase,
					ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod"},
					RuntimeResult: map[string]string{
						runtimeResultStateKey:            runtimeResultWaitingForWorkloadState,
						runtimeResultCaptureSessionIDKey: "session",
						runtimeResultSourcePodNameKey:    "model-pod",
					},
					Conditions: []metav1.Condition{{
						Type:   kernelCacheCaptureReadyConditionType,
						Status: metav1.ConditionFalse,
						Reason: kernelCacheCaptureReasonProducerGone,
					}},
				},
			}
			before := capture.Status.DeepCopy()
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(capture).WithStatusSubresource(capture).Build()
			reconciler := &KernelCacheCaptureReconciler{Client: k8sClient}

			_, err := reconciler.reconcileRuntimeResult(t.Context(), capture)
			require.NoError(t, err)

			updated := &v1alpha1.KernelCacheCapture{}
			require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
			require.Equal(t, *before, updated.Status)
		})
	}
}

func TestCaptureRuntimeResultRetriesAfterSessionBackfill(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Status: v1alpha1.KernelCacheCaptureStatus{
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod"},
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            runtimeResultWaitingForWorkloadState,
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "model-pod",
			},
		},
	}
	k8sClient := newCaptureClientWithStatusConflict(scheme, capture, func(ctx context.Context, c client.Client, current *v1alpha1.KernelCacheCapture) error {
		current.Status.ActiveSession.NodeName = "gpu-node"
		return c.Status().Update(ctx, current)
	})
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.reconcileRuntimeResult(t.Context(), capture)
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, "gpu-node", updated.Status.ActiveSession.NodeName)
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseWaitingForWorkload, updated.Status.Phase)
}

func TestCaptureRuntimeResultIgnoresSupersededSession(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Status: v1alpha1.KernelCacheCaptureStatus{
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session-a", PodName: "pod-a"},
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            runtimeResultWaitingForWorkloadState,
				runtimeResultCaptureSessionIDKey: "session-a",
				runtimeResultSourcePodNameKey:    "pod-a",
			},
		},
	}
	k8sClient := newCaptureClientWithStatusConflict(scheme, capture, func(ctx context.Context, c client.Client, current *v1alpha1.KernelCacheCapture) error {
		current.Status.ActiveSession = &v1alpha1.KernelCacheCaptureSession{ID: "session-b", PodName: "pod-b"}
		current.Status.RuntimeResult = nil
		return c.Status().Update(ctx, current)
	})
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.reconcileRuntimeResult(t.Context(), capture)
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, "session-b", updated.Status.ActiveSession.ID)
	require.Nil(t, updated.Status.RuntimeResult)
	require.Equal(t, v1alpha1.KernelCacheCapturePhasePending, updated.Status.Phase)
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, kernelCacheCaptureReasonPending, condition.Reason)
}

func TestCaptureRuntimeResultHandlesDeletedCaptureDuringRetry(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Status: v1alpha1.KernelCacheCaptureStatus{
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod"},
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            runtimeResultWaitingForWorkloadState,
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "model-pod",
			},
		},
	}
	k8sClient := newCaptureClientWithStatusConflict(scheme, capture, func(ctx context.Context, c client.Client, current *v1alpha1.KernelCacheCapture) error {
		return c.Delete(ctx, current)
	})
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	current, err := reconciler.reconcileRuntimeResult(t.Context(), capture)
	require.NoError(t, err)
	require.Nil(t, current)
}

func TestKernelCacheCaptureReconcilerProcessesEveryCaptureForInferenceService(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))

	inferenceService := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "team", UID: "isvc-uid"},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true}`},
	}
	first := completedUnsignedCapture("model-capture-first", inferenceService)
	second := completedUnsignedCapture("model-capture-second", inferenceService)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inferenceService, configMap, first, second).
		WithStatusSubresource(first, second).
		Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.NoError(t, err)

	for _, capture := range []*v1alpha1.KernelCacheCapture{first, second} {
		updated := &v1alpha1.KernelCacheCapture{}
		require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
		require.NotNil(t, updated.Status.Signing, "capture %s was not processed", capture.Name)
		require.Equal(t, v1alpha1.KernelCacheArtifactSecurityStateSkipped, updated.Status.Signing.State)
	}
}

func TestGeneratedUnchangedCaptureIsDeletedWhenProducerPodIsGone(t *testing.T) {
	inferenceService, configMap, capture := unchangedCaptureFixture()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inferenceService, configMap, capture).
		WithStatusSubresource(capture).
		Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.NoError(t, err)

	err = k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), &v1alpha1.KernelCacheCapture{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestGeneratedCaptureDeleteDoesNotDeleteReplacement(t *testing.T) {
	inferenceService, configMap, capture := unchangedCaptureFixture()
	capture.UID = types.UID("capture-old")
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))

	replaced := false
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inferenceService, configMap, capture).
		WithStatusSubresource(capture).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, object client.Object, options ...client.DeleteOption) error {
				current := &v1alpha1.KernelCacheCapture{}
				require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(object), current))
				replacement := current.DeepCopy()
				replacement.UID = types.UID("capture-replacement")
				replacement.ResourceVersion = ""
				require.NoError(t, c.Delete(ctx, current))
				require.NoError(t, c.Create(ctx, replacement))
				replaced = true

				deleteOptions := client.DeleteOptions{}
				deleteOptions.ApplyOptions(options)
				if deleteOptions.Preconditions == nil || deleteOptions.Preconditions.UID == nil ||
					*deleteOptions.Preconditions.UID != current.UID {
					return c.Delete(ctx, replacement)
				}
				return apierrors.NewConflict(schema.GroupResource{Group: v1alpha1.SchemeGroupVersion.Group, Resource: "kernelcachecaptures"}, object.GetName(), nil)
			},
		}).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.True(t, replaced)
	require.Error(t, err)
	require.True(t, apierrors.IsConflict(err))

	remaining := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), remaining))
	require.Equal(t, types.UID("capture-replacement"), remaining.UID)
}

func TestGeneratedUnchangedCaptureIsRetainedWhileProducerPodIsLive(t *testing.T) {
	inferenceService, configMap, capture := unchangedCaptureFixture()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-pod",
			Namespace: inferenceService.Namespace,
			Labels: map[string]string{
				constants.InferenceServicePodLabelKey: inferenceService.Name,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inferenceService, configMap, capture, pod).
		WithStatusSubresource(capture).
		Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseUnchanged, updated.Status.Phase)
	updatedPod := &corev1.Pod{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(pod), updatedPod))
	require.Empty(t, updatedPod.Annotations)
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, metav1.ConditionTrue, condition.Status)
	require.Equal(t, kernelCacheCaptureReasonUnchanged, condition.Reason)
}

func TestCompletedCaptureDoesNotPatchProducerPod(t *testing.T) {
	inferenceService := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "team", UID: "isvc-uid"},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true}`},
	}
	capture := completedUnsignedCapture("model-capture", inferenceService)
	capture.Status.ActiveSession = &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod"}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-pod",
			Namespace: inferenceService.Namespace,
			Labels:    map[string]string{constants.InferenceServicePodLabelKey: inferenceService.Name},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inferenceService, configMap, capture, pod).
		WithStatusSubresource(capture).
		Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.NoError(t, err)

	updatedPod := &corev1.Pod{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(pod), updatedPod))
	require.Empty(t, updatedPod.Annotations)
}

func TestGeneratedInProgressCaptureHandlesMissingProducerByPolicy(t *testing.T) {
	for _, test := range []struct {
		name       string
		policy     string
		wantDelete bool
	}{
		{name: "retains by default"},
		{name: "deletes when configured", policy: "delete", wantDelete: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			inferenceService, configMap, capture := unchangedCaptureFixture()
			capture.Status.RuntimeResult = nil
			capture.Status.Phase = v1alpha1.KernelCacheCapturePhaseCapturing
			if test.policy != "" {
				configMap.Data["kernelcache"] = `{"enabled":true,"abandonedCapturePolicy":"` + test.policy + `"}`
			}
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			require.NoError(t, v1beta1.AddToScheme(scheme))
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(inferenceService, configMap, capture).
				WithStatusSubresource(capture).
				Build()
			reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

			_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
			require.NoError(t, err)

			updated := &v1alpha1.KernelCacheCapture{}
			err = k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated)
			if test.wantDelete {
				require.True(t, apierrors.IsNotFound(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, v1alpha1.KernelCacheCapturePhaseFailed, updated.Status.Phase)
			condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
			require.NotNil(t, condition)
			require.Equal(t, kernelCacheCaptureReasonProducerGone, condition.Reason)
		})
	}
}

func TestUserManagedUnchangedCaptureIsRetained(t *testing.T) {
	inferenceService, configMap, capture := unchangedCaptureFixture()
	capture.Labels = nil
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inferenceService, configMap, capture).
		WithStatusSubresource(capture).
		Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseUnchanged, updated.Status.Phase)
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, metav1.ConditionTrue, condition.Status)
	require.Equal(t, kernelCacheCaptureReasonUnchanged, condition.Reason)
}

func TestGeneratedFailedCaptureIsRetained(t *testing.T) {
	inferenceService, configMap, capture := unchangedCaptureFixture()
	capture.Status.RuntimeResult[runtimeResultStateKey] = runtimeResultFailedState
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inferenceService, configMap, capture).
		WithStatusSubresource(capture).
		Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient, Reader: k8sClient}

	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(inferenceService)})
	require.NoError(t, err)

	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.Equal(t, v1alpha1.KernelCacheCapturePhaseFailed, updated.Status.Phase)
	condition := meta.FindStatusCondition(updated.Status.Conditions, kernelCacheCaptureReadyConditionType)
	require.NotNil(t, condition)
	require.Equal(t, kernelCacheCaptureReasonFailed, condition.Reason)
}

func unchangedCaptureFixture() (*v1beta1.InferenceService, *corev1.ConfigMap, *v1alpha1.KernelCacheCapture) {
	inferenceService := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "team", UID: "isvc-uid"},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true}`},
	}
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-kcc-669889f77f",
			Namespace: inferenceService.Namespace,
			Labels:    map[string]string{constants.KernelCacheCaptureGeneratedLabelKey: "true"},
		},
		Spec: v1alpha1.KernelCacheCaptureSpec{SourceRef: v1alpha1.KernelCacheSourceRef{
			Kind: "InferenceService", Name: inferenceService.Name,
		}},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase:         v1alpha1.KernelCacheCapturePhaseCapturing,
			ActiveSession: &v1alpha1.KernelCacheCaptureSession{ID: "session", PodName: "model-pod"},
			RuntimeResult: map[string]string{
				runtimeResultStateKey:            runtimeResultUnchangedState,
				runtimeResultCaptureSessionIDKey: "session",
				runtimeResultSourcePodNameKey:    "model-pod",
			},
		},
	}
	return inferenceService, configMap, capture
}

func completedUnsignedCapture(name string, inferenceService *v1beta1.InferenceService) *v1alpha1.KernelCacheCapture {
	image := "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: inferenceService.Namespace},
		Spec: v1alpha1.KernelCacheCaptureSpec{SourceRef: v1alpha1.KernelCacheSourceRef{
			Kind: "InferenceService", Name: inferenceService.Name,
		}},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseComplete,
			RuntimeResult: map[string]string{
				runtimeResultStateKey:          runtimeResultSucceededState,
				runtimeResultImageReferenceKey: image,
			},
			Artifact: &v1alpha1.KernelCacheArtifact{ImageReference: image},
			Conditions: []metav1.Condition{{
				Type: kernelCacheCaptureReadyConditionType, Status: metav1.ConditionTrue,
			}},
		},
	}
}

func TestCaptureSigningIsSkippedInNoneMode(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	image := "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "capture", Namespace: "team"},
		Status: v1alpha1.KernelCacheCaptureStatus{
			Phase: v1alpha1.KernelCacheCapturePhaseComplete,
			RuntimeResult: map[string]string{
				runtimeResultStateKey:          runtimeResultSucceededState,
				runtimeResultImageReferenceKey: image,
			},
			Artifact: &v1alpha1.KernelCacheArtifact{ImageReference: image},
			Conditions: []metav1.Condition{{
				Type:   kernelCacheCaptureReadyConditionType,
				Status: metav1.ConditionTrue,
			}},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(capture).WithStatusSubresource(capture).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient}
	config := &v1beta1.KernelCacheConfig{ArtifactSecurity: v1beta1.KernelCacheArtifactSecurityConfig{Mode: string(kernelcachetypes.ModeDisabled)}}

	require.NoError(t, reconciler.reconcileArtifactSigning(t.Context(), capture, config))
	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.NotNil(t, updated.Status.Signing)
	require.Equal(t, v1alpha1.KernelCacheArtifactSecurityStateSkipped, updated.Status.Signing.State)
	require.True(t, isCaptureComplete(updated))
}

func TestCaptureSigningRejectsMissingProfileReference(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	inferenceService := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "team"},
	}
	capture := completedUnsignedCapture("capture", inferenceService)
	capture.Spec.Signing = &v1alpha1.KernelCacheSigningSpec{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(capture).WithStatusSubresource(capture).Build()
	reconciler := &KernelCacheCaptureReconciler{Client: k8sClient}
	config := &v1beta1.KernelCacheConfig{
		ArtifactSecurity: v1beta1.KernelCacheArtifactSecurityConfig{Mode: string(kernelcachetypes.ModeCert)},
	}

	require.NoError(t, reconciler.reconcileArtifactSigning(t.Context(), capture, config))
	updated := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
	require.NotNil(t, updated.Status.Signing)
	require.Equal(t, "SigningProfileNotFound", updated.Status.Signing.Reason)
}
