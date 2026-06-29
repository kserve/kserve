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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// ruleWithTimeouts builds a managed HTTPRoute rule carrying timeouts, mirroring the
// shape produced by the config-llm-router-route preset (timeouts: 0s on every rule).
func ruleWithTimeouts(path string) gwapiv1.HTTPRouteRule {
	return gwapiv1.HTTPRouteRule{
		BackendRefs: []gwapiv1.HTTPBackendRef{{
			BackendRef: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("Service")),
					Name: gwapiv1.ObjectName("svc"),
					Port: ptr.To(gwapiv1.PortNumber(8000)),
				},
				Weight: ptr.To(int32(1)),
			},
		}},
		Matches: []gwapiv1.HTTPRouteMatch{{
			Path: &gwapiv1.HTTPPathMatch{
				Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
				Value: ptr.To(path),
			},
		}},
		Timeouts: &gwapiv1.HTTPRouteTimeouts{
			BackendRequest: ptr.To(gwapiv1.Duration("0s")),
			Request:        ptr.To(gwapiv1.Duration("0s")),
		},
	}
}

// TestExpectedHTTPRouteDisableTimeout verifies that the v1alpha2 LLMISVC router
// honors the disableHTTPRouteTimeout ingress flag by stripping spec.rules.timeouts
// from the generated HTTPRoute. Some Gateway implementations (e.g. GKE) reject the
// timeouts field, so when the flag is set the field must be omitted entirely.
func TestExpectedHTTPRouteDisableTimeout(t *testing.T) {
	tests := []struct {
		name                    string
		disableHTTPRouteTimeout bool
		wantTimeouts            bool
	}{
		{
			name:                    "timeouts preserved when flag disabled",
			disableHTTPRouteTimeout: false,
			wantTimeouts:            true,
		},
		{
			name:                    "timeouts stripped when flag enabled",
			disableHTTPRouteTimeout: true,
			wantTimeouts:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "default"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{
							HTTP: &v1alpha2.HTTPRouteSpec{
								Spec: &gwapiv1.HTTPRouteSpec{
									Rules: []gwapiv1.HTTPRouteRule{
										ruleWithTimeouts("/v1/completions"),
										ruleWithTimeouts("/v1/chat/completions"),
									},
								},
							},
						},
					},
				},
			}

			r := &LLMISVCReconciler{}
			cfg := &Config{DisableHTTPRouteTimeout: tt.disableHTTPRouteTimeout}

			route := r.expectedHTTPRoute(context.Background(), llmSvc, cfg)

			if len(route.Spec.Rules) == 0 {
				t.Fatalf("expected generated HTTPRoute to have rules, got none")
			}
			for i, rule := range route.Spec.Rules {
				if tt.wantTimeouts && rule.Timeouts == nil {
					t.Errorf("rule[%d]: expected timeouts to be preserved, got nil", i)
				}
				if !tt.wantTimeouts && rule.Timeouts != nil {
					t.Errorf("rule[%d]: expected timeouts to be stripped, got %+v", i, *rule.Timeouts)
				}
			}
		})
	}
}
