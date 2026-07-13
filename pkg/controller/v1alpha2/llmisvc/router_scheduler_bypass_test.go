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

package llmisvc_test

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

func backendRefAgentgateway(name string) gwapiv1.HTTPBackendRef {
	return gwapiv1.HTTPBackendRef{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Group: ptr.To(gwapiv1.Group("agentgateway.dev")),
				Kind:  ptr.To(gwapiv1.Kind("AgentgatewayBackend")),
				Name:  gwapiv1.ObjectName(name),
			},
		},
	}
}

func TestValidateSchedulerBackendRefConsistency(t *testing.T) {
	tests := []struct {
		name        string
		llmSvc      *v1alpha2.LLMInferenceService
		expectError bool
		errorSubstr string
	}{
		{
			name: "no scheduler - no error",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						HTTPRouteRule(
							WithBackendRefs(backendRefAgentgateway("no-sched-backend")),
						),
					},
				}),
			),
			expectError: false,
		},
		{
			name: "scheduler with InferencePool backendRef - no error",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithManagedScheduler(),
				WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						HTTPRouteRule(
							WithBackendRefs(BackendRefInferencePool("test-llm-inference-pool")),
							WithMatches(PathPrefixMatch("/v1/completions")),
						),
					},
				}),
			),
			expectError: false,
		},
		{
			name: "scheduler with custom backendRef bypassing InferencePool - validation error",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithManagedScheduler(),
				WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						HTTPRouteRule(
							WithBackendRefs(backendRefAgentgateway("mismatched-backend")),
							WithMatches(PathPrefixMatch("/v1/chat/completions")),
						),
					},
				}),
			),
			expectError: true,
			errorSubstr: "spec.router.scheduler is configured but no HTTPRoute backendRef references the managed InferencePool",
		},
		{
			name: "external pool ref with matching backendRef - no error",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithInferencePoolRef("custom-pool"),
				WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						HTTPRouteRule(
							WithBackendRefs(BackendRefInferencePool("custom-pool")),
							WithMatches(PathPrefixMatch("/v1/completions")),
						),
					},
				}),
			),
			expectError: false,
		},
		{
			name: "scheduler without route spec - no error (defaults to InferencePool)",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithManagedScheduler(),
			),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			reconciler := &llmisvc.LLMISVCReconciler{}
			err := llmisvc.ValidateSchedulerBackendRefConsistencyForTest(reconciler, tt.llmSvc)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(llmisvc.IsValidationError(err)).To(BeTrue(), "error should be a ValidationError")
				g.Expect(err.Error()).To(ContainSubstring(tt.errorSubstr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
