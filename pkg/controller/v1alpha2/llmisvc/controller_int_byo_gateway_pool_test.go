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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller", func() {
	Context("BYO gateway InferencePool readiness", func() {
		// Regression test: with a bring-your-own Gateway (spec.router.gateway.refs),
		// a managed scheduler, and no managed route, there are no kserve-managed
		// HTTPRoutes to resolve gateways from, so the pool-readiness gateway scope
		// must come from the spec gateway refs. An external controller (e.g. Envoy
		// AI Gateway) attaches the pool to the referenced gateway.
		It("should mark InferencePoolReady when the referenced gateway accepts the pool without a managed route", func(ctx SpecContext) {
			svcName := "test-byo-gw-pool-readiness"
			testNs := NewTestNamespace(ctx, envTest)

			gwName := "byo-gw"
			gw := Gateway(gwName,
				InNamespace[*gwapiv1.Gateway](testNs.Name),
				WithListener(gwapiv1.HTTPProtocolType),
			)
			Expect(envTest.Client.Create(ctx, gw)).To(Succeed())
			ensureGatewayReady(ctx, envTest.Client, gw)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithGatewayRefs(LLMGatewayRef(gwName, testNs.Name)),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			poolName := svcName + "-inference-pool"
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Client.Get(ctx, client.ObjectKey{
					Name: poolName, Namespace: testNs.Name,
				}, &igwapi.InferencePool{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// RouterReady also aggregates the scheduler workload - mark its
			// deployment ready so the assertions below isolate pool readiness.
			ensureSchedulerDeploymentReady(ctx, envTest.Client, llmSvc)

			// Simulate the external gateway controller accepting the pool from the
			// referenced gateway - this is what happens when an externally-managed
			// route references the pool as a backend.
			acceptPoolFromGateway(ctx, envTest.Client,
				client.ObjectKey{Name: poolName, Namespace: testNs.Name},
				igwapi.ParentReference{
					Group:     ptr.To(igwapi.Group("gateway.networking.k8s.io")),
					Kind:      igwapi.Kind("Gateway"),
					Name:      igwapi.ObjectName(gwName),
					Namespace: igwapi.Namespace(testNs.Name),
				},
			)

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"),
					"InferencePool accepted by the spec-referenced gateway must satisfy readiness even without managed HTTPRoutes")
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.RouterReady), "True"))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

// acceptPoolFromGateway sets an InferencePool's status to Accepted by the given
// gateway parent, simulating an external gateway controller (e.g. Envoy AI Gateway)
// that attaches the pool to a bring-your-own gateway. Only runs in envtest (no-op
// against a real cluster where the gateway controller manages pool status).
func acceptPoolFromGateway(ctx context.Context, c client.Client, poolKey client.ObjectKey, parentRef igwapi.ParentReference) {
	if envTest.UsingExistingCluster() {
		return
	}

	Eventually(func(g Gomega, ctx context.Context) {
		pool := &igwapi.InferencePool{}
		g.Expect(c.Get(ctx, poolKey, pool)).To(Succeed())
		WithInferencePoolReadyStatus(parentRef)(pool)
		g.Expect(c.Status().Update(ctx, pool)).To(Succeed())
	}).WithContext(ctx).Should(Succeed())
}

var _ = Describe("LLMInferenceService Controller", func() {
	Context("BYO gateway readiness and observed topology", func() {
		// Regression: with gateway.refs and no managed route, gateway readiness was
		// evaluated against the route-resolved gateway set, which is empty in this
		// shape - so an unprogrammed gateway was reported ready having been evaluated
		// against nothing.
		It("should not mark GatewaysReady when the referenced gateway is not programmed", func(ctx SpecContext) {
			svcName := "test-byo-gw-unprogrammed"
			testNs := NewTestNamespace(ctx, envTest)

			gwName := "byo-gw-unprogrammed"
			// Created but never programmed: no Accepted/Programmed conditions.
			gw := Gateway(gwName,
				InNamespace[*gwapiv1.Gateway](testNs.Name),
				WithListener(gwapiv1.HTTPProtocolType),
			)
			Expect(envTest.Client.Create(ctx, gw)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithGatewayRefs(LLMGatewayRef(gwName, testNs.Name)),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.GatewaysReady), "False"),
					"an unprogrammed referenced Gateway must not report ready")
			}).WithContext(ctx).Should(Succeed())
		})

		// Regression: updateRoutingStatus nils status.router on the Route == nil path,
		// so the gateway the service is actually attached to, and the pool/EPP refs,
		// were never reported.
		It("should report the referenced gateway and scheduler refs in status.router", func(ctx SpecContext) {
			svcName := "test-byo-gw-observed"
			testNs := NewTestNamespace(ctx, envTest)

			gwName := "byo-gw-observed"
			gw := Gateway(gwName,
				InNamespace[*gwapiv1.Gateway](testNs.Name),
				WithListener(gwapiv1.HTTPProtocolType),
			)
			Expect(envTest.Client.Create(ctx, gw)).To(Succeed())
			ensureGatewayReady(ctx, envTest.Client, gw)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithGatewayRefs(LLMGatewayRef(gwName, testNs.Name)),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				g.Expect(current.Status.Router).ToNot(BeNil(),
					"status.router must report topology even without a managed route")
				g.Expect(current.Status.Router.Gateways).To(HaveLen(1))
				g.Expect(string(current.Status.Router.Gateways[0].Name)).To(Equal(gwName))
				g.Expect(current.Status.Router.Gateways[0].Namespace).ToNot(BeNil())
				g.Expect(string(*current.Status.Router.Gateways[0].Namespace)).To(Equal(testNs.Name))
				g.Expect(current.Status.Router.Gateways[0].HTTPRoutes).To(BeEmpty(),
					"no KServe-managed HTTPRoute exists in this shape")

				g.Expect(current.Status.Router.Scheduler).ToNot(BeNil())
				g.Expect(current.Status.Router.Scheduler.InferencePool).ToNot(BeNil())
				g.Expect(string(current.Status.Router.Scheduler.InferencePool.Name)).To(Equal(svcName + "-inference-pool"))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
