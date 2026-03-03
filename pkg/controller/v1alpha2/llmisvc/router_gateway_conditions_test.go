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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

func TestGatewayConditionsEvaluation(t *testing.T) {
	tests := []struct {
		name             string
		llmSvc           *v1alpha2.LLMInferenceService
		gateways         []*gwapiv1.Gateway
		expectedErrorMsg string
		assertCondition  func(routerCondition, gatewayCondition *apis.Condition) assertConditionsFunc
	}{
		{
			name: "single ready gateway - router should be ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithGatewayRefs(v1alpha2.UntypedObjectReference{
					Name:      "ready-gateway",
					Namespace: "test-ns",
				}),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("ready-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
					WithProgrammedCondition(metav1.ConditionTrue, "Ready", "Gateway is ready"),
				),
			},
			assertCondition: assertRouterReady,
		},
		{
			name: "single not ready gateway - router should not be ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithGatewayRefs(v1alpha2.UntypedObjectReference{
					Name:      "not-ready-gateway",
					Namespace: "test-ns",
				}),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("not-ready-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithAddresses("203.0.113.1"),
					WithProgrammedCondition(metav1.ConditionFalse, "NotReady", "Gateway is not ready"),
				),
			},
			assertCondition: func(routerCondition, gatewayCondition *apis.Condition) assertConditionsFunc {
				return assertRouterNotReadyWithReason(routerCondition, gatewayCondition, "GatewaysNotReady")
			},
		},
		{
			name: "multiple gateways - all ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithGatewayRefs(
					v1alpha2.UntypedObjectReference{Name: "gateway-1", Namespace: "test-ns"},
					v1alpha2.UntypedObjectReference{Name: "gateway-2", Namespace: "test-ns"},
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("gateway-1",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithProgrammedCondition(metav1.ConditionTrue, "Ready", "Gateway 1 is ready"),
				),
				Gateway("gateway-2",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithProgrammedCondition(metav1.ConditionTrue, "Ready", "Gateway 2 is ready"),
				),
			},
			assertCondition: assertRouterReady,
		},
		{
			name: "multiple gateways - some not ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithGatewayRefs(
					v1alpha2.UntypedObjectReference{Name: "ready-gateway", Namespace: "test-ns"},
					v1alpha2.UntypedObjectReference{Name: "not-ready-gateway", Namespace: "test-ns"},
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("ready-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithProgrammedCondition(metav1.ConditionTrue, "Ready", "Gateway is ready"),
				),
				Gateway("not-ready-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithProgrammedCondition(metav1.ConditionFalse, "NotReady", "Gateway is not ready"),
				),
			},
			assertCondition: func(routerCondition, gatewayCondition *apis.Condition) assertConditionsFunc {
				return assertRouterNotReadyWithReason(routerCondition, gatewayCondition, "GatewaysNotReady")
			},
		},
		{
			name: "gateway with no programmed condition - should be not ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithGatewayRefs(v1alpha2.UntypedObjectReference{
					Name:      "no-condition-gateway",
					Namespace: "test-ns",
				}),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("no-condition-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					// No programmed condition set
				),
			},
			assertCondition: func(routerCondition, gatewayCondition *apis.Condition) assertConditionsFunc {
				return assertRouterNotReadyWithReason(routerCondition, gatewayCondition, "GatewaysNotReady")
			},
		},
		{
			name: "gateway not found - should return error",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithGatewayRefs(v1alpha2.UntypedObjectReference{
					Name:      "missing-gateway",
					Namespace: "test-ns",
				}),
			),
			gateways:         []*gwapiv1.Gateway{},
			expectedErrorMsg: "failed to get Gateway",
		},
		{
			name: "no gateway refs - should skip evaluation",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				// No gateway refs
			),
			gateways:        []*gwapiv1.Gateway{},
			assertCondition: assertConditionUnset,
		},
		{
			name: "gateway without namespace uses LLMInferenceService namespace",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithGatewayRefs(v1alpha2.UntypedObjectReference{
					Name: "same-ns-gateway",
					// Namespace omitted - should use test-ns
				}),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("same-ns-gateway",
					InNamespace[*gwapiv1.Gateway]("test-ns"),
					WithListener(gwapiv1.HTTPProtocolType),
					WithProgrammedCondition(metav1.ConditionTrue, "Ready", "Gateway is ready"),
				),
			},
			assertCondition: assertRouterReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			ctx := t.Context()

			scheme := runtime.NewScheme()
			err := v1alpha2.AddToScheme(scheme)
			g.Expect(err).ToNot(HaveOccurred())
			err = gwapiv1.Install(scheme)
			g.Expect(err).ToNot(HaveOccurred())

			var objects []client.Object
			objects = append(objects, tt.llmSvc)
			for _, gw := range tt.gateways {
				objects = append(objects, gw)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			reconciler := &llmisvc.LLMISVCReconciler{
				Client: fakeClient,
			}

			err = reconciler.EvaluateGatewayConditions(ctx, tt.llmSvc)

			if tt.expectedErrorMsg != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.expectedErrorMsg))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			tt.llmSvc.DetermineRouterReadiness()

			routerCondition := tt.llmSvc.GetStatus().GetCondition(v1alpha2.RouterReady)
			gatewayCondition := tt.llmSvc.GetStatus().GetCondition(v1alpha2.GatewaysReady)

			if tt.assertCondition != nil {
				tt.assertCondition(routerCondition, gatewayCondition)(g)
			}
		})
	}
}

func TestIsGatewayReady(t *testing.T) {
	tests := []struct {
		name     string
		gateway  *gwapiv1.Gateway
		expected bool
	}{
		{
			name: "gateway with programmed condition true - should be ready",
			gateway: Gateway("test-gateway",
				WithProgrammedCondition(metav1.ConditionTrue, "Ready", "Gateway is ready"),
			),
			expected: true,
		},
		{
			name: "gateway with programmed condition false - should not be ready",
			gateway: Gateway("test-gateway",
				WithProgrammedCondition(metav1.ConditionFalse, "NotReady", "Gateway is not ready"),
			),
			expected: false,
		},
		{
			name: "gateway with programmed condition unknown - should not be ready",
			gateway: Gateway("test-gateway",
				WithProgrammedCondition(metav1.ConditionUnknown, "Unknown", "Gateway status unknown"),
			),
			expected: false,
		},
		{
			name: "gateway with no conditions - should not be ready",
			gateway: Gateway("test-gateway",
				WithListener(gwapiv1.HTTPProtocolType),
			),
			expected: false,
		},
		{
			name: "gateway with other conditions but no programmed - should not be ready",
			gateway: Gateway("test-gateway",
				WithGatewayCondition("Accepted", metav1.ConditionTrue, "Accepted", "Gateway accepted"),
			),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			result := llmisvc.IsGatewayReady(tt.gateway)

			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestHTTPRouteConditionsEvaluation(t *testing.T) {
	tests := []struct {
		name             string
		llmSvc           *v1alpha2.LLMInferenceService
		httpRoutes       []*gwapiv1.HTTPRoute
		expectedErrorMsg string
		createAssertion  func(routerCondition, httpRouteCondition *apis.Condition) assertConditionsFunc
	}{
		{
			name: "HTTPRoute with multiple controllers - should be ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("llm"),
				WithModelURI("hf://facebook/opt-125m"),
				WithHTTPRouteRefs(HTTPRouteRef("facebook-opt-125m-single-simulated-kserve-route")),
			),
			httpRoutes: []*gwapiv1.HTTPRoute{
				HTTPRoute("facebook-opt-125m-single-simulated-kserve-route",
					InNamespace[*gwapiv1.HTTPRoute]("llm"),
					WithParentRefs(GatewayParentRef("openshift-ai-inference", "openshift-ingress")),
					WithHTTPRule(
						Matches(PathPrefixMatch("/llm/facebook-opt-125m-single-simulated")),
						WithBackendRefs(ServiceRef("facebook-opt-125m-single-simulated-kserve-workload-svc", 8000, 1)),
						Timeouts("0s", "0s"),
						Filters(gwapiv1.HTTPRouteFilter{
							Type: gwapiv1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gwapiv1.HTTPURLRewriteFilter{
								Path: &gwapiv1.HTTPPathModifier{
									Type:               gwapiv1.PrefixMatchHTTPPathModifier,
									ReplacePrefixMatch: ptr.To("/"),
								},
							},
						}),
					),
					WithHTTPRouteMultipleControllerStatus(
						GatewayParentRef("openshift-ai-inference", "openshift-ingress"),
						KuadrantControllerStatus,
						GatewayAPIControllerStatus,
					),
				),
			},
			createAssertion: assertRouterReady,
		},
		{
			name: "HTTPRoute with standard controller only - should be ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithHTTPRouteRefs(HTTPRouteRef("test-route")),
			),
			httpRoutes: []*gwapiv1.HTTPRoute{
				HTTPRoute("test-route",
					InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
					WithParentRefs(GatewayParentRef("test-gateway", "test-ns")),
					WithHTTPRouteReadyStatus("openshift.io/gateway-controller/v1"),
				),
			},
			createAssertion: assertRouterReady,
		},
		{
			name: "HTTPRoute not ready - should not be ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithHTTPRouteRefs(HTTPRouteRef("not-ready-route")),
			),
			httpRoutes: []*gwapiv1.HTTPRoute{
				HTTPRoute("not-ready-route",
					InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
					WithParentRefs(GatewayParentRef("test-gateway", "test-ns")),
					WithHTTPRouteNotReadyStatus("openshift.io/gateway-controller/v1", "NotAccepted", "Route was not accepted"),
				),
			},
			createAssertion: func(routerCondition, httpRouteCondition *apis.Condition) assertConditionsFunc {
				return assertRouterNotReadyWithReason(routerCondition, httpRouteCondition, "HTTPRoutesNotReady")
			},
		},
		{
			name: "no HTTPRoute refs - should skip evaluation",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				// No HTTPRoute refs
			),
			httpRoutes:      []*gwapiv1.HTTPRoute{},
			createAssertion: assertHTTPRouteConditionUnset,
		},
		{
			name: "HTTPRoute with only policy-controller status (no Accepted) - should not be ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithHTTPRouteRefs(HTTPRouteRef("policy-only-route")),
			),
			httpRoutes: []*gwapiv1.HTTPRoute{
				HTTPRoute("policy-only-route",
					InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
					WithParentRefs(GatewayParentRef("test-gateway", "test-ns")),
					WithHTTPRouteMultipleControllerStatus(
						GatewayParentRef("test-gateway", "test-ns"),
						KuadrantControllerStatus, // Only policy controller, no gateway controller
					),
				),
			},
			createAssertion: func(routerCondition, httpRouteCondition *apis.Condition) assertConditionsFunc {
				return assertRouterNotReadyWithReason(routerCondition, httpRouteCondition, "HTTPRoutesNotReady")
			},
		},
		{
			name: "HTTPRoute with Accepted=True but ResolvedRefs missing - should not be ready",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithModelURI("hf://test/model"),
				WithHTTPRouteRefs(HTTPRouteRef("accepted-no-resolved-route")),
			),
			httpRoutes: []*gwapiv1.HTTPRoute{
				HTTPRoute("accepted-no-resolved-route",
					InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
					WithParentRefs(GatewayParentRef("test-gateway", "test-ns")),
					WithHTTPRouteParentStatus(
						GatewayParentRef("test-gateway", "test-ns"),
						"openshift.io/gateway-controller/v1",
						metav1.Condition{
							Type:               string(gwapiv1.RouteConditionAccepted),
							Status:             metav1.ConditionTrue,
							Reason:             "Accepted",
							Message:            "Route was valid",
							LastTransitionTime: metav1.Now(),
						},
						// ResolvedRefs intentionally omitted
					),
				),
			},
			createAssertion: func(routerCondition, httpRouteCondition *apis.Condition) assertConditionsFunc {
				return assertRouterNotReadyWithReason(routerCondition, httpRouteCondition, "HTTPRoutesNotReady")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			ctx := t.Context()

			scheme := runtime.NewScheme()
			err := v1alpha2.AddToScheme(scheme)
			g.Expect(err).ToNot(HaveOccurred())
			err = gwapiv1.Install(scheme)
			g.Expect(err).ToNot(HaveOccurred())

			var objects []client.Object
			objects = append(objects, tt.llmSvc)
			for _, route := range tt.httpRoutes {
				objects = append(objects, route)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			reconciler := &llmisvc.LLMISVCReconciler{
				Client: fakeClient,
			}

			err = reconciler.EvaluateHTTPRouteConditions(ctx, tt.llmSvc)

			if tt.expectedErrorMsg != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.expectedErrorMsg))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			tt.llmSvc.DetermineRouterReadiness()

			routerCondition := tt.llmSvc.GetStatus().GetCondition(v1alpha2.RouterReady)
			httpRouteCondition := tt.llmSvc.GetStatus().GetCondition(v1alpha2.HTTPRoutesReady)

			if tt.createAssertion != nil {
				tt.createAssertion(routerCondition, httpRouteCondition)(g)
			}
		})
	}
}

func TestIsHTTPRouteReady(t *testing.T) {
	tests := []struct {
		name     string
		route    *gwapiv1.HTTPRoute
		expected bool
	}{
		{
			name:     "nil route",
			route:    nil,
			expected: false,
		},
		{
			name: "no parent refs",
			route: HTTPRoute("empty-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
			),
			expected: false,
		},
		{
			name: "single parent ref - ready",
			route: HTTPRoute("ready-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(GatewayParentRef("gw", "test-ns")),
				WithHTTPRouteReadyStatus("controller"),
			),
			expected: true,
		},
		{
			name: "single parent ref with multiple controllers - ready",
			route: HTTPRoute("multi-ctrl-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(GatewayParentRef("gw", "test-ns")),
				WithHTTPRouteMultipleControllerStatus(
					GatewayParentRef("gw", "test-ns"),
					KuadrantControllerStatus,
					GatewayAPIControllerStatus,
				),
			),
			expected: true,
		},
		{
			name: "multiple parent refs - all have status - ready",
			route: HTTPRoute("multi-parent-ready",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(
					GatewayParentRef("gw-a", "test-ns"),
					GatewayParentRef("gw-b", "test-ns"),
				),
				WithHTTPRouteParentStatus(
					GatewayParentRef("gw-a", "test-ns"), "controller",
					metav1.Condition{Type: string(gwapiv1.RouteConditionAccepted), Status: metav1.ConditionTrue, Reason: "Accepted", LastTransitionTime: metav1.Now()},
					metav1.Condition{Type: string(gwapiv1.RouteConditionResolvedRefs), Status: metav1.ConditionTrue, Reason: "ResolvedRefs", LastTransitionTime: metav1.Now()},
				),
				WithHTTPRouteParentStatus(
					GatewayParentRef("gw-b", "test-ns"), "controller",
					metav1.Condition{Type: string(gwapiv1.RouteConditionAccepted), Status: metav1.ConditionTrue, Reason: "Accepted", LastTransitionTime: metav1.Now()},
					metav1.Condition{Type: string(gwapiv1.RouteConditionResolvedRefs), Status: metav1.ConditionTrue, Reason: "ResolvedRefs", LastTransitionTime: metav1.Now()},
				),
			),
			expected: true,
		},
		{
			name: "single parent ref with Accepted=False - not ready",
			route: HTTPRoute("rejected-route",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(GatewayParentRef("gw", "test-ns")),
				WithHTTPRouteParentStatus(
					GatewayParentRef("gw", "test-ns"), "controller",
					metav1.Condition{Type: string(gwapiv1.RouteConditionAccepted), Status: metav1.ConditionFalse, Reason: "NotAllowed", LastTransitionTime: metav1.Now()},
				),
			),
			expected: false,
		},
		{
			name: "multiple parent refs - one has no status entry - not ready",
			route: HTTPRoute("multi-parent-partial",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(
					GatewayParentRef("gw-a", "test-ns"),
					GatewayParentRef("gw-b", "test-ns"),
				),
				// Only gw-a has a status entry; gw-b has none
				WithHTTPRouteParentStatus(
					GatewayParentRef("gw-a", "test-ns"), "controller",
					metav1.Condition{Type: string(gwapiv1.RouteConditionAccepted), Status: metav1.ConditionTrue, Reason: "Accepted", LastTransitionTime: metav1.Now()},
					metav1.Condition{Type: string(gwapiv1.RouteConditionResolvedRefs), Status: metav1.ConditionTrue, Reason: "ResolvedRefs", LastTransitionTime: metav1.Now()},
				),
			),
			expected: false,
		},
		{
			name: "multiple parent refs - one has only policy controller status - not ready",
			route: HTTPRoute("multi-parent-policy-only",
				InNamespace[*gwapiv1.HTTPRoute]("test-ns"),
				WithParentRefs(
					GatewayParentRef("gw-a", "test-ns"),
					GatewayParentRef("gw-b", "test-ns"),
				),
				// gw-a: gateway controller ready
				WithHTTPRouteParentStatus(
					GatewayParentRef("gw-a", "test-ns"), "controller",
					metav1.Condition{Type: string(gwapiv1.RouteConditionAccepted), Status: metav1.ConditionTrue, Reason: "Accepted", LastTransitionTime: metav1.Now()},
					metav1.Condition{Type: string(gwapiv1.RouteConditionResolvedRefs), Status: metav1.ConditionTrue, Reason: "ResolvedRefs", LastTransitionTime: metav1.Now()},
				),
				// gw-b: only a policy controller entry, no gateway controller status
				WithHTTPRouteMultipleControllerStatus(
					GatewayParentRef("gw-b", "test-ns"),
					KuadrantControllerStatus,
				),
			),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(llmisvc.IsHTTPRouteReady(tt.route)).To(Equal(tt.expected))
		})
	}
}

func TestFetchReferencedGateways(t *testing.T) {
	tests := []struct {
		name          string
		llmSvc        *v1alpha2.LLMInferenceService
		gateways      []*gwapiv1.Gateway
		expectedCount int
		expectedError string
	}{
		{
			name: "fetch single gateway successfully",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithGatewayRefs(v1alpha2.UntypedObjectReference{
					Name:      "test-gateway",
					Namespace: "test-ns",
				}),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("test-gateway", InNamespace[*gwapiv1.Gateway]("test-ns")),
			},
			expectedCount: 1,
		},
		{
			name: "fetch multiple gateways successfully",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithGatewayRefs(
					v1alpha2.UntypedObjectReference{Name: "gateway-1", Namespace: "test-ns"},
					v1alpha2.UntypedObjectReference{Name: "gateway-2", Namespace: "other-ns"},
				),
			),
			gateways: []*gwapiv1.Gateway{
				Gateway("gateway-1", InNamespace[*gwapiv1.Gateway]("test-ns")),
				Gateway("gateway-2", InNamespace[*gwapiv1.Gateway]("other-ns")),
			},
			expectedCount: 2,
		},
		{
			name: "gateway not found - should return error",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				WithGatewayRefs(v1alpha2.UntypedObjectReference{
					Name:      "missing-gateway",
					Namespace: "test-ns",
				}),
			),
			gateways:      []*gwapiv1.Gateway{},
			expectedCount: 0,
			expectedError: "failed to get Gateway",
		},
		{
			name: "no gateway refs - should return empty slice",
			llmSvc: LLMInferenceService("test-llm",
				InNamespace[*v1alpha2.LLMInferenceService]("test-ns"),
				// No gateway refs
			),
			gateways:      []*gwapiv1.Gateway{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			ctx := t.Context()

			scheme := runtime.NewScheme()
			err := v1alpha2.AddToScheme(scheme)
			g.Expect(err).ToNot(HaveOccurred())
			err = gwapiv1.Install(scheme)
			g.Expect(err).ToNot(HaveOccurred())

			var objects []client.Object
			objects = append(objects, tt.llmSvc)
			for _, gw := range tt.gateways {
				objects = append(objects, gw)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			reconciler := &llmisvc.LLMISVCReconciler{
				Client: fakeClient,
			}

			gateways, err := reconciler.CollectReferencedGateways(ctx, tt.llmSvc)

			if tt.expectedError != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.expectedError))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gateways).To(HaveLen(tt.expectedCount))
		})
	}
}

// assertConditionsFunc defines the signature for condition assertion functions
type assertConditionsFunc func(g *WithT)

// assertConditionUnset returns a function that verifies the router is ready but gateway condition is not set
func assertConditionUnset(routerCondition, gatewayCondition *apis.Condition) assertConditionsFunc {
	return func(g *WithT) {
		g.Expect(routerCondition.IsTrue()).To(BeTrue(), "Router should be ready")
		g.Expect(gatewayCondition).To(BeNil(), "Gateway condition should not be set when no gateway refs")
	}
}

// assertRouterReady returns a function that verifies both router and gateway conditions are set and ready
func assertRouterReady(routerCondition, gatewayCondition *apis.Condition) assertConditionsFunc {
	return func(g *WithT) {
		g.Expect(routerCondition).ToNot(BeNil(), "Router condition should be set")
		g.Expect(routerCondition.IsTrue()).To(BeTrue(), "Router should be ready")
		g.Expect(gatewayCondition).ToNot(BeNil(), "Gateway condition should be set")
		g.Expect(gatewayCondition.IsTrue()).To(BeTrue(), "Gateways should be ready")
	}
}

// assertRouterNotReady returns a function that verifies both router and gateway conditions are set but not ready
func assertRouterNotReady(routerCondition, gatewayCondition *apis.Condition) assertConditionsFunc {
	return func(g *WithT) {
		g.Expect(routerCondition).ToNot(BeNil(), "Router condition should be set")
		g.Expect(routerCondition.IsFalse()).To(BeTrue(), "Router should not be ready")
		g.Expect(gatewayCondition).ToNot(BeNil(), "Gateway condition should be set")
		g.Expect(gatewayCondition.IsFalse()).To(BeTrue(), "Gateways should not be ready")
	}
}

// assertRouterNotReadyWithReason returns a function that verifies conditions are not ready and checks the reason
func assertRouterNotReadyWithReason(routerCondition, gatewayCondition *apis.Condition, expectedReason string) assertConditionsFunc {
	return func(g *WithT) {
		g.Expect(routerCondition).ToNot(BeNil(), "Router condition should be set")
		g.Expect(routerCondition.IsFalse()).To(BeTrue(), "Router should not be ready")
		g.Expect(gatewayCondition).ToNot(BeNil(), "Gateway condition should be set")
		g.Expect(gatewayCondition.IsFalse()).To(BeTrue(), "Gateways should not be ready")
		g.Expect(routerCondition.Reason).To(Equal(gatewayCondition.Reason))
		g.Expect(routerCondition.Reason).To(Equal(expectedReason))
	}
}

// assertHTTPRouteConditionUnset returns a function that verifies the router is ready and HTTPRoute condition is cleared
func assertHTTPRouteConditionUnset(routerCondition, httpRouteCondition *apis.Condition) assertConditionsFunc {
	return func(g *WithT) {
		g.Expect(routerCondition.IsTrue()).To(BeTrue(), "Router should be ready")
		g.Expect(httpRouteCondition).To(BeNil(), "HTTPRoute condition should not be set when no HTTPRoute refs")
	}
}
