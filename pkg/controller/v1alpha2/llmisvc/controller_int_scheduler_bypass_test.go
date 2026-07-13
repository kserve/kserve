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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Controller", func() {
	Context("Scheduler bypass when backendRefs do not reference InferencePool", func() {
		It("should reach RouterReady without InferencePool when no scheduler is configured and route uses a custom backendRef kind", func(ctx SpecContext) {
			svcName := "test-no-sched-custom-ref"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{
					Rules: []gwapiv1.HTTPRouteRule{
						HTTPRouteRule(
							WithBackendRefs(gwapiv1.HTTPBackendRef{
								BackendRef: gwapiv1.BackendRef{
									BackendObjectReference: gwapiv1.BackendObjectReference{
										Group: ptr.To(gwapiv1.Group("agentgateway.dev")),
										Kind:  ptr.To(gwapiv1.Kind("AgentgatewayBackend")),
										Name:  gwapiv1.ObjectName(svcName + "-backend"),
									},
								},
							}),
							WithMatches(PathPrefixMatch("/v1/chat/completions")),
						),
					},
				}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the controller to reconcile and set at least one condition
			Eventually(func(g Gomega, ctx SpecContext) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.Conditions).ToNot(BeEmpty(), "controller should have set at least one condition")
			}).WithContext(ctx).Should(Succeed())

			// Mark the controller-created HTTPRoute as ready
			Eventually(func(g Gomega, ctx SpecContext) {
				routes := &gwapiv1.HTTPRouteList{}
				g.Expect(envTest.Client.List(ctx, routes,
					client.InNamespace(testNs.Name),
					client.MatchingLabels(llmisvc.RouterLabels(llmSvc)),
				)).To(Succeed())
				g.Expect(routes.Items).To(HaveLen(1))

				updatedRoute := routes.Items[0].DeepCopy()
				WithHTTPRouteReadyStatus(DefaultGatewayControllerName)(updatedRoute)
				g.Expect(envTest.Client.Status().Update(ctx, updatedRoute)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// RouterReady should become True - no InferencePool needed
			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				poolCond := current.GetStatus().GetCondition(v1alpha2.InferencePoolReady)
				g.Expect(poolCond).To(BeNil(), "InferencePoolReady should be cleared when no scheduler is set")

				schedulerCond := current.GetStatus().GetCondition(v1alpha2.SchedulerWorkloadReady)
				g.Expect(schedulerCond).To(BeNil(), "SchedulerWorkloadReady should be cleared when no scheduler is set")
			})).WithContext(ctx).Should(Succeed())

			// No InferencePool should exist
			pools := &igwapi.InferencePoolList{}
			Expect(envTest.Client.List(ctx, pools, client.InNamespace(testNs.Name))).To(Succeed())
			Expect(pools.Items).To(BeEmpty(), "No InferencePool should be created")

			// No scheduler deployment should exist
			deps := &appsv1.DeploymentList{}
			Expect(envTest.Client.List(ctx, deps,
				client.InNamespace(testNs.Name),
				client.MatchingLabels(llmisvc.SchedulerLabels(llmSvc)),
			)).To(Succeed())
			Expect(deps.Items).To(BeEmpty(), "No scheduler deployment should be created")
		})

		It("should create scheduler and InferencePool when using default route with InferencePool backendRef", func(ctx SpecContext) {
			svcName := "test-default-pool-ref"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// InferencePool and scheduler deployment should be created
			Eventually(func(g Gomega, ctx SpecContext) {
				pools := &igwapi.InferencePoolList{}
				g.Expect(envTest.Client.List(ctx, pools, client.InNamespace(testNs.Name))).To(Succeed())
				g.Expect(pools.Items).ToNot(BeEmpty(), "InferencePool should be created for default backendRef")
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega, ctx SpecContext) {
				deps := &appsv1.DeploymentList{}
				g.Expect(envTest.Client.List(ctx, deps,
					client.InNamespace(testNs.Name),
					client.MatchingLabels(llmisvc.SchedulerLabels(llmSvc)),
				)).To(Succeed())
				g.Expect(deps.Items).ToNot(BeEmpty(), "Scheduler deployment should be created for default backendRef")
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
