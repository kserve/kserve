/*
Copyright 2024 The KServe Authors.

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

package aigateway

import (
	"context"
	"testing"

	aigwv1a1 "github.com/envoyproxy/ai-gateway/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestNewAIServiceBackendReconciler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}
	logger := log.Log.WithName("test")

	reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)

	assert.NotNil(t, reconciler)
	assert.Equal(t, fakeClient, reconciler.client)
	assert.Equal(t, ingressConfig, reconciler.ingressConfig)
	assert.Equal(t, logger, reconciler.log)
}

func TestCreateAIServiceBackend(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	t.Run("predictor only inference service", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"test-label": "test-value",
				},
				Annotations: map[string]string{
					"test-annotation": "test-value",
				},
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{
							Name: "sklearn",
						},
					},
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}
		expectedResult := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
				Labels: map[string]string{
					"test-label":                             "test-value",
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
				Annotations: map[string]string{
					"test-annotation": "test-value",
				},
			},
			Spec: aigwv1a1.AIServiceBackendSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				BackendRef: gwapiv1.BackendObjectReference{
					Kind:      ptr.To(gwapiv1.Kind(constants.KindService)),
					Name:      gwapiv1.ObjectName("test-isvc-predictor"),
					Namespace: ptr.To(gwapiv1.Namespace("test-namespace")),
					Port:      ptr.To(gwapiv1.PortNumber(constants.CommonDefaultHttpPort)),
				},
				Timeouts: &gwapiv1.HTTPRouteTimeouts{
					Request: ptr.To(gwapiv1.Duration("60s")),
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := log.Log.WithName("test")

		reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)
		result := reconciler.createAIServiceBackend(isvc)

		if diff := cmp.Diff(expectedResult, result); diff != "" {
			t.Errorf("createAIServiceBackend() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("inference service with transformer", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-transformer",
				Namespace: "test-namespace",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{
							Name: "sklearn",
						},
					},
				},
				Transformer: &v1beta1.TransformerSpec{
					PodSpec: v1beta1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "transformer",
								Image: "transformer:latest",
							},
						},
					},
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "custom-gateway/custom-ingress-gateway",
		}
		expectedResult := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-transformer",
				Namespace: "custom-gateway",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc-transformer",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
			},
			Spec: aigwv1a1.AIServiceBackendSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				BackendRef: gwapiv1.BackendObjectReference{
					Kind:      ptr.To(gwapiv1.Kind(constants.KindService)),
					Name:      gwapiv1.ObjectName("test-isvc-transformer-transformer"),
					Namespace: ptr.To(gwapiv1.Namespace("test-namespace")),
					Port:      ptr.To(gwapiv1.PortNumber(constants.CommonDefaultHttpPort)),
				},
				Timeouts: &gwapiv1.HTTPRouteTimeouts{
					Request: ptr.To(gwapiv1.Duration("60s")),
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := log.Log.WithName("test")

		reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)
		result := reconciler.createAIServiceBackend(isvc)

		if diff := cmp.Diff(expectedResult, result); diff != "" {
			t.Errorf("createAIServiceBackend() mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestAIServiceBackendSemanticEquals(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ingressConfig := &v1beta1.IngressConfig{}
	logger := log.Log.WithName("test")
	reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)

	baseBackend := &aigwv1a1.AIServiceBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backend",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
		Spec: aigwv1a1.AIServiceBackendSpec{
			APISchema: aigwv1a1.VersionedAPISchema{
				Name: aigwv1a1.APISchemaOpenAI,
			},
			BackendRef: gwapiv1.BackendObjectReference{
				Kind:      ptr.To(gwapiv1.Kind("Service")),
				Name:      gwapiv1.ObjectName("test-service"),
				Namespace: ptr.To(gwapiv1.Namespace("test-namespace")),
				Port:      ptr.To(gwapiv1.PortNumber(80)),
			},
		},
	}

	t.Run("identical backends", func(t *testing.T) {
		desired := baseBackend.DeepCopy()
		existing := baseBackend.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		assert.True(t, result)
	})

	t.Run("different spec", func(t *testing.T) {
		desired := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.Spec.BackendRef.Port = ptr.To(gwapiv1.PortNumber(8080))
			return backend
		}()
		existing := baseBackend.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		assert.False(t, result)
	})

	t.Run("different labels", func(t *testing.T) {
		desired := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.Labels["new-label"] = "new-value"
			return backend
		}()
		existing := baseBackend.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		assert.False(t, result)
	})

	t.Run("different annotations", func(t *testing.T) {
		desired := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.Annotations["new-annotation"] = "new-value"
			return backend
		}()
		existing := baseBackend.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		assert.False(t, result)
	})

	t.Run("different resource version should be equal", func(t *testing.T) {
		desired := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.ResourceVersion = "123"
			return backend
		}()
		existing := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.ResourceVersion = "456"
			return backend
		}()
		result := reconciler.SemanticEquals(desired, existing)
		assert.True(t, result)
	})
}

func TestGetAIServiceBackendName(t *testing.T) {
	t.Run("simple inference service name", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-model",
			},
		}
		result := getAIServiceBackendName(isvc)
		assert.Equal(t, "my-model", result)
	})

	t.Run("inference service with complex name", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-complex-model-v2",
			},
		}
		result := getAIServiceBackendName(isvc)
		assert.Equal(t, "my-complex-model-v2", result)
	})
}

func TestAIServiceBackendReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("create new backend", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{
							Name: "sklearn",
						},
					},
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
		fakeClient := clientBuilder.Build()

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		assert.NoError(t, err)

		// Verify the backend exists in the cluster
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		assert.NoError(t, err)

		// Verify ownership labels are set
		assert.Equal(t, isvc.Name, backend.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, backend.Labels[constants.InferenceServiceNamespaceLabel])

		// Verify spec is correct
		assert.Equal(t, aigwv1a1.APISchemaOpenAI, backend.Spec.APISchema.Name)
		assert.Equal(t, constants.PredictorServiceName(isvc.Name), string(backend.Spec.BackendRef.Name))
		assert.Equal(t, isvc.Namespace, string(*backend.Spec.BackendRef.Namespace))
	})

	t.Run("update existing backend", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"updated-label": "updated-value",
				},
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{
							Name: "sklearn",
						},
					},
				},
			},
		}
		existingBackend := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-isvc",
				Namespace:       "kserve-gateway",
				ResourceVersion: "123",
				Labels: map[string]string{
					"old-label": "old-value",
				},
			},
			Spec: aigwv1a1.AIServiceBackendSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				BackendRef: gwapiv1.BackendObjectReference{
					Kind:      ptr.To(gwapiv1.Kind("Service")),
					Name:      gwapiv1.ObjectName("test-isvc-predictor"),
					Namespace: ptr.To(gwapiv1.Namespace("test-namespace")),
					Port:      ptr.To(gwapiv1.PortNumber(80)),
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
		clientBuilder = clientBuilder.WithObjects(existingBackend)
		fakeClient := clientBuilder.Build()

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		assert.NoError(t, err)

		// Verify the backend exists in the cluster
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		assert.NoError(t, err)

		// Verify ownership labels are set
		assert.Equal(t, isvc.Name, backend.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, backend.Labels[constants.InferenceServiceNamespaceLabel])

		// Verify spec is correct
		assert.Equal(t, aigwv1a1.APISchemaOpenAI, backend.Spec.APISchema.Name)
		assert.Equal(t, constants.PredictorServiceName(isvc.Name), string(backend.Spec.BackendRef.Name))
		assert.Equal(t, isvc.Namespace, string(*backend.Spec.BackendRef.Namespace))
	})

	t.Run("no update needed", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{
							Name: "sklearn",
						},
					},
				},
			},
		}
		existingBackend := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-isvc",
				Namespace:       "kserve-gateway",
				ResourceVersion: "123",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
			},
			Spec: aigwv1a1.AIServiceBackendSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				BackendRef: gwapiv1.BackendObjectReference{
					Kind:      ptr.To(gwapiv1.Kind("Service")),
					Name:      gwapiv1.ObjectName("test-isvc-predictor"),
					Namespace: ptr.To(gwapiv1.Namespace("test-namespace")),
					Port:      ptr.To(gwapiv1.PortNumber(80)),
				},
				Timeouts: &gwapiv1.HTTPRouteTimeouts{
					Request: ptr.To(gwapiv1.Duration("60s")),
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
		clientBuilder = clientBuilder.WithObjects(existingBackend)
		fakeClient := clientBuilder.Build()

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		assert.NoError(t, err)

		// Verify the backend exists in the cluster
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		assert.NoError(t, err)

		// Verify ownership labels are set
		assert.Equal(t, isvc.Name, backend.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, backend.Labels[constants.InferenceServiceNamespaceLabel])

		// Verify spec is correct
		assert.Equal(t, aigwv1a1.APISchemaOpenAI, backend.Spec.APISchema.Name)
		assert.Equal(t, constants.PredictorServiceName(isvc.Name), string(backend.Spec.BackendRef.Name))
		assert.Equal(t, isvc.Namespace, string(*backend.Spec.BackendRef.Namespace))
	})
}

func TestDeleteAIServiceBackend(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("delete existing backend", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
			},
		}
		existingBackend := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
		clientBuilder = clientBuilder.WithObjects(existingBackend)
		fakeClient := clientBuilder.Build()

		logger := log.Log.WithName("test")

		err := DeleteAIServiceBackend(ctx, fakeClient, ingressConfig, isvc, logger)
		assert.NoError(t, err)

		// Verify the backend is deleted
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		assert.True(t, apierr.IsNotFound(err), "Backend should be deleted")
	})
}
