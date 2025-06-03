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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	v1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
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

	assert.NotNil(t, reconciler)
	assert.Equal(t, fakeClient, reconciler.Client)
	assert.Equal(t, scheme, reconciler.scheme)
	assert.Equal(t, ingressConfig, reconciler.ingressConfig)
	assert.NotNil(t, reconciler.log)
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

		route, err := reconciler.createAIGatewayRoute(isvc)
		require.NoError(t, err)
		require.NotNil(t, route)

		// Check metadata
		assert.Equal(t, "test-isvc", route.Name)
		assert.Equal(t, "kserve-gateway", route.Namespace)

		// Check ownership tracking labels
		assert.Equal(t, isvc.Name, route.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, route.Labels[constants.InferenceServiceNamespaceLabel])

		// Check that original labels are preserved
		for k, v := range isvc.Labels {
			assert.Equal(t, v, route.Labels[k])
		}

		// Check annotations
		for k, v := range isvc.Annotations {
			assert.Equal(t, v, route.Annotations[k])
		}

		// Check spec
		assert.Len(t, route.Spec.TargetRefs, 1)
		assert.Equal(t, gwapiv1a2.GroupName, string(route.Spec.TargetRefs[0].Group))
		assert.Equal(t, constants.KindGateway, string(route.Spec.TargetRefs[0].Kind))
		assert.Equal(t, "kserve-ingress-gateway", string(route.Spec.TargetRefs[0].Name))

		// Check API schema
		assert.Equal(t, aigwv1a1.APISchemaOpenAI, route.Spec.APISchema.Name)

		// Check rules
		assert.Len(t, route.Spec.Rules, 1)
		rule := route.Spec.Rules[0]

		// Check matches
		assert.Len(t, rule.Matches, 1)
		match := rule.Matches[0]
		assert.Len(t, match.Headers, 1)
		assert.Equal(t, gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey), match.Headers[0].Name)
		assert.Equal(t, "test-isvc", match.Headers[0].Value)

		// Check backend refs
		assert.Len(t, rule.BackendRefs, 1)
		assert.Equal(t, "test-isvc", rule.BackendRefs[0].Name)

		// Check LLM request costs
		assert.Len(t, route.Spec.LLMRequestCosts, 3)
		costTypes := []aigwv1a1.LLMRequestCostType{
			aigwv1a1.LLMRequestCostTypeInputToken,
			aigwv1a1.LLMRequestCostTypeOutputToken,
			aigwv1a1.LLMRequestCostTypeTotalToken,
		}
		for i, cost := range route.Spec.LLMRequestCosts {
			assert.Equal(t, costTypes[i], cost.Type)
		}
	})

	t.Run("predictor with transformer", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-transformer-isvc",
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
				Transformer: &v1beta1.TransformerSpec{
					PodSpec: v1beta1.PodSpec{
						Containers: []corev1.Container{
							{
								Image: "transformer:latest",
							},
						},
					},
				},
			},
		}

		route, err := reconciler.createAIGatewayRoute(isvc)
		require.NoError(t, err)
		require.NotNil(t, route)

		// Check metadata
		assert.Equal(t, "test-transformer-isvc", route.Name)
		assert.Equal(t, "kserve-gateway", route.Namespace)

		// Check ownership tracking labels
		assert.Equal(t, isvc.Name, route.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, route.Labels[constants.InferenceServiceNamespaceLabel])

		// Check spec
		assert.Len(t, route.Spec.TargetRefs, 1)
		assert.Equal(t, gwapiv1a2.GroupName, string(route.Spec.TargetRefs[0].Group))
		assert.Equal(t, constants.KindGateway, string(route.Spec.TargetRefs[0].Kind))
		assert.Equal(t, "kserve-ingress-gateway", string(route.Spec.TargetRefs[0].Name))

		// Check API schema
		assert.Equal(t, aigwv1a1.APISchemaOpenAI, route.Spec.APISchema.Name)

		// Check rules
		assert.Len(t, route.Spec.Rules, 1)
		rule := route.Spec.Rules[0]

		// Check matches
		assert.Len(t, rule.Matches, 1)
		match := rule.Matches[0]
		assert.Len(t, match.Headers, 1)
		assert.Equal(t, gwapiv1.HTTPHeaderName(aigwv1a1.AIModelHeaderKey), match.Headers[0].Name)
		assert.Equal(t, "test-transformer-isvc", match.Headers[0].Value)

		// Check backend refs
		assert.Len(t, rule.BackendRefs, 1)
		assert.Equal(t, "test-transformer-isvc", rule.BackendRefs[0].Name)

		// Check LLM request costs
		assert.Len(t, route.Spec.LLMRequestCosts, 3)
		costTypes := []aigwv1a1.LLMRequestCostType{
			aigwv1a1.LLMRequestCostTypeInputToken,
			aigwv1a1.LLMRequestCostTypeOutputToken,
			aigwv1a1.LLMRequestCostTypeTotalToken,
		}
		for i, cost := range route.Spec.LLMRequestCosts {
			assert.Equal(t, costTypes[i], cost.Type)
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
		assert.True(t, result)
	})

	t.Run("different spec", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.Spec.Rules[0].Matches[0].Headers[0].Value = "different-model"
		result := reconciler.SemanticEquals(route1, route2)
		assert.False(t, result)
	})

	t.Run("different labels", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.Labels["different-label"] = "different-value"
		result := reconciler.SemanticEquals(route1, route2)
		assert.False(t, result)
	})

	t.Run("different annotations", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.Annotations["different-annotation"] = "different-value"
		result := reconciler.SemanticEquals(route1, route2)
		assert.False(t, result)
	})

	t.Run("different resource version (should be equal)", func(t *testing.T) {
		route1 := baseRoute.DeepCopy()
		route2 := baseRoute.DeepCopy()
		route2.ResourceVersion = "12345"
		result := reconciler.SemanticEquals(route1, route2)
		assert.True(t, result) // ResourceVersion should not affect semantic equality
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
	assert.Equal(t, "test-service", name)
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := context.Background()

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
		assert.NoError(t, err)

		// Verify the route exists
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		assert.NoError(t, err)

		// Verify ownership tracking labels
		assert.Equal(t, isvc.Name, route.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, route.Labels[constants.InferenceServiceNamespaceLabel])
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
		assert.NoError(t, err)

		// Verify the route exists
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		assert.NoError(t, err)

		// Verify ownership tracking labels
		assert.Equal(t, isvc.Name, route.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, route.Labels[constants.InferenceServiceNamespaceLabel])
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
								Name: getAIServiceBackendName(tempIsvc),
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
		assert.NoError(t, err)

		// Verify the route exists
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		assert.NoError(t, err)

		// Verify ownership tracking labels
		assert.Equal(t, isvc.Name, route.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, route.Labels[constants.InferenceServiceNamespaceLabel])
	})
}

func TestDeleteAIGatewayRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, aigwv1a1.AddToScheme(scheme))

	ctx := context.Background()

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
		assert.NoError(t, err)

		// Verify the route was deleted
		var route aigwv1a1.AIGatewayRoute
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      isvc.Name,
			Namespace: "kserve-gateway",
		}, &route)
		assert.Error(t, err) // Should be NotFound error
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
		assert.Error(t, err) // Should return error for non-existing route
	})
}
