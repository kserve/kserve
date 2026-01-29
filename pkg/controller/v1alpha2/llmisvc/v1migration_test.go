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

package llmisvc

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestIsMigratedToV1(t *testing.T) {
	tests := []struct {
		name     string
		llmSvc   *v1alpha2.LLMInferenceService
		expected bool
	}{
		{
			name:     "nil LLMInferenceService returns false",
			llmSvc:   nil,
			expected: false,
		},
		{
			name: "no annotations returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
				},
			},
			expected: false,
		},
		{
			name: "migration annotation not set returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "migration annotation set to v1 returns true",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v1",
					},
				},
			},
			expected: true,
		},
		{
			name: "migration annotation set to different value returns false",
			llmSvc: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-svc",
					Annotations: map[string]string{
						constants.InferencePoolMigratedAnnotationKey: "v2",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			result := isMigratedToV1(tt.llmSvc)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestAdjustBackendRefForMigration_V1OnlyCRD(t *testing.T) {
	// Test that when v1alpha2 CRD is not available, backendRefs stay as v1
	g := NewGomegaWithT(t)

	// Create a reconciler with v1alpha2 not available
	reconciler := &LLMISVCReconciler{
		InferencePoolV1Alpha2Available: false, // v1-only CRD scenario
	}

	// Create a config with v1 InferencePool backendRef
	v1Group := gwapiv1.Group(constants.InferencePoolV1APIGroupName)
	llmSvcCfg := &v1alpha2.LLMInferenceServiceConfig{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Spec: &igwapi.InferencePoolSpec{
							TargetPorts: []igwapi.Port{{Number: 8000}},
						},
					},
				},
				Route: &v1alpha2.GatewayRoutesSpec{
					HTTP: &v1alpha2.HTTPRouteSpec{
						Spec: &gwapiv1.HTTPRouteSpec{
							Rules: []gwapiv1.HTTPRouteRule{
								{
									BackendRefs: []gwapiv1.HTTPBackendRef{
										{
											BackendRef: gwapiv1.BackendRef{
												BackendObjectReference: gwapiv1.BackendObjectReference{
													Group: &v1Group,
													Kind:  ptr.To[gwapiv1.Kind]("InferencePool"),
													Name:  "test-pool",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
	}

	// Call adjustBackendRefForMigration
	result := reconciler.adjustBackendRefForMigration(llmSvc, llmSvcCfg, false)

	// Verify that backendRef still points to v1 (not changed to v1alpha2)
	backendRef := result.Spec.Router.Route.HTTP.Spec.Rules[0].BackendRefs[0]
	g.Expect(backendRef.Group).ToNot(BeNil())
	g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1APIGroupName),
		"When v1alpha2 CRD not available, backendRef should stay as v1")
}

func TestAdjustBackendRefForMigration_V1Alpha2Available(t *testing.T) {
	// Test that when v1alpha2 CRD is available and not migrated, backendRefs change to v1alpha2
	g := NewGomegaWithT(t)

	// Create a reconciler with v1alpha2 available
	reconciler := &LLMISVCReconciler{
		InferencePoolV1Alpha2Available: true, // Both CRDs available
	}

	// Create a config with v1 InferencePool backendRef
	v1Group := gwapiv1.Group(constants.InferencePoolV1APIGroupName)
	llmSvcCfg := &v1alpha2.LLMInferenceServiceConfig{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Spec: &igwapi.InferencePoolSpec{
							TargetPorts: []igwapi.Port{{Number: 8000}},
						},
					},
				},
				Route: &v1alpha2.GatewayRoutesSpec{
					HTTP: &v1alpha2.HTTPRouteSpec{
						Spec: &gwapiv1.HTTPRouteSpec{
							Rules: []gwapiv1.HTTPRouteRule{
								{
									BackendRefs: []gwapiv1.HTTPBackendRef{
										{
											BackendRef: gwapiv1.BackendRef{
												BackendObjectReference: gwapiv1.BackendObjectReference{
													Group: &v1Group,
													Kind:  ptr.To[gwapiv1.Kind]("InferencePool"),
													Name:  "test-pool",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
	}

	// Call adjustBackendRefForMigration with no rejection
	result := reconciler.adjustBackendRefForMigration(llmSvc, llmSvcCfg, false)

	// Verify that backendRef changed to v1alpha2
	backendRef := result.Spec.Router.Route.HTTP.Spec.Rules[0].BackendRefs[0]
	g.Expect(backendRef.Group).ToNot(BeNil())
	g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1Alpha2APIGroupName),
		"When v1alpha2 CRD available and not migrated, backendRef should change to v1alpha2")
}

func TestAdjustBackendRefForMigration_V1Alpha2Rejected(t *testing.T) {
	// Test that when v1alpha2 is rejected by Gateway, backendRefs stay as v1
	g := NewGomegaWithT(t)

	// Create a reconciler with v1alpha2 available
	reconciler := &LLMISVCReconciler{
		InferencePoolV1Alpha2Available: true,
	}

	// Create a config with v1 InferencePool backendRef
	v1Group := gwapiv1.Group(constants.InferencePoolV1APIGroupName)
	llmSvcCfg := &v1alpha2.LLMInferenceServiceConfig{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Spec: &igwapi.InferencePoolSpec{
							TargetPorts: []igwapi.Port{{Number: 8000}},
						},
					},
				},
				Route: &v1alpha2.GatewayRoutesSpec{
					HTTP: &v1alpha2.HTTPRouteSpec{
						Spec: &gwapiv1.HTTPRouteSpec{
							Rules: []gwapiv1.HTTPRouteRule{
								{
									BackendRefs: []gwapiv1.HTTPBackendRef{
										{
											BackendRef: gwapiv1.BackendRef{
												BackendObjectReference: gwapiv1.BackendObjectReference{
													Group: &v1Group,
													Kind:  ptr.To[gwapiv1.Kind]("InferencePool"),
													Name:  "test-pool",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "test-ns",
		},
	}

	// Call adjustBackendRefForMigration WITH rejection flag
	result := reconciler.adjustBackendRefForMigration(llmSvc, llmSvcCfg, true)

	// Verify that backendRef stays as v1 (because of rejection)
	backendRef := result.Spec.Router.Route.HTTP.Spec.Rules[0].BackendRefs[0]
	g.Expect(backendRef.Group).ToNot(BeNil())
	g.Expect(string(*backendRef.Group)).To(Equal(constants.InferencePoolV1APIGroupName),
		"When v1alpha2 is rejected, backendRef should stay as v1")
}
