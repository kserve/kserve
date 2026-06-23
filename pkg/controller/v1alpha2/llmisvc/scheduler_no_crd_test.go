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

package llmisvc

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// errV1Alpha2PoolNoMatch simulates the error returned when the v1alpha2 InferencePool CRD is not installed.
var errV1Alpha2PoolNoMatch = &meta.NoKindMatchError{
	GroupKind: schema.GroupKind{
		Group: igwapiv1alpha2.SchemeGroupVersion.Group,
		Kind:  "InferencePool",
	},
	SearchedVersions: []string{igwapiv1alpha2.SchemeGroupVersion.Version},
}

// newFakeClientWithV1Alpha2PoolNoMatch creates a fake client that returns NoKindMatchError
// for v1alpha2 InferencePool operations while allowing all other operations to proceed normally.
// It seeds the inferenceservice-config ConfigMap with a valid ingress/storage/credentials
// config and the given scheduler JSON.
func newFakeClientWithV1Alpha2PoolNoMatch(t *testing.T, schedulerConfigJSON string) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}
	if err := igwapi.Install(scheme); err != nil {
		t.Fatalf("failed to add igwapi to scheme: %v", err)
	}
	if err := igwapiv1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add igwapiv1alpha2 to scheme: %v", err)
	}

	// Seed the inferenceservice-config ConfigMap with valid data so loadConfig() succeeds.
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: constants.KServeNamespace,
		},
		Data: map[string]string{
			"ingress": `{
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
			}`,
			"storageInitializer": `{
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1"
			}`,
			"credentials": `{}`,
		},
	}
	if schedulerConfigJSON != "" {
		configMap.Data["scheduler"] = schedulerConfigJSON
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(configMap).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*igwapiv1alpha2.InferencePool); ok {
					return errV1Alpha2PoolNoMatch
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*igwapiv1alpha2.InferencePool); ok {
					return errV1Alpha2PoolNoMatch
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()
}

func newLLMInferenceServiceWithScheduler(name, namespace string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "main",
								Image: "quay.io/test/epp:latest",
								Ports: []corev1.ContainerPort{
									{Name: "grpc", ContainerPort: 9002},
								},
							},
						},
					},
					Pool: v1alpha2.InferencePoolSpec{},
				},
			},
		},
	}
}

func TestReconcileV1Alpha2InferencePool_WhenCRDNotInstalled_FlagEnabled_ShouldNotFail(t *testing.T) {
	// This test verifies that reconcileV1Alpha2InferencePool gracefully skips
	// when the v1alpha2 InferencePool CRD is not installed AND the
	// AllowSkippingV1Alpha2InferencePool feature flag is enabled.

	// given - feature flag enabled
	fakeClient := newFakeClientWithV1Alpha2PoolNoMatch(t, `{"allowSkippingV1Alpha2InferencePool": true}`)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	llmSvc := newLLMInferenceServiceWithScheduler("test-llm", "default")

	// when
	err := reconciler.reconcileV1Alpha2InferencePool(t.Context(), llmSvc, false)
	// then - should succeed without error (flag enabled, CRD absent → skip)
	if err != nil {
		t.Errorf("reconcileV1Alpha2InferencePool should not fail when flag is enabled and v1alpha2 CRD is missing, got: %v", err)
	}
}

func TestReconcileV1Alpha2InferencePool_WhenCRDNotInstalled_FlagDisabled_ShouldFail(t *testing.T) {
	// This test verifies that reconcileV1Alpha2InferencePool returns an error
	// when the v1alpha2 InferencePool CRD is not installed AND the feature flag
	// is disabled (default). This preserves backward-compatible behavior.

	// given - feature flag disabled (default)
	fakeClient := newFakeClientWithV1Alpha2PoolNoMatch(t, `{}`)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	llmSvc := newLLMInferenceServiceWithScheduler("test-llm", "default")

	// when
	err := reconciler.reconcileV1Alpha2InferencePool(t.Context(), llmSvc, false)
	// then - should fail (flag disabled → original error behavior)
	if err == nil {
		t.Error("reconcileV1Alpha2InferencePool should fail when flag is disabled and v1alpha2 CRD is missing")
	}
	if !meta.IsNoMatchError(err) {
		t.Errorf("expected NoKindMatchError, got: %v", err)
	}
}

func TestReconcileV1Alpha2InferencePool_WhenCRDNotInstalled_FlagEnabled_DeleteShouldNotFail(t *testing.T) {
	// This test verifies that the delete path in reconcileV1Alpha2InferencePool
	// also handles the absent CRD when the feature flag is enabled.

	// given - feature flag enabled
	fakeClient := newFakeClientWithV1Alpha2PoolNoMatch(t, `{"allowSkippingV1Alpha2InferencePool": true}`)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	llmSvc := newLLMInferenceServiceWithScheduler("test-llm", "default")

	// when - shouldDelete = true
	err := reconciler.reconcileV1Alpha2InferencePool(t.Context(), llmSvc, true)
	// then - should succeed without error
	if err != nil {
		t.Errorf("reconcileV1Alpha2InferencePool (delete) should not fail when flag is enabled and v1alpha2 CRD is missing, got: %v", err)
	}
}

func TestReconcileSchedulerInferencePool_WhenV1Alpha2CRDNotInstalled_FlagEnabled_ShouldNotFail(t *testing.T) {
	// This test verifies the full reconcileSchedulerInferencePool flow succeeds
	// when only the v1 CRD is available, v1alpha2 is absent, and the feature flag is enabled.
	// This is the exact scenario reported in issue #5708.

	// given - v1alpha2 CRD is missing, v1 CRD is available, feature flag enabled
	fakeClient := newFakeClientWithV1Alpha2PoolNoMatch(t, `{"allowSkippingV1Alpha2InferencePool": true}`)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	llmSvc := newLLMInferenceServiceWithScheduler("test-llm", "default")

	// when
	err := reconciler.reconcileSchedulerInferencePool(t.Context(), llmSvc)
	// then - should succeed: v1 pool created, v1alpha2 pool skipped
	if err != nil {
		t.Errorf("reconcileSchedulerInferencePool should not fail when flag is enabled and v1alpha2 CRD is missing, got: %v", err)
	}
}

func TestReconcileSchedulerInferencePool_WhenV1Alpha2CRDNotInstalled_FlagDisabled_ShouldFail(t *testing.T) {
	// This test verifies the full reconcileSchedulerInferencePool flow fails
	// when v1alpha2 CRD is absent and the feature flag is disabled (default).

	// given - v1alpha2 CRD is missing, feature flag disabled
	fakeClient := newFakeClientWithV1Alpha2PoolNoMatch(t, `{}`)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	llmSvc := newLLMInferenceServiceWithScheduler("test-llm", "default")

	// when
	err := reconciler.reconcileSchedulerInferencePool(t.Context(), llmSvc)
	// then - should fail (flag disabled → v1alpha2 error propagated)
	if err == nil {
		t.Error("reconcileSchedulerInferencePool should fail when flag is disabled and v1alpha2 CRD is missing")
	}
}
