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

package trafficpolicy

import (
	"context"
	"testing"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestNewBackendTrafficPolicyReconciler(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}
	logger := log.Log.WithName("test")

	reconciler := NewBackendTrafficPolicyReconciler(fakeClient, ingressConfig, logger)

	assert.NotNil(t, reconciler)
	assert.Equal(t, fakeClient, reconciler.client)
	assert.Equal(t, ingressConfig, reconciler.ingressConfig)
	assert.Equal(t, logger, reconciler.log)
}

func TestCreateTrafficPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, egv1a1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}
	logger := log.Log.WithName("test")
	reconciler := NewBackendTrafficPolicyReconciler(fakeClient, ingressConfig, logger)

	t.Run("basic traffic policy creation", func(t *testing.T) {
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
				TrafficPolicy: &v1beta1.TrafficPolicy{
					RateLimit: &v1beta1.RateLimit{
						Global: egv1a1.GlobalRateLimit{
							Rules: []egv1a1.RateLimitRule{
								{
									ClientSelectors: []egv1a1.RateLimitSelectCondition{
										{
											Headers: []egv1a1.HeaderMatch{
												{
													Name:  "x-user-id",
													Value: ptr.To("user123"),
												},
											},
										},
									},
									Limit: egv1a1.RateLimitValue{
										Requests: 100,
										Unit:     egv1a1.RateLimitUnitMinute,
									},
								},
							},
						},
					},
				},
			},
		}

		result := reconciler.createTrafficPolicy(isvc)
		require.NotNil(t, result)

		// Check metadata
		assert.Equal(t, "test-isvc", result.Name)
		assert.Equal(t, "kserve-gateway", result.Namespace)

		// Check ownership tracking labels
		assert.Equal(t, isvc.Name, result.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, result.Labels[constants.InferenceServiceNamespaceLabel])

		// Check that original labels are preserved
		for k, v := range isvc.Labels {
			assert.Equal(t, v, result.Labels[k])
		}

		// Check annotations
		for k, v := range isvc.Annotations {
			assert.Equal(t, v, result.Annotations[k])
		}

		// Check spec
		assert.Len(t, result.Spec.TargetRefs, 1)
		assert.Equal(t, "gateway.networking.k8s.io", string(result.Spec.TargetRefs[0].Group))
		assert.Equal(t, constants.KindGateway, string(result.Spec.TargetRefs[0].Kind))
		assert.Equal(t, "kserve-ingress-gateway", string(result.Spec.TargetRefs[0].Name))

		// Check rate limit
		assert.NotNil(t, result.Spec.RateLimit)
		assert.Equal(t, egv1a1.GlobalRateLimitType, result.Spec.RateLimit.Type)
		assert.Equal(t, &isvc.Spec.TrafficPolicy.RateLimit.Global, result.Spec.RateLimit.Global)
	})

	t.Run("traffic policy with custom gateway", func(t *testing.T) {
		customIngressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "custom-gateway/custom-ingress-gateway",
		}
		customReconciler := NewBackendTrafficPolicyReconciler(fakeClient, customIngressConfig, logger)

		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc-custom",
				Namespace: "test-namespace",
			},
			Spec: v1beta1.InferenceServiceSpec{
				TrafficPolicy: &v1beta1.TrafficPolicy{
					RateLimit: &v1beta1.RateLimit{
						Global: egv1a1.GlobalRateLimit{
							Rules: []egv1a1.RateLimitRule{
								{
									ClientSelectors: []egv1a1.RateLimitSelectCondition{
										{
											Headers: []egv1a1.HeaderMatch{
												{
													Name:  "x-api-key",
													Value: ptr.To("key123"),
												},
											},
										},
									},
									Limit: egv1a1.RateLimitValue{
										Requests: 50,
										Unit:     egv1a1.RateLimitUnitSecond,
									},
								},
							},
						},
					},
				},
			},
		}

		result := customReconciler.createTrafficPolicy(isvc)
		require.NotNil(t, result)

		// Check metadata with custom gateway
		assert.Equal(t, "test-isvc-custom", result.Name)
		assert.Equal(t, "custom-gateway", result.Namespace)

		// Check target refs with custom gateway
		assert.Len(t, result.Spec.TargetRefs, 1)
		assert.Equal(t, "custom-ingress-gateway", string(result.Spec.TargetRefs[0].Name))
	})
}

func TestSemanticEquals(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, egv1a1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ingressConfig := &v1beta1.IngressConfig{
		KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
	}
	logger := log.Log.WithName("test")
	reconciler := NewBackendTrafficPolicyReconciler(fakeClient, ingressConfig, logger)

	basePolicy := &egv1a1.BackendTrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"test-label": "test-value",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
		Spec: egv1a1.BackendTrafficPolicySpec{
			PolicyTargetReferences: egv1a1.PolicyTargetReferences{
				TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
					{
						LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
							Group: "gateway.networking.k8s.io",
							Kind:  constants.KindGateway,
							Name:  "test-gateway",
						},
					},
				},
			},
			RateLimit: &egv1a1.RateLimitSpec{
				Type: egv1a1.GlobalRateLimitType,
				Global: &egv1a1.GlobalRateLimit{
					Rules: []egv1a1.RateLimitRule{
						{
							ClientSelectors: []egv1a1.RateLimitSelectCondition{
								{
									Headers: []egv1a1.HeaderMatch{
										{
											Name:  "x-user-id",
											Value: ptr.To("user123"),
										},
									},
								},
							},
							Limit: egv1a1.RateLimitValue{
								Requests: 100,
								Unit:     egv1a1.RateLimitUnitMinute,
							},
						},
					},
				},
			},
		},
	}

	t.Run("identical policies", func(t *testing.T) {
		desired := basePolicy.DeepCopy()
		existing := basePolicy.DeepCopy()
		result := reconciler.SemanticEquals(desired, existing)
		assert.True(t, result)
	})

	t.Run("different spec", func(t *testing.T) {
		desired := basePolicy.DeepCopy()
		existing := basePolicy.DeepCopy()
		existing.Spec.RateLimit.Global.Rules[0].Limit.Requests = 200
		result := reconciler.SemanticEquals(desired, existing)
		assert.False(t, result)
	})

	t.Run("different labels", func(t *testing.T) {
		desired := basePolicy.DeepCopy()
		existing := basePolicy.DeepCopy()
		existing.Labels["different-label"] = "different-value"
		result := reconciler.SemanticEquals(desired, existing)
		assert.False(t, result)
	})

	t.Run("different annotations", func(t *testing.T) {
		desired := basePolicy.DeepCopy()
		existing := basePolicy.DeepCopy()
		existing.Annotations["different-annotation"] = "different-value"
		result := reconciler.SemanticEquals(desired, existing)
		assert.False(t, result)
	})

	t.Run("different resource version (should be equal)", func(t *testing.T) {
		desired := basePolicy.DeepCopy()
		existing := basePolicy.DeepCopy()
		existing.ResourceVersion = "12345"
		result := reconciler.SemanticEquals(desired, existing)
		assert.True(t, result) // ResourceVersion should not affect semantic equality
	})
}

func TestGetTrafficPolicyName(t *testing.T) {
	t.Run("simple inference service name", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-model",
			},
		}
		result := getTrafficPolicyName(isvc)
		assert.Equal(t, "my-model", result)
	})

	t.Run("inference service with complex name", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-complex-model-v2",
			},
		}
		result := getTrafficPolicyName(isvc)
		assert.Equal(t, "my-complex-model-v2", result)
	})
}

func TestReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, egv1a1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("create new traffic policy", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
			},
			Spec: v1beta1.InferenceServiceSpec{
				TrafficPolicy: &v1beta1.TrafficPolicy{
					RateLimit: &v1beta1.RateLimit{
						Global: egv1a1.GlobalRateLimit{
							Rules: []egv1a1.RateLimitRule{
								{
									ClientSelectors: []egv1a1.RateLimitSelectCondition{
										{
											Headers: []egv1a1.HeaderMatch{
												{
													Name:  "x-user-id",
													Value: ptr.To("user123"),
												},
											},
										},
									},
									Limit: egv1a1.RateLimitValue{
										Requests: 100,
										Unit:     egv1a1.RateLimitUnitMinute,
									},
								},
							},
						},
					},
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := log.Log.WithName("test")
		reconciler := NewBackendTrafficPolicyReconciler(fakeClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		assert.NoError(t, err)

		// Verify the policy exists in the cluster
		policy := &egv1a1.BackendTrafficPolicy{}
		key := types.NamespacedName{
			Name:      getTrafficPolicyName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, policy)
		assert.NoError(t, err)

		// Verify ownership labels are set
		assert.Equal(t, isvc.Name, policy.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, policy.Labels[constants.InferenceServiceNamespaceLabel])

		// Verify spec is correct
		assert.Equal(t, egv1a1.GlobalRateLimitType, policy.Spec.RateLimit.Type)
		assert.Equal(t, &isvc.Spec.TrafficPolicy.RateLimit.Global, policy.Spec.RateLimit.Global)
	})

	t.Run("update existing traffic policy", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"updated-label": "updated-value",
				},
			},
			Spec: v1beta1.InferenceServiceSpec{
				TrafficPolicy: &v1beta1.TrafficPolicy{
					RateLimit: &v1beta1.RateLimit{
						Global: egv1a1.GlobalRateLimit{
							Rules: []egv1a1.RateLimitRule{
								{
									ClientSelectors: []egv1a1.RateLimitSelectCondition{
										{
											Headers: []egv1a1.HeaderMatch{
												{
													Name:  "x-user-id",
													Value: ptr.To("user123"),
												},
											},
										},
									},
									Limit: egv1a1.RateLimitValue{
										Requests: 200, // Updated limit
										Unit:     egv1a1.RateLimitUnitMinute,
									},
								},
							},
						},
					},
				},
			},
		}
		existingPolicy := &egv1a1.BackendTrafficPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-isvc",
				Namespace:       "kserve-gateway",
				ResourceVersion: "123",
				Labels: map[string]string{
					"old-label": "old-value",
				},
			},
			Spec: egv1a1.BackendTrafficPolicySpec{
				PolicyTargetReferences: egv1a1.PolicyTargetReferences{
					TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
								Group: "gateway.networking.k8s.io",
								Kind:  constants.KindGateway,
								Name:  "kserve-ingress-gateway",
							},
						},
					},
				},
				RateLimit: &egv1a1.RateLimitSpec{
					Type: egv1a1.GlobalRateLimitType,
					Global: &egv1a1.GlobalRateLimit{
						Rules: []egv1a1.RateLimitRule{
							{
								ClientSelectors: []egv1a1.RateLimitSelectCondition{
									{
										Headers: []egv1a1.HeaderMatch{
											{
												Name:  "x-user-id",
												Value: ptr.To("user123"),
											},
										},
									},
								},
								Limit: egv1a1.RateLimitValue{
									Requests: 100, // Old limit
									Unit:     egv1a1.RateLimitUnitMinute,
								},
							},
						},
					},
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPolicy).Build()
		logger := log.Log.WithName("test")
		reconciler := NewBackendTrafficPolicyReconciler(fakeClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		assert.NoError(t, err)

		// Verify the policy exists in the cluster
		policy := &egv1a1.BackendTrafficPolicy{}
		key := types.NamespacedName{
			Name:      getTrafficPolicyName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, policy)
		assert.NoError(t, err)

		// Verify ownership labels are set
		assert.Equal(t, isvc.Name, policy.Labels[constants.InferenceServiceNameLabel])
		assert.Equal(t, isvc.Namespace, policy.Labels[constants.InferenceServiceNamespaceLabel])

		// Verify updated label is present
		assert.Equal(t, "updated-value", policy.Labels["updated-label"])

		// Verify spec is updated
		assert.Equal(t, uint(200), policy.Spec.RateLimit.Global.Rules[0].Limit.Requests)
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
				TrafficPolicy: &v1beta1.TrafficPolicy{
					RateLimit: &v1beta1.RateLimit{
						Global: egv1a1.GlobalRateLimit{
							Rules: []egv1a1.RateLimitRule{
								{
									ClientSelectors: []egv1a1.RateLimitSelectCondition{
										{
											Headers: []egv1a1.HeaderMatch{
												{
													Name:  "x-user-id",
													Value: ptr.To("user123"),
												},
											},
										},
									},
									Limit: egv1a1.RateLimitValue{
										Requests: 100,
										Unit:     egv1a1.RateLimitUnitMinute,
									},
								},
							},
						},
					},
				},
			},
		}
		existingPolicy := &egv1a1.BackendTrafficPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-isvc",
				Namespace:       "kserve-gateway",
				ResourceVersion: "123",
				Labels: map[string]string{
					constants.InferenceServiceNameLabel:      "test-isvc",
					constants.InferenceServiceNamespaceLabel: "test-namespace",
				},
			},
			Spec: egv1a1.BackendTrafficPolicySpec{
				PolicyTargetReferences: egv1a1.PolicyTargetReferences{
					TargetRefs: []gwapiv1a2.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gwapiv1a2.LocalPolicyTargetReference{
								Group: "gateway.networking.k8s.io",
								Kind:  constants.KindGateway,
								Name:  "kserve-ingress-gateway",
							},
						},
					},
				},
				RateLimit: &egv1a1.RateLimitSpec{
					Type: egv1a1.GlobalRateLimitType,
					Global: &egv1a1.GlobalRateLimit{
						Rules: []egv1a1.RateLimitRule{
							{
								ClientSelectors: []egv1a1.RateLimitSelectCondition{
									{
										Headers: []egv1a1.HeaderMatch{
											{
												Name:  "x-user-id",
												Value: ptr.To("user123"),
											},
										},
									},
								},
								Limit: egv1a1.RateLimitValue{
									Requests: 100,
									Unit:     egv1a1.RateLimitUnitMinute,
								},
							},
						},
					},
				},
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPolicy).Build()
		logger := log.Log.WithName("test")
		reconciler := NewBackendTrafficPolicyReconciler(fakeClient, ingressConfig, logger)

		err := reconciler.Reconcile(ctx, isvc)
		assert.NoError(t, err)

		// Verify the policy exists in the cluster
		policy := &egv1a1.BackendTrafficPolicy{}
		key := types.NamespacedName{
			Name:      getTrafficPolicyName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, policy)
		assert.NoError(t, err)

		// Verify ResourceVersion hasn't changed (no update occurred)
		assert.Equal(t, "123", policy.ResourceVersion)
	})
}

func TestDeleteTrafficPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, egv1a1.AddToScheme(scheme))

	ctx := context.Background()

	t.Run("delete existing traffic policy", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
			},
		}
		existingPolicy := &egv1a1.BackendTrafficPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "kserve-gateway",
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPolicy).Build()
		logger := log.Log.WithName("test")

		err := DeleteTrafficPolicy(ctx, fakeClient, ingressConfig, isvc, logger)
		assert.NoError(t, err)

		// Verify the policy is deleted
		policy := &egv1a1.BackendTrafficPolicy{}
		key := types.NamespacedName{
			Name:      getTrafficPolicyName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, policy)
		assert.True(t, apierr.IsNotFound(err), "Policy should be deleted")
	})

	t.Run("delete non-existing traffic policy should not error", func(t *testing.T) {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-isvc",
				Namespace: "test-namespace",
			},
		}
		ingressConfig := &v1beta1.IngressConfig{
			KserveIngressGateway: "kserve-gateway/kserve-ingress-gateway",
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		logger := log.Log.WithName("test")

		err := DeleteTrafficPolicy(ctx, fakeClient, ingressConfig, isvc, logger)
		assert.NoError(t, err) // Should not error on not found

		// Verify the policy doesn't exist
		policy := &egv1a1.BackendTrafficPolicy{}
		key := types.NamespacedName{
			Name:      getTrafficPolicyName(isvc),
			Namespace: "kserve-gateway",
		}
		err = fakeClient.Get(ctx, key, policy)
		assert.True(t, apierr.IsNotFound(err), "Policy should not exist")
	})
}
