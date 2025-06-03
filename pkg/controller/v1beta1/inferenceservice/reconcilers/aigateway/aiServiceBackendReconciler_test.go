/*
Copyright 2025 The KServe Authors.

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
	"errors"
	"testing"

	aigwv1a1 "github.com/envoyproxy/ai-gateway/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// clientInterceptor wraps a client.Client to simulate errors for testing
type clientInterceptor struct {
	client.Client
	deleteError error
	createError error
	updateError error
	dryRunError error
	getError    error
}

func (c *clientInterceptor) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.deleteError != nil {
		return c.deleteError
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *clientInterceptor) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.createError != nil {
		return c.createError
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *clientInterceptor) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// Check if this is a dry-run update
	for _, opt := range opts {
		if opt == client.DryRunAll && c.dryRunError != nil {
			return c.dryRunError
		}
	}
	if c.updateError != nil {
		return c.updateError
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c *clientInterceptor) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.getError != nil {
		return c.getError
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// actualUpdateFailingClient is a specialized client that allows dry-run updates but fails regular updates
type actualUpdateFailingClient struct {
	client.Client
}

func (c *actualUpdateFailingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// Check if this is a dry-run update
	for _, opt := range opts {
		if opt == client.DryRunAll {
			// Allow dry-run updates to succeed
			return c.Client.Update(ctx, obj, opts...)
		}
	}
	// Fail regular updates
	return errors.New("simulated actual update error")
}

func TestNewAIServiceBackendReconciler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}
	logger := log.Log.WithName("test")

	reconciler := NewAIServiceBackendReconciler(fakeClient, ingressConfig, logger)

	require.NotNil(t, reconciler)
	require.Equal(t, fakeClient, reconciler.client)
	require.Equal(t, ingressConfig, reconciler.ingressConfig)
	require.Equal(t, logger, reconciler.log)
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
					Group:     ptr.To(gwapiv1.Group("")),
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

	t.Run("inference service with custom predictor timeout", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-timeout-isvc",
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
					ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
						TimeoutSeconds: ptr.To[int64](120),
					},
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
				Name:      "test-timeout-isvc",
				Namespace: "kserve-gateway",
				Labels: map[string]string{
					"test-label":                             "test-value",
					constants.InferenceServiceNameLabel:      "test-timeout-isvc",
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
					Group:     ptr.To(gwapiv1.Group("")),
					Kind:      ptr.To(gwapiv1.Kind(constants.KindService)),
					Name:      gwapiv1.ObjectName("test-timeout-isvc-predictor"),
					Namespace: ptr.To(gwapiv1.Namespace("test-namespace")),
					Port:      ptr.To(gwapiv1.PortNumber(constants.CommonDefaultHttpPort)),
				},
				Timeouts: &gwapiv1.HTTPRouteTimeouts{
					Request: ptr.To(gwapiv1.Duration("120s")),
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
				Group:     ptr.To(gwapiv1.Group("")),
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
		require.True(t, result)
	})

	t.Run("different spec", func(t *testing.T) {
		desired := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.Spec.BackendRef.Port = ptr.To(gwapiv1.PortNumber(8080))
			return backend
		}()
		existing := baseBackend.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		require.False(t, result)
	})

	t.Run("different labels", func(t *testing.T) {
		desired := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.Labels["new-label"] = "new-value"
			return backend
		}()
		existing := baseBackend.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		require.False(t, result)
	})

	t.Run("different annotations", func(t *testing.T) {
		desired := func() *aigwv1a1.AIServiceBackend {
			backend := baseBackend.DeepCopy()
			backend.Annotations["new-annotation"] = "new-value"
			return backend
		}()
		existing := baseBackend.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		require.False(t, result)
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
		require.True(t, result)
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
		require.Equal(t, "my-model", result)
	})

	t.Run("inference service with complex name", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-complex-model-v2",
			},
		}
		result := getAIServiceBackendName(isvc)
		require.Equal(t, "my-complex-model-v2", result)
	})
}

func TestAIServiceBackendReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := t.Context()

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
		require.NoError(t, err)

		// Verify the backend exists in the cluster
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.NoError(t, err)

		// Create expected backend for full comparison
		expectedBackend := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
				Annotations: nil,
			},
			Spec: aigwv1a1.AIServiceBackendSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				BackendRef: gwapiv1.BackendObjectReference{
					Group:     ptr.To(gwapiv1.Group("")),
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

		// Compare full objects, ignoring ResourceVersion
		actualBackend := backend.DeepCopy()
		actualBackend.ResourceVersion = ""
		expectedBackend.ResourceVersion = ""
		if diff := cmp.Diff(expectedBackend, actualBackend); diff != "" {
			t.Errorf("Backend mismatch (-want +got):\n%s", diff)
		}
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
		require.NoError(t, err)

		// Verify the backend exists in the cluster
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.NoError(t, err)

		// Create expected backend for full comparison
		expectedBackend := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
				Labels: map[string]string{
					"updated-label":                          "updated-value",
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
				Annotations: nil,
			},
			Spec: aigwv1a1.AIServiceBackendSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				BackendRef: gwapiv1.BackendObjectReference{
					Group:     ptr.To(gwapiv1.Group("")),
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

		// Compare full objects, ignoring ResourceVersion
		actualBackend := backend.DeepCopy()
		actualBackend.ResourceVersion = ""
		expectedBackend.ResourceVersion = ""
		if diff := cmp.Diff(expectedBackend, actualBackend); diff != "" {
			t.Errorf("Backend mismatch (-want +got):\n%s", diff)
		}
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
		require.NoError(t, err)

		// Verify the backend exists in the cluster
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.NoError(t, err)

		// Create expected backend for full comparison
		expectedBackend := &aigwv1a1.AIServiceBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
				Annotations: nil,
			},
			Spec: aigwv1a1.AIServiceBackendSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				BackendRef: gwapiv1.BackendObjectReference{
					Group:     ptr.To(gwapiv1.Group("")),
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

		// Compare full objects, ignoring ResourceVersion
		actualBackend := backend.DeepCopy()
		actualBackend.ResourceVersion = ""
		expectedBackend.ResourceVersion = ""
		if diff := cmp.Diff(expectedBackend, actualBackend); diff != "" {
			t.Errorf("Backend mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("create backend with client error", func(t *testing.T) {
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

		// Create a client interceptor that will make create operations fail
		interceptorClient := &clientInterceptor{
			Client:      fakeClient,
			createError: errors.New("simulated create error"),
		}

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(interceptorClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated create error")

		// Verify the backend was not created
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.True(t, apierr.IsNotFound(err), "Backend should not exist after create error")
	})

	t.Run("update backend with client error", func(t *testing.T) {
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

		// Create a client interceptor that will make update operations fail
		interceptorClient := &clientInterceptor{
			Client:      fakeClient,
			updateError: errors.New("simulated update error"),
		}

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(interceptorClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated update error")

		// Verify the backend still exists but wasn't updated
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.NoError(t, err)

		// Verify the old labels are still there (update failed)
		require.Equal(t, "old-value", backend.Labels["old-label"])
		require.NotContains(t, backend.Labels, "updated-label")
	})

	t.Run("dry run update with error", func(t *testing.T) {
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

		// Create a client interceptor that will make dry-run operations fail
		interceptorClient := &clientInterceptor{
			Client:      fakeClient,
			dryRunError: errors.New("simulated dry-run error"),
		}

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(interceptorClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated dry-run error")
	})

	t.Run("get backend with non-not-found error", func(t *testing.T) {
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

		// Create a client interceptor that will make get operations fail with a non-NotFound error
		interceptorClient := &clientInterceptor{
			Client:   fakeClient,
			getError: errors.New("simulated get error - server unavailable"),
		}

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(interceptorClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated get error - server unavailable")

		// Verify no backend was created (because get failed before create could happen)
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.True(t, apierr.IsNotFound(err), "Backend should not exist after get error")
	})

	t.Run("actual update fails after successful dry-run", func(t *testing.T) {
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

		// Create a specialized client interceptor that allows dry-run but fails regular updates
		interceptorClient := &actualUpdateFailingClient{
			Client: fakeClient,
		}

		logger := log.Log.WithName("test")
		reconciler := NewAIServiceBackendReconciler(interceptorClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated actual update error")

		// Verify the backend still exists but wasn't updated
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.NoError(t, err)

		// Verify the old labels are still there (update failed)
		require.Equal(t, "old-value", backend.Labels["old-label"])
		require.NotContains(t, backend.Labels, "updated-label")
	})
}

func TestDeleteAIServiceBackend(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := t.Context()

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
		require.NoError(t, err)

		// Verify the backend is deleted
		backend := &aigwv1a1.AIServiceBackend{}
		key := types.NamespacedName{
			Name:      getAIServiceBackendName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, backend)
		require.True(t, apierr.IsNotFound(err), "Backend should be deleted")
	})

	t.Run("delete backend with client error", func(t *testing.T) {
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

		// Create a client that will return an error on delete operations
		clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
		clientBuilder = clientBuilder.WithObjects(existingBackend)
		fakeClient := clientBuilder.Build()

		// Create a client interceptor that will make delete operations fail
		interceptorClient := &clientInterceptor{
			Client:      fakeClient,
			deleteError: errors.New("simulated delete error"),
		}

		logger := log.Log.WithName("test")

		err := DeleteAIServiceBackend(ctx, interceptorClient, ingressConfig, isvc, logger)
		require.Error(t, err)
		require.Contains(t, err.Error(), "simulated delete error")
	})
}
