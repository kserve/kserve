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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// TestExpectedHTTPRoute_DisableHTTPRouteTimeout verifies that the v1alpha2
// LLMInferenceService router honors the disableHTTPRouteTimeout config flag by
// omitting spec.rules[*].timeouts from the generated HTTPRoute, mirroring the
// v1beta1 ingress reconciler. Regression test for issue #5707.
func TestExpectedHTTPRoute_DisableHTTPRouteTimeout(t *testing.T) {
	cases := []struct {
		name                    string
		disableHTTPRouteTimeout bool
		expectTimeouts          bool
		// withScheduler exercises the managed-pool path: a non-nil Scheduler
		// (with no pool Ref) passes the Scheduler==nil guard so the migration
		// block runs. This guards against a future regression where the
		// migration block is extended to touch Timeouts.
		withScheduler bool
	}{
		{
			name:                    "flag set omits timeouts",
			disableHTTPRouteTimeout: true,
			expectTimeouts:          false,
		},
		{
			name:                    "flag unset keeps timeouts",
			disableHTTPRouteTimeout: false,
			expectTimeouts:          true,
		},
		{
			name:                    "flag set omits timeouts on managed-pool migration path",
			disableHTTPRouteTimeout: true,
			expectTimeouts:          false,
			withScheduler:           true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			// The preset-rendered route spec always carries per-rule timeouts.
			ruleWithTimeouts := gwapiv1.HTTPRouteRule{
				Timeouts: &gwapiv1.HTTPRouteTimeouts{
					BackendRequest: ptr.To(gwapiv1.Duration("0s")),
					Request:        ptr.To(gwapiv1.Duration("0s")),
				},
			}
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{
							HTTP: &v1alpha2.HTTPRouteSpec{
								Spec: &gwapiv1.HTTPRouteSpec{
									Rules: []gwapiv1.HTTPRouteRule{ruleWithTimeouts},
								},
							},
						},
					},
				},
			}

			r := &LLMISVCReconciler{}

			if tc.withScheduler {
				// A managed pool (Scheduler set, no Ref) enters the migration
				// block. Add a default backendRef so the block mutates its
				// Group, proving the block actually ran past the early return.
				defaultPool := (&v1alpha2.SchedulerSpec{}).InferencePoolName(llmSvc)
				llmSvc.Spec.Router.Scheduler = &v1alpha2.SchedulerSpec{}
				llmSvc.Spec.Router.Route.HTTP.Spec.Rules[0].BackendRefs = []gwapiv1.HTTPBackendRef{
					{
						BackendRef: gwapiv1.BackendRef{
							BackendObjectReference: gwapiv1.BackendObjectReference{
								Kind: ptr.To(gwapiv1.Kind("InferencePool")),
								Name: gwapiv1.ObjectName(defaultPool),
							},
						},
					},
				}

				// r.Get in the migration block needs a real client. The route
				// doesn't exist, so Get returns NotFound and the reconciler
				// keeps the v1alpha2 API group (not yet migrated).
				scheme := runtime.NewScheme()
				g.Expect(gwapiv1.Install(scheme)).To(Succeed())
				r.Client = fake.NewClientBuilder().WithScheme(scheme).Build()
			}

			cfg := &Config{DisableHTTPRouteTimeout: tc.disableHTTPRouteTimeout}

			route := r.expectedHTTPRoute(context.Background(), llmSvc, cfg)

			g.Expect(route.Spec.Rules).To(HaveLen(1))
			if tc.expectTimeouts {
				g.Expect(route.Spec.Rules[0].Timeouts).ToNot(BeNil(),
					"expected Timeouts to be set when disableHTTPRouteTimeout is false")
			} else {
				g.Expect(route.Spec.Rules[0].Timeouts).To(BeNil(),
					"expected Timeouts to be nil when disableHTTPRouteTimeout is true")
			}

			if tc.withScheduler {
				// Confirm the migration block executed (didn't return early):
				// the default backendRef's Group is set to the v1alpha2 pool
				// group. The nil-Timeouts assertion above must survive it.
				g.Expect(route.Spec.Rules[0].BackendRefs).To(HaveLen(1))
				g.Expect(route.Spec.Rules[0].BackendRefs[0].Group).ToNot(BeNil())
				g.Expect(string(*route.Spec.Rules[0].BackendRefs[0].Group)).
					To(Equal(constants.InferencePoolV1Alpha2APIGroupName),
						"expected migration block to run and set the v1alpha2 pool group")
			}
		})
	}
}
