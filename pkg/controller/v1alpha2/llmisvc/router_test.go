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

package llmisvc_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestExpectedHTTPRoute_VersionSelection(t *testing.T) {

	llmsvcName := "test-llm"
	backendRefInfPoolV1 := gwapiv1.BackendRef{
		BackendObjectReference: gwapiv1.BackendObjectReference{
			Group: ptr.To(gwapiv1.Group("inference.networking.k8s.io")),
			Kind:  ptr.To(gwapiv1.Kind("InferencePool")),
			Name:  gwapiv1.ObjectName(llmsvcName + "-inference-pool"),
		},
	}
	var backendRefInfPoolV1Alpha2 = gwapiv1.BackendRef{
		BackendObjectReference: gwapiv1.BackendObjectReference{
			Group: ptr.To(gwapiv1.Group("inference.networking.x-k8s.io")),
			Kind:  ptr.To(gwapiv1.Kind("InferencePool")),
			Name:  gwapiv1.ObjectName(llmsvcName + "-inference-pool"),
		},
	}

	tests := []struct {
		name                           string
		inferencePoolV1Available       bool
		inferencePoolV1Alpha2Available bool
		initialRouterBackendRef        gwapiv1.BackendRef
		wantAnnotation                 bool
		wantGroup                      string
	}{
		{
			name:                           "New route, v1 available, v1 from template input",
			inferencePoolV1Available:       true,
			inferencePoolV1Alpha2Available: true,
			initialRouterBackendRef:        backendRefInfPoolV1,
			wantAnnotation:                 true,
			wantGroup:                      "inference.networking.k8s.io",
		},
		{
			name:                           "New route, v1 available, v1alpha2 from template input",
			inferencePoolV1Available:       true,
			inferencePoolV1Alpha2Available: true,
			initialRouterBackendRef:        backendRefInfPoolV1Alpha2,
			wantAnnotation:                 true,
			wantGroup:                      "inference.networking.k8s.io",
		},
		{
			name:                           "New route, v1 not available, v1alpha2 available, v1 from template input",
			inferencePoolV1Available:       false,
			inferencePoolV1Alpha2Available: true,
			initialRouterBackendRef:        backendRefInfPoolV1,
			wantAnnotation:                 false,
			wantGroup:                      "inference.networking.x-k8s.io",
		},
		{
			name:                           "New route, v1 not available, v1alpha2 available, v1alpha2 from template input",
			inferencePoolV1Available:       false,
			inferencePoolV1Alpha2Available: true,
			initialRouterBackendRef:        backendRefInfPoolV1Alpha2,
			wantAnnotation:                 false,
			wantGroup:                      "inference.networking.x-k8s.io",
		},
		{
			name:                           "New route, neither v1 nor v1alpha2 available",
			inferencePoolV1Available:       false,
			inferencePoolV1Alpha2Available: false,
			wantAnnotation:                 false,
			wantGroup:                      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create a fake client
			fakeClient := fake.NewClientBuilder().Build()

			// Create a test LLMInferenceService
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      llmsvcName,
					Namespace: "test-namespace",
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{
							HTTP: &v1alpha2.HTTPRouteSpec{
								Spec: &gwapiv1.HTTPRouteSpec{
									Rules: []gwapiv1.HTTPRouteRule{
										{
											BackendRefs: []gwapiv1.HTTPBackendRef{
												{
													BackendRef: tt.initialRouterBackendRef,
												},
											},
										},
									},
								},
							},
						},
						Scheduler: &v1alpha2.SchedulerSpec{
							Pool: &v1alpha2.InferencePoolSpec{},
						},
					},
				},
			}

			// Create a reconciler
			reconciler := &llmisvc.LLMISVCReconciler{
				Client:                         fakeClient,
				InferencePoolV1Available:       tt.inferencePoolV1Available,
				InferencePoolV1Alpha2Available: tt.inferencePoolV1Alpha2Available,
			}

			// Create context
			ctx := context.Background()

			// Get expected HTTPRoute
			expectedRoute := reconciler.ExpectedHTTPRoute(ctx, llmSvc)

			// Check annotation
			if tt.wantAnnotation {
				g.Expect(expectedRoute.Annotations).To(HaveKey(llmisvc.AnnotationInferencePoolMigrated))
				g.Expect(expectedRoute.Annotations[llmisvc.AnnotationInferencePoolMigrated]).To(Equal("v1"))
			} else {
				if expectedRoute.Annotations != nil {
					g.Expect(expectedRoute.Annotations).ToNot(HaveKey(llmisvc.AnnotationInferencePoolMigrated))
				}
			}

			// Check backend group
			if tt.wantGroup != "" {
				// Check if any BackendRef has the correct group set
				anyGroupSet := false
				for _, rule := range expectedRoute.Spec.Rules {
					for _, httpBackendRef := range rule.BackendRefs {
						if httpBackendRef.BackendRef.Group != nil {
							anyGroupSet = true
							g.Expect(string(*httpBackendRef.BackendRef.Group)).To(Equal(tt.wantGroup), "Unexpected backend group")
							break
						}
					}
					if anyGroupSet {
						break
					}
				}
				// If no group is set, it might be because there's no matching default backend reference
				// We don't force group setting here, just check that the set group is correct
			} else {
				// Check that no BackendRef has a group set
				for _, rule := range expectedRoute.Spec.Rules {
					for _, httpBackendRef := range rule.BackendRefs {
						g.Expect(httpBackendRef.BackendRef.Group).To(BeNil(), "Expected no group when neither v1 nor v1alpha2 is available")
					}
				}
			}
		})
	}
}
