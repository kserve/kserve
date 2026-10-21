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
	"fmt"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

// pathRules returns n single-match path rules routed to backend.
func pathRules(n int, backend string) []gwapiv1.HTTPRouteRule {
	rules := make([]gwapiv1.HTTPRouteRule, 0, n)
	for i := range n {
		rules = append(rules, gwapiv1.HTTPRouteRule{
			Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{
				Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
				Value: ptr.To(fmt.Sprintf("/shard-test/%d", i)),
			}}},
			BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{
					Kind: ptr.To(gwapiv1.Kind("Service")),
					Name: gwapiv1.ObjectName(backend),
					Port: ptr.To(gwapiv1.PortNumber(8000)),
				},
			}}},
		})
	}
	return rules
}

// sortedManagedRoutes lists the service's managed HTTPRoutes ordered by name,
// which matches shard order for fewer than ten shards.
func sortedManagedRoutes(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) ([]gwapiv1.HTTPRoute, error) {
	routes, err := managedRoutes(ctx, llmSvc)
	slices.SortFunc(routes, func(a, b gwapiv1.HTTPRoute) int { return strings.Compare(a.Name, b.Name) })
	return routes, err
}

var _ = Describe("LLMInferenceService HTTPRoute sharding", func() {
	It("should split an inline route over the Gateway API rule limit and clean up when it shrinks", func(ctx SpecContext) {
		// given
		svcName := "sharded-route"
		testNs := NewTestNamespace(ctx, envTest)

		spec := customRouteSpec(ctx, envTest.Client, testNs.Name, "shard-gateway", "shard-backend")
		spec.Hostnames = []gwapiv1.Hostname{"models.example.com"}
		spec.Rules = pathRules(20, "shard-backend")

		llmSvc := LLMInferenceService(svcName,
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedGateway(),
			WithHTTPRouteSpec(spec),
			WithSpecAnnotations(map[string]string{constants.HTTPRouteMaxShardsAnnotationKey: "3"}),
		)

		// when
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		var routes []gwapiv1.HTTPRoute
		Eventually(func(g Gomega, ctx context.Context) {
			var err error
			routes, err = sortedManagedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(2))
		}).WithContext(ctx).Should(Succeed())

		Expect(routes[0].Name).To(Equal(svcName + "-kserve-route"))
		Expect(routes[0].Labels).NotTo(HaveKey(constants.LLMHTTPRouteShardLabelKey))
		Expect(routes[0].Spec.Rules).To(HaveLen(16))
		Expect(routes[1].Name).To(Equal(svcName + "-kserve-route-1"))
		Expect(routes[1].Labels).To(HaveKeyWithValue(constants.LLMHTTPRouteShardLabelKey, "1"))
		Expect(routes[1].Spec.Rules).To(HaveLen(4))
		for i := range routes {
			Expect(&routes[i]).To(BeControlledBy(llmSvc))
			Expect(&routes[i]).To(HaveGatewayRefs(gwapiv1.ParentReference{Name: "shard-gateway"}))
			Expect(routes[i].Spec.Hostnames).To(ConsistOf(gwapiv1.Hostname("models.example.com")))
		}

		// Readiness covers every shard.
		for i := range routes {
			updated := routes[i].DeepCopy()
			WithHTTPRouteReadyStatus(DefaultGatewayControllerName)(updated)
			Expect(envTest.Client.Status().Update(ctx, updated)).To(Succeed())
		}
		Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
			g.Expect(current.Status).To(HaveCondition(string(v1alpha2.HTTPRoutesReady), "True"))
		})).WithContext(ctx).Should(Succeed())

		// when the logical route grows past two resources
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current); err != nil {
				return err
			}
			current.Spec.Router.Route.HTTP.Spec.Rules = pathRules(40, "shard-backend")
			return envTest.Update(ctx, current)
		})).To(Succeed())

		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := sortedManagedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(3))
			g.Expect(routes[2].Name).To(Equal(svcName + "-kserve-route-2"))
			g.Expect(routes[2].Spec.Rules).To(HaveLen(8))
		}).WithContext(ctx).Should(Succeed())

		// when the logical route shrinks back into one resource
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current); err != nil {
				return err
			}
			current.Spec.Router.Route.HTTP.Spec.Rules = pathRules(3, "shard-backend")
			return envTest.Update(ctx, current)
		})).To(Succeed())

		// then the extra shards are deleted and the base route keeps its name
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := sortedManagedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(routes[0].Name).To(Equal(svcName + "-kserve-route"))
			g.Expect(routes[0].Spec.Rules).To(HaveLen(3))
		}).WithContext(ctx).Should(Succeed())
	})

	It("should admit a large inline route but not shard it past the configured cap", func(ctx SpecContext) {
		// given the default cap of one HTTPRoute
		testNs := NewTestNamespace(ctx, envTest)
		spec := customRouteSpec(ctx, envTest.Client, testNs.Name, "cap-gateway", "cap-backend")
		spec.Rules = pathRules(20, "cap-backend")

		llmSvc := LLMInferenceService("over-shard-cap",
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedGateway(),
			WithHTTPRouteSpec(spec),
		)

		// when: more than 16 rules pass admission, the inline schema allows up to 256
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then nothing is written and the condition names the cap
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			cond := current.Status.GetCondition(v1alpha2.HTTPRoutesReady)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.IsFalse()).To(BeTrue())
			g.Expect(cond.Reason).To(Equal("RoutingPreconditionNotMet"))
			g.Expect(cond.Message).To(ContainSubstring("needs 20 rules (at most 16 per HTTPRoute)"))
			g.Expect(cond.Message).To(ContainSubstring("allows 1 HTTPRoutes"))
		}).WithContext(ctx).Should(Succeed())
		routes, err := sortedManagedRoutes(ctx, llmSvc)
		Expect(err).NotTo(HaveOccurred())
		Expect(routes).To(BeEmpty())
	})
})
