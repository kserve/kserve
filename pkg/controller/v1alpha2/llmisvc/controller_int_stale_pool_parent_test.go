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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller", func() {
	Context("Stale InferencePool parent handling", func() {
		It("should not block readiness when a stale rejected parent from an old gateway is present", func(ctx SpecContext) {
			// This test reproduces the scenario where a user switches gateway references
			// on an LLMInferenceService. The gateway controller for the old gateway leaves
			// a rejected parent entry in InferencePool.Status.Parents, while the current
			// gateway has accepted the pool. The stale rejected parent should not prevent
			// the service from reaching ready state.

			svcName := "test-stale-pool-parent"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the InferencePool to be created by the reconciler.
			poolName := svcName + "-inference-pool"
			Eventually(func(g Gomega, ctx context.Context) {
				pool := &igwapi.InferencePool{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKey{
					Name: poolName, Namespace: testNs.Name,
				}, pool)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Simulate the gateway controller writing pool parents and mark all resources ready.
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			// Verify the service reaches ready state with the current gateway.
			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"))
			})).WithContext(ctx).Should(Succeed())

			// Now inject a stale rejected parent from an old gateway into the pool status.
			// This simulates what happens when:
			// 1. The LLMISVC previously referenced data-science-gateway
			// 2. The user switched to the default gateway (kserve-ingress-gateway)
			// 3. The old gateway controller's rejected parent entry lingers in the pool status
			pool := &igwapi.InferencePool{}
			Expect(envTest.Client.Get(ctx, client.ObjectKey{
				Name: poolName, Namespace: testNs.Name,
			}, pool)).To(Succeed())

			pool.Status.Parents = append(pool.Status.Parents, igwapi.ParentStatus{
				ParentRef: igwapi.ParentReference{
					Group:     ptr.To(igwapi.Group("networking.istio.io")),
					Kind:      igwapi.Kind("Gateway"),
					Name:      "data-science-gateway",
					Namespace: "openshift-ingress",
				},
				Conditions: []metav1.Condition{
					{
						Type:               string(igwapi.InferencePoolConditionAccepted),
						Status:             metav1.ConditionFalse,
						Reason:             "HTTPRouteNotAccepted",
						Message:            "namespace not allowed by the parent",
						LastTransitionTime: metav1.Now(),
					},
				},
			})
			Expect(envTest.Client.Status().Update(ctx, pool)).To(Succeed())

			// Wait for the reconciler to process the pool status change. The stale rejected
			// parent causes the unfiltered IsInferencePoolReady check to return false, which
			// flips InferencePoolReady to False.
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "False"))
			}).WithContext(ctx).Should(Succeed(),
				"InferencePoolReady should flip to False after stale parent injection (demonstrating the bug)")

			// BUG: The service should recover to ready state because the current gateway
			// (kserve-ingress-gateway) has accepted the pool. The stale parent from
			// data-science-gateway is irrelevant - it belongs to a gateway the HTTPRoute
			// no longer references. But the unfiltered readiness check treats ANY rejected
			// parent as a hard failure.
			//
			// This assertion will fail on the unfixed code, proving the bug exists.
			// After the fix, it will pass because readiness is scoped to current gateways.
			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"),
					"stale rejected parent from old gateway should not block readiness")
			})).WithContext(ctx).Should(Succeed())
		})
	})
})
