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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	isvcutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/utils"
	"github.com/kserve/kserve/pkg/utils"
)

func TestNewAIGatewayRouteReconciler(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}

	reconciler := NewAIGatewayRouteReconciler(fakeClient, scheme, ingressConfig)

	require.NotNil(t, reconciler)
	require.Equal(t, fakeClient, reconciler.Client)
	require.Equal(t, scheme, reconciler.scheme)
	require.Equal(t, ingressConfig, reconciler.ingressConfig)
	require.NotNil(t, reconciler.log)
}

func TestCreateAIGatewayRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}
	reconciler := NewAIGatewayRouteReconciler(fakeClient, scheme, ingressConfig)

	t.Run("predictor only", func(t *testing.T) {
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
							Name: "pytorch",
						},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI: ptr.To("s3://bucket/model"),
						},
					},
				},
			},
		}

		route := reconciler.createAIGatewayRoute(isvc)
		require.NotNil(t, route)

		// Create expected route for full comparison
		expectedRoute := &aigwv1a1.AIGatewayRoute{
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
			Spec: aigwv1a1.AIGatewayRouteSpec{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: gwapiv1a2.GroupName,
							Kind:  constants.KindGateway,
							Name:  gwapiv1a2.ObjectName("kserve-ingress-gateway"),
						},
					},
				},
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				Rules: []aigwv1a1.AIGatewayRouteRule{
					{
						Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
							{
								Headers: []gwapiv1.HTTPHeaderMatch{
									{
										Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
										Value: "test-isvc",
									},
								},
							},
						},
						BackendRefs: []aigwv1a1.AIGatewayRouteRuleBackendRef{
							{
								Name:   "test-isvc",
								Weight: 1,
							},
						},
					},
				},
				LLMRequestCosts: []aigwv1a1.LLMRequestCost{
					{
						MetadataKey: constants.MetadataKeyInputToken,
						Type:        aigwv1a1.LLMRequestCostTypeInputToken,
					},
					{
						MetadataKey: constants.MetadataKeyOutputToken,
						Type:        aigwv1a1.LLMRequestCostTypeOutputToken,
					},
					{
						MetadataKey: constants.MetadataKeyTotalToken,
						Type:        aigwv1a1.LLMRequestCostTypeTotalToken,
					},
				},
			},
		}

		// Compare full objects
		if diff := cmp.Diff(expectedRoute, route); diff != "" {
			t.Errorf("createAIGatewayRoute() mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestSemanticEquals(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}
	reconciler := NewAIGatewayRouteReconciler(fakeClient, scheme, ingressConfig)

	baseRoute := &aigwv1a1.AIGatewayRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-route",
			Namespace: "kserve-gateway",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
		Spec: aigwv1a1.AIGatewayRouteSpec{
			APISchema: aigwv1a1.VersionedAPISchema{
				Name: aigwv1a1.APISchemaOpenAI,
			},
			Rules: []aigwv1a1.AIGatewayRouteRule{
				{
					Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
						{
							Headers: []gwapiv1.HTTPHeaderMatch{
								{
									Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
									Value: "test-model",
								},
							},
						},
					},
				},
			},
		},
	}

	t.Run("identical routes", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		result := reconciler.SemanticEquals(route1, route2)
		require.True(t, result)
	})

	t.Run("different spec", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.Spec.Rules[0].Matches[0].Headers[0].Value = "different-model"
		result := reconciler.SemanticEquals(route1, route2)
		require.False(t, result)
	})

	t.Run("different labels", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.Labels["different-label"] = "different-value"
		result := reconciler.SemanticEquals(route1, route2)
		require.False(t, result)
	})

	t.Run("different annotations", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.Annotations["different-annotation"] = "different-value"
		result := reconciler.SemanticEquals(route1, route2)
		require.False(t, result)
	})

	t.Run("different resource version (should be equal)", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.ResourceVersion = "12345"
		result := reconciler.SemanticEquals(route1, route2)
		require.True(t, result) // ResourceVersion should not affect semantic equality
	})
}

func TestGetAIGatewayRouteName(t *testing.T) {
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "test-namespace",
		},
	}

	name := getAIGatewayRouteName(isvc)
	require.Equal(t, "test-service", name)
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := t.Context()

	t.Run("create new route", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{
							Name: "pytorch",
						},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI: ptr.To("s3://bucket/model"),
						},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}
		reconciler := NewAIGatewayRouteReconciler(fakeClient, scheme, ingressConfig)

		err := reconciler.Reconcile(ctx, isvc)
		require.NoError(t, err)

		// Verify the route exists
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		require.NoError(t, err)

		// Create expected route for full comparison
		expectedRoute := &aigwv1a1.AIGatewayRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
				Annotations: nil,
			},
			Spec: aigwv1a1.AIGatewayRouteSpec{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: gwapiv1a2.GroupName,
							Kind:  constants.KindGateway,
							Name:  gwapiv1a2.ObjectName("kserve-ingress-gateway"),
						},
					},
				},
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				Rules: []aigwv1a1.AIGatewayRouteRule{
					{
						Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
							{
								Headers: []gwapiv1.HTTPHeaderMatch{
									{
										Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
										Value: "test-isvc",
									},
								},
							},
						},
						BackendRefs: []aigwv1a1.AIGatewayRouteRuleBackendRef{
							{
								Name:   "test-isvc",
								Weight: 1,
							},
						},
					},
				},
				LLMRequestCosts: []aigwv1a1.LLMRequestCost{
					{
						MetadataKey: constants.MetadataKeyInputToken,
						Type:        aigwv1a1.LLMRequestCostTypeInputToken,
					},
					{
						MetadataKey: constants.MetadataKeyOutputToken,
						Type:        aigwv1a1.LLMRequestCostTypeOutputToken,
					},
					{
						MetadataKey: constants.MetadataKeyTotalToken,
						Type:        aigwv1a1.LLMRequestCostTypeTotalToken,
					},
				},
			},
		}

		// Compare full objects, ignoring ResourceVersion
		actualRoute := route.DeepCopy()
		actualRoute.ResourceVersion = ""
		expectedRoute.ResourceVersion = ""
		if diff := cmp.Diff(expectedRoute, actualRoute); diff != "" {
			t.Errorf("Route mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("update existing route", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"new-label": "new-value",
				},
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{
							Name: "pytorch",
						},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI: ptr.To("s3://bucket/model"),
						},
					},
				},
			},
		}

		existingRoute := &aigwv1a1.AIGatewayRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-isvc",
				Namespace:       "kserve-gateway",
				ResourceVersion: "1",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
			},
			Spec: aigwv1a1.AIGatewayRouteSpec{
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				Rules: []aigwv1a1.AIGatewayRouteRule{
					{
						Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
							{
								Headers: []gwapiv1.HTTPHeaderMatch{
									{
										Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
										Value: "test-isvc",
									},
								},
							},
						},
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRoute).Build()
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}
		reconciler := NewAIGatewayRouteReconciler(fakeClient, scheme, ingressConfig)

		err := reconciler.Reconcile(ctx, isvc)
		require.NoError(t, err)

		// Verify the route exists
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		require.NoError(t, err)

		// Create expected route for full comparison
		expectedRoute := &aigwv1a1.AIGatewayRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
				Labels: map[string]string{
					"new-label":                              "new-value",
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
				Annotations: nil,
			},
			Spec: aigwv1a1.AIGatewayRouteSpec{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: gwapiv1a2.GroupName,
							Kind:  constants.KindGateway,
							Name:  gwapiv1a2.ObjectName("kserve-ingress-gateway"),
						},
					},
				},
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				Rules: []aigwv1a1.AIGatewayRouteRule{
					{
						Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
							{
								Headers: []gwapiv1.HTTPHeaderMatch{
									{
										Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
										Value: "test-isvc",
									},
								},
							},
						},
						BackendRefs: []aigwv1a1.AIGatewayRouteRuleBackendRef{
							{
								Name:   "test-isvc",
								Weight: 1,
							},
						},
					},
				},
				LLMRequestCosts: []aigwv1a1.LLMRequestCost{
					{
						MetadataKey: constants.MetadataKeyInputToken,
						Type:        aigwv1a1.LLMRequestCostTypeInputToken,
					},
					{
						MetadataKey: constants.MetadataKeyOutputToken,
						Type:        aigwv1a1.LLMRequestCostTypeOutputToken,
					},
					{
						MetadataKey: constants.MetadataKeyTotalToken,
						Type:        aigwv1a1.LLMRequestCostTypeTotalToken,
					},
				},
			},
		}

		// Compare full objects, ignoring ResourceVersion
		actualRoute := route.DeepCopy()
		actualRoute.ResourceVersion = ""
		expectedRoute.ResourceVersion = ""
		if diff := cmp.Diff(expectedRoute, actualRoute); diff != "" {
			t.Errorf("Route mismatch (-want +got):\n%s", diff)
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
							Name: "pytorch",
						},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI: ptr.To("s3://bucket/model"),
						},
					},
				},
			},
		}

		// Create an exactly matching route
		tempIsvc := &v1beta1.InferenceService{
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
							Name: "pytorch",
						},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI: ptr.To("s3://bucket/model"),
						},
					},
				},
			},
		}

		existingRoute := &aigwv1a1.AIGatewayRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-isvc",
				Namespace:       "kserve-gateway",
				ResourceVersion: "1",
				Labels: utils.Union(tempIsvc.Labels, map[string]string{
					constants.InferenceServiceNameLabel:      tempIsvc.Name,
					constants.InferenceServiceNamespaceLabel: tempIsvc.Namespace,
				}),
				Annotations: tempIsvc.Annotations,
			},
			Spec: aigwv1a1.AIGatewayRouteSpec{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: gwapiv1a2.GroupName,
							Kind:  constants.KindGateway,
							Name:  gwapiv1a2.ObjectName("kserve-ingress-gateway"),
						},
					},
				},
				APISchema: aigwv1a1.VersionedAPISchema{
					Name: aigwv1a1.APISchemaOpenAI,
				},
				Rules: []aigwv1a1.AIGatewayRouteRule{
					{
						Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
							{
								Headers: []gwapiv1.HTTPHeaderMatch{
									{
										Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
										Value: isvcutils.GetModelName(tempIsvc),
									},
								},
							},
						},
						BackendRefs: []aigwv1a1.AIGatewayRouteRuleBackendRef{
							{
								Name:   getAIServiceBackendName(tempIsvc),
								Weight: 1,
							},
						},
					},
				},
				LLMRequestCosts: []aigwv1a1.LLMRequestCost{
					{
						MetadataKey: constants.MetadataKeyInputToken,
						Type:        aigwv1a1.LLMRequestCostTypeInputToken,
					},
					{
						MetadataKey: constants.MetadataKeyOutputToken,
						Type:        aigwv1a1.LLMRequestCostTypeOutputToken,
					},
					{
						MetadataKey: constants.MetadataKeyTotalToken,
						Type:        aigwv1a1.LLMRequestCostTypeTotalToken,
					},
				},
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRoute).Build()
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}
		reconciler := NewAIGatewayRouteReconciler(fakeClient, scheme, ingressConfig)

		err := reconciler.Reconcile(ctx, isvc)
		require.NoError(t, err)

		// Verify the route exists
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		require.NoError(t, err)

		// Since this is a "no update needed" test, the route should match the existing route exactly
		// We verify that the reconciler didn't modify anything by comparing with the initial route
		expectedRoute := existingRoute.DeepCopy()
		actualRoute := route.DeepCopy()

		// Ignore ResourceVersion as it may change during operations
		expectedRoute.ResourceVersion = ""
		actualRoute.ResourceVersion = ""

		if diff := cmp.Diff(expectedRoute, actualRoute); diff != "" {
			t.Errorf("Route should not have changed (-want +got):\n%s", diff)
		}
	})
}

// routeClientInterceptor wraps a fake client to inject errors for testing
type routeClientInterceptor struct {
	client.Client
	createError error
	updateError error
	dryRunError error
	getError    error
}

func (c *routeClientInterceptor) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.createError != nil {
		return c.createError
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *routeClientInterceptor) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
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

func (c *routeClientInterceptor) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.getError != nil {
		return c.getError
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// routeActualUpdateFailingClient is a specialized client that allows dry-run updates but fails regular updates
type routeActualUpdateFailingClient struct {
	client.Client
}

func (c *routeActualUpdateFailingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	// Allow dry-run updates to succeed
	for _, opt := range opts {
		if opt == client.DryRunAll {
			return c.Client.Update(ctx, obj, opts...)
		}
	}
	// Fail regular updates
	return errors.New("actual update failed")
}

func TestReconcileWithErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := t.Context()
	isvc := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-isvc",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.InferenceServiceSpec{
			Predictor: v1beta1.PredictorSpec{
				Model: &v1beta1.ModelSpec{
					ModelFormat: v1beta1.ModelFormat{
						Name: "pytorch",
					},
					PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
						StorageURI: ptr.To("s3://bucket/model"),
					},
				},
			},
		},
	}

	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}

	testCases := []struct {
		name         string
		setupClient  func() client.Client
		expectError  bool
		errorMessage string
	}{
		{
			name: "create_route_with_client_error",
			setupClient: func() client.Client {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				return &routeClientInterceptor{
					Client:      fakeClient,
					createError: errors.New("failed to create route"),
				}
			},
			expectError:  true,
			errorMessage: "failed to create route",
		},
		{
			name: "update_route_with_client_error",
			setupClient: func() client.Client {
				existingRoute := &aigwv1a1.AIGatewayRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-isvc",
						Namespace:       "kserve-gateway",
						ResourceVersion: "1",
						Labels: map[string]string{
							constants.InferenceServiceNameLabel:      "test-isvc",
							constants.InferenceServiceNamespaceLabel: "test-namespace",
						},
					},
					Spec: aigwv1a1.AIGatewayRouteSpec{
						APISchema: aigwv1a1.VersionedAPISchema{
							Name: aigwv1a1.APISchemaOpenAI,
						},
						Rules: []aigwv1a1.AIGatewayRouteRule{
							{
								Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
									{
										Headers: []gwapiv1.HTTPHeaderMatch{
											{
												Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
												Value: "old-model-name", // Different from current to force update
											},
										},
									},
								},
							},
						},
					},
				}
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRoute).Build()
				return &routeClientInterceptor{
					Client:      fakeClient,
					updateError: errors.New("failed to update route"),
				}
			},
			expectError:  true,
			errorMessage: "failed to update route",
		},
		{
			name: "dry_run_update_with_error",
			setupClient: func() client.Client {
				existingRoute := &aigwv1a1.AIGatewayRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-isvc",
						Namespace:       "kserve-gateway",
						ResourceVersion: "1",
						Labels: map[string]string{
							constants.InferenceServiceNameLabel:      "test-isvc",
							constants.InferenceServiceNamespaceLabel: "test-namespace",
						},
					},
					Spec: aigwv1a1.AIGatewayRouteSpec{
						APISchema: aigwv1a1.VersionedAPISchema{
							Name: aigwv1a1.APISchemaOpenAI,
						},
					},
				}
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRoute).Build()
				return &routeClientInterceptor{
					Client:      fakeClient,
					dryRunError: errors.New("dry-run update failed"),
				}
			},
			expectError:  true,
			errorMessage: "dry-run update failed",
		},
		{
			name: "get_route_with_non-not-found_error",
			setupClient: func() client.Client {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				return &routeClientInterceptor{
					Client:   fakeClient,
					getError: errors.New("internal server error"),
				}
			},
			expectError:  true,
			errorMessage: "internal server error",
		},
		{
			name: "actual_update_fails_after_successful_dry-run",
			setupClient: func() client.Client {
				existingRoute := &aigwv1a1.AIGatewayRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-isvc",
						Namespace:       "kserve-gateway",
						ResourceVersion: "1",
						Labels: map[string]string{
							constants.InferenceServiceNameLabel:      "test-isvc",
							constants.InferenceServiceNamespaceLabel: "test-namespace",
						},
					},
					Spec: aigwv1a1.AIGatewayRouteSpec{
						APISchema: aigwv1a1.VersionedAPISchema{
							Name: aigwv1a1.APISchemaOpenAI,
						},
						Rules: []aigwv1a1.AIGatewayRouteRule{
							{
								Matches: []aigwv1a1.AIGatewayRouteRuleMatch{
									{
										Headers: []gwapiv1.HTTPHeaderMatch{
											{
												Name:  gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey),
												Value: "old-model-name", // Different from current to force update
											},
										},
									},
								},
							},
						},
					},
				}
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRoute).Build()
				return &routeActualUpdateFailingClient{
					Client: fakeClient,
				}
			},
			expectError:  true,
			errorMessage: "actual update failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.setupClient()
			reconciler := NewAIGatewayRouteReconciler(client, scheme, ingressConfig)

			err := reconciler.Reconcile(ctx, isvc)

			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteAIGatewayRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := t.Context()

	t.Run("delete existing route", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
			},
		}

		existingRoute := &aigwv1a1.AIGatewayRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
			},
		}

		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingRoute).Build()
		logger := ctrl.Log.WithName("test")

		err := DeleteAIGatewayRoute(ctx, fakeClient, ingressConfig, isvc, logger)
		require.NoError(t, err)

		// Verify the route was deleted
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		require.Error(t, err) // Should be NotFound error
	})

	t.Run("delete non-existing route", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "non-existing-isvc",
				Namespace: "test-namespace",
			},
		}

		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := ctrl.Log.WithName("test")

		err := DeleteAIGatewayRoute(ctx, fakeClient, ingressConfig, isvc, logger)
		require.Error(t, err) // Should return error for non-existing route
	})
}
