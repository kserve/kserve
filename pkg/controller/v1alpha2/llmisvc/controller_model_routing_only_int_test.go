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
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

// countPerModelPathMatches counts matches that reach llmSvc through one of its
// per-model URL prefixes without the model-routing header.
func countPerModelPathMatches(rules []gwapiv1.HTTPRouteRule, llmSvc *v1alpha2.LLMInferenceService) int {
	const modelRoutingHeader = "X-Gateway-Model-Name"
	prefixes := []string{
		"/" + llmSvc.Namespace + "/" + llmSvc.Name,
		"/" + publisherModel(llmSvc.Namespace, *llmSvc.Spec.Model.Name),
	}
	count := 0
	for _, rule := range rules {
		for _, match := range rule.Matches {
			if match.Path == nil || match.Path.Value == nil || slices.ContainsFunc(match.Headers, func(h gwapiv1.HTTPHeaderMatch) bool {
				return strings.EqualFold(string(h.Name), modelRoutingHeader)
			}) {
				continue
			}
			for _, prefix := range prefixes {
				if *match.Path.Value == prefix || strings.HasPrefix(*match.Path.Value, prefix+"/") {
					count++
					break
				}
			}
		}
	}
	return count
}

// expectPerModelPathsDropped waits for the stored service's PerModelPathsDropped
// condition to have status and reason; an empty status means absent.
func expectPerModelPathsDropped(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, status corev1.ConditionStatus, reason string) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) {
		current := &v1alpha2.LLMInferenceService{}
		g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		cond := current.Status.GetCondition(v1alpha2.PerModelPathsDropped)
		if status == "" {
			g.Expect(cond).To(BeNil())
			return
		}
		g.Expect(cond).NotTo(BeNil())
		g.Expect(cond.Status).To(Equal(status))
		g.Expect(cond.Reason).To(Equal(reason))
		g.Expect(cond.Message).NotTo(BeEmpty())
	}).WithContext(ctx).Should(Succeed())
}

var _ = Describe("Model Based Routing Only", func() {
	const headerName = "X-Gateway-Model-Name"

	modelRoutingOnly := map[string]string{llmisvc.AnnotationModelBasedRoutingOnly: "true"}

	createGateway := func(ctx context.Context, testNs *TestNamespace, name string, annotations map[string]string, address string) *gwapiv1.Gateway {
		gw := Gateway(name,
			InNamespace[*gwapiv1.Gateway](testNs.Name),
			WithListener(gwapiv1.HTTPProtocolType),
		)
		gw.Annotations = annotations
		Expect(envTest.Client.Create(ctx, gw)).To(Succeed())
		ensureGatewayReady(ctx, envTest.Client, gw)
		setGatewayStatusAddresses(ctx, envTest.Client, gw, address)
		return gw
	}

	createService := func(ctx context.Context, testNs *TestNamespace, name string, opts ...LLMInferenceServiceOption) *v1alpha2.LLMInferenceService {
		llmSvc := LLMInferenceService(name, append([]LLMInferenceServiceOption{
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithModelName("facebook/opt-125m"),
			WithManagedRoute(),
		}, opts...)...)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		return llmSvc
	}

	It("should drop per-model path matches when the only Gateway is set to model-based routing only", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "path-model-only-gw", modelRoutingOnly, "203.0.113.60")

		// when
		llmSvc := createService(ctx, testNs, "test-path-model-only",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))

			rules := routes[0].Spec.Rules
			g.Expect(countPerModelPathMatches(rules, llmSvc)).To(BeZero(), "per-model path matches should be gone")
			g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, publisherModel(testNs.Name, "facebook/opt-125m")))
			for _, rule := range rules {
				for _, match := range rule.Matches {
					g.Expect(match.Headers).NotTo(BeEmpty(), "only the model-routing tree should remain, got rule %v", rule.Name)
				}
			}
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnAllGateways")

		ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)
		Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())

		// then - status.url is the gateway root, the base URL for body-routed clients
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			g.Expect(current.Status.URL).NotTo(BeNil())
			g.Expect(current.Status.URL.String()).To(Equal("http://203.0.113.60/"))
			g.Expect(current.Status.Addresses).NotTo(BeEmpty())
			for _, addr := range current.Status.Addresses {
				g.Expect(addr.URL.Path).NotTo(HavePrefix("/"+testNs.Name+"/"), "no address should advertise a per-model path")
				g.Expect(addr.URL.Path).NotTo(HavePrefix("/publishers/"), "no address should advertise a per-model path")
			}
		}).WithContext(ctx).Should(Succeed())
	})

	It("should keep per-model path matches when another parent Gateway still serves them", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		modelOnly := createGateway(ctx, testNs, "path-mixed-model-only-gw", modelRoutingOnly, "203.0.113.61")
		regular := createGateway(ctx, testNs, "path-mixed-regular-gw", nil, "203.0.113.62")

		// when
		llmSvc := createService(ctx, testNs, "test-path-mixed",
			WithGatewayRefs(
				LLMGatewayRef(modelOnly.Name, testNs.Name),
				LLMGatewayRef(regular.Name, testNs.Name),
			),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(routes[0].Spec.ParentRefs).To(HaveLen(2))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeNumerically(">", 0))
			g.Expect(countModelRoutingRules(routes[0].Spec.Rules)).To(BeNumerically(">", 0))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionFalse, "NotSetOnAllGateways")
	})

	It("should drop per-model path matches when every parent Gateway is set to model-based routing only", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		first := createGateway(ctx, testNs, "path-all-model-only-gw-1", modelRoutingOnly, "203.0.113.63")
		second := createGateway(ctx, testNs, "path-all-model-only-gw-2", modelRoutingOnly, "203.0.113.64")

		// when
		llmSvc := createService(ctx, testNs, "test-path-all-model-only",
			WithGatewayRefs(
				LLMGatewayRef(first.Name, testNs.Name),
				LLMGatewayRef(second.Name, testNs.Name),
			),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeZero())
			g.Expect(countModelRoutingRules(routes[0].Spec.Rules)).To(BeNumerically(">", 0))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnAllGateways")
	})

	It("should let a service opt in behind a Gateway without the setting", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "service-opt-in-gw", nil, "203.0.113.70")

		// when
		llmSvc := createService(ctx, testNs, "test-service-opt-in",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
			WithSpecAnnotations(modelRoutingOnly),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeZero())
			g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, publisherModel(testNs.Name, "facebook/opt-125m")))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnService")

		// then - the setting is routing-only, so it stays off the pod template
		// and toggling it never rolls the workload
		deployment := &appsv1.Deployment{}
		Eventually(func(g Gomega, ctx context.Context) {
			g.Expect(envTest.Get(ctx, k8stypes.NamespacedName{Name: llmSvc.Name + "-kserve", Namespace: testNs.Name}, deployment)).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
		Expect(deployment.Spec.Template.Annotations).NotTo(HaveKey(llmisvc.AnnotationModelBasedRoutingOnly))
	})

	It("should let a service opt out of a Gateway-wide setting", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "service-opt-out-gw", modelRoutingOnly, "203.0.113.71")

		// when
		llmSvc := createService(ctx, testNs, "test-service-opt-out",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
			WithSpecAnnotations(map[string]string{llmisvc.AnnotationModelBasedRoutingOnly: "false"}),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeNumerically(">", 0))
			g.Expect(countModelRoutingRules(routes[0].Spec.Rules)).To(BeNumerically(">", 0))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionFalse, "DisabledOnService")
	})

	It("should take the service setting from a preset", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "preset-opt-in-gw", nil, "203.0.113.72")
		preset := LLMInferenceServiceConfig("model-routing-only",
			InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
			WithConfigSpecAnnotations(modelRoutingOnly),
		)
		Expect(envTest.Client.Create(ctx, preset)).To(Succeed())

		// when
		llmSvc := createService(ctx, testNs, "test-preset-opt-in",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
			WithBaseRefs(corev1.LocalObjectReference{Name: preset.Name}),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeZero())
			g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, publisherModel(testNs.Name, "facebook/opt-125m")))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnService")
	})

	It("should keep a routing group's split when only one member is model-based routing only", func(ctx SpecContext) {
		// given - one member drops its per-model paths, so its status lists only
		// publisher-qualified model names while its peer also lists the plain one
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "group-mixed-gw", nil, "203.0.113.73")
		groupName := "mixed-group"

		member := func(name string, weight int32, opts ...LLMInferenceServiceOption) *v1alpha2.LLMInferenceService {
			return createService(ctx, testNs, name, append([]LLMInferenceServiceOption{
				WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
				WithManagedScheduler(),
				WithGroup(groupName),
				WithWeight(weight),
			}, opts...)...)
		}

		// when
		modelOnly := member("test-group-model-only", 80, WithSpecAnnotations(modelRoutingOnly))
		regular := member("test-group-regular", 20)
		defer func() {
			testNs.DeleteAndWait(ctx, modelOnly)
			testNs.DeleteAndWait(ctx, regular)
		}()
		ensureRouterManagedResourcesAreReady(ctx, envTest.Client, modelOnly)
		ensureRouterManagedResourcesAreReady(ctx, envTest.Client, regular)

		// then - both routes split across both members, and neither member reports divergence
		for _, svc := range []*v1alpha2.LLMInferenceService{modelOnly, regular} {
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, svc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				backendRefs := groupRoutingBackendRefs(&routes[0], svc)
				g.Expect(backendRefs).To(ContainElement(SatisfyAll(HaveBackendName(modelOnly.Name), HaveBackendWeight(int32(80)))))
				g.Expect(backendRefs).To(ContainElement(SatisfyAll(HaveBackendName(regular.Name), HaveBackendWeight(int32(20)))))

				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())
				if degraded := current.Status.GetCondition(v1alpha2.GroupDegraded); degraded != nil {
					g.Expect(degraded.IsTrue()).To(BeFalse(), "%s: %s", degraded.Reason, degraded.Message)
				}
			}).WithContext(ctx).Should(Succeed())
		}

		expectPerModelPathsDropped(ctx, modelOnly, corev1.ConditionTrue, "SetOnService")
		expectPerModelPathsDropped(ctx, regular, "", "")

		// then - the model-only member really dropped its per-model paths
		routes, err := managedRoutes(ctx, modelOnly)
		Expect(err).NotTo(HaveOccurred())
		Expect(countPerModelPathMatches(routes[0].Spec.Rules, modelOnly)).To(BeZero())
	})

	It("should keep per-model path matches when model-based routing is off for the service", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "path-model-only-no-mbr-gw", modelRoutingOnly, "203.0.113.65")

		// when
		llmSvc := createService(ctx, testNs, "test-path-model-only-no-mbr",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
			WithSpecAnnotations(map[string]string{
				llmisvc.AnnotationModelBasedRoutingEnabled: "false",
			}),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then - dropping the paths as well would leave the model unreachable
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(countModelRoutingRules(routes[0].Spec.Rules)).To(BeZero())
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeNumerically(">", 0))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionFalse, "ModelBasedRoutingNotEnabled")

		ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)
		Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())
	})

	It("should keep LoRA adapter matches in the model-routing tree", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "path-model-only-lora-gw", modelRoutingOnly, "203.0.113.66")

		// when
		llmSvc := createService(ctx, testNs, "test-path-model-only-lora",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
			WithLoRAAdapters("lora-adapter-a", "lora-adapter-b"),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeZero())
			g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, publisherModel(testNs.Name, "facebook/opt-125m")))
			g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, publisherModel(testNs.Name, "lora-adapter-a")))
			g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, publisherModel(testNs.Name, "lora-adapter-b")))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnAllGateways")
	})

	It("should honour the setting on a parent that comes from the route spec rather than gateway refs", func(ctx SpecContext) {
		// given - the shape of a service on the default ingress Gateway: the
		// parent is only in the route spec, spec.router.gateway.refs is empty
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "path-model-only-route-parent-gw", modelRoutingOnly, "203.0.113.68")
		svcName := "test-path-model-only-route-parent"

		routeSpec := HTTPRoute("inline",
			InNamespace[*gwapiv1.HTTPRoute](testNs.Name),
			WithParentRef(GatewayParentRef(gw.Name, testNs.Name)),
			WithHTTPRule(
				Matches(PathPrefixMatch("/"+testNs.Name+"/"+svcName)),
				WithBackendRefs(ServiceRef("custom-backend", 8000, 1)),
			),
			WithHTTPRule(
				Matches(ExactPathWithHeaderMatch("/v1/completions", headerName, publisherModel(testNs.Name, "facebook/opt-125m"))),
				WithBackendRefs(ServiceRef("custom-backend", 8000, 1)),
			),
		).Spec

		// when
		llmSvc := LLMInferenceService(svcName,
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithModelName("facebook/opt-125m"),
			WithHTTPRouteSpec(&routeSpec),
			WithSpecAnnotations(map[string]string{
				llmisvc.AnnotationModelBasedRoutingEnabled: "true",
			}),
		)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(BeZero())
			g.Expect(&routes[0]).To(HaveHeaderMatch(headerName, publisherModel(testNs.Name, "facebook/opt-125m")))
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnAllGateways")
	})

	It("should keep per-model path matches when the route has no model-routing match to fall back on", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "path-model-only-no-header-tree-gw", modelRoutingOnly, "203.0.113.69")
		svcName := "test-path-model-only-no-header-tree"

		routeSpec := HTTPRoute("inline",
			InNamespace[*gwapiv1.HTTPRoute](testNs.Name),
			WithParentRef(GatewayParentRef(gw.Name, testNs.Name)),
			WithHTTPRule(
				Matches(PathPrefixMatch("/"+testNs.Name+"/"+svcName)),
				WithBackendRefs(ServiceRef("custom-backend", 8000, 1)),
			),
		).Spec

		// when
		llmSvc := LLMInferenceService(svcName,
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithModelName("facebook/opt-125m"),
			WithHTTPRouteSpec(&routeSpec),
			WithSpecAnnotations(map[string]string{
				llmisvc.AnnotationModelBasedRoutingEnabled: "true",
			}),
		)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then - stripping would empty the rules, which the API server
		// defaults to a backendless PathPrefix / rule
		Eventually(func(g Gomega, ctx context.Context) {
			routes, err := managedRoutes(ctx, llmSvc)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
			g.Expect(routes[0].Spec.Rules).To(HaveLen(1))
			g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(Equal(1))
			g.Expect(routes[0].Spec.Rules[0].BackendRefs).NotTo(BeEmpty())
		}).WithContext(ctx).Should(Succeed())
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionFalse, "NoModelRoutingMatches")
	})

	It("should follow the Gateway as the setting is added and removed", func(ctx SpecContext) {
		// given - a settled service, so the Gateway watch is the only thing
		// that can move the route
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "path-model-only-flip-gw", nil, "203.0.113.67")

		llmSvc := createService(ctx, testNs, "test-path-model-only-flip",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)
		Eventually(LLMInferenceServiceIsReady(llmSvc)).WithContext(ctx).Should(Succeed())

		setModelRoutingOnly := func(enabled bool) {
			current := &gwapiv1.Gateway{}
			Expect(envTest.Client.Get(ctx, client.ObjectKeyFromObject(gw), current)).To(Succeed())
			patch := client.MergeFrom(current.DeepCopy())
			if enabled {
				current.Annotations = map[string]string{llmisvc.AnnotationModelBasedRoutingOnly: "true"}
			} else {
				delete(current.Annotations, llmisvc.AnnotationModelBasedRoutingOnly)
			}
			Expect(envTest.Client.Patch(ctx, current, patch)).To(Succeed())
		}
		expectRouting := func(pathMatches types.GomegaMatcher, statusURL string) {
			Eventually(func(g Gomega, ctx context.Context) {
				routes, err := managedRoutes(ctx, llmSvc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(routes).To(HaveLen(1))
				g.Expect(countPerModelPathMatches(routes[0].Spec.Rules, llmSvc)).To(pathMatches)

				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.URL).NotTo(BeNil())
				g.Expect(current.Status.URL.String()).To(Equal(statusURL))
			}).WithContext(ctx).Should(Succeed())
		}
		expectRouting(BeNumerically(">", 0), "http://203.0.113.67/"+testNs.Name+"/"+llmSvc.Name)
		expectPerModelPathsDropped(ctx, llmSvc, "", "")

		// when
		setModelRoutingOnly(true)

		// then
		expectRouting(BeZero(), "http://203.0.113.67/")
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnAllGateways")

		// when
		setModelRoutingOnly(false)

		// then
		expectRouting(BeNumerically(">", 0), "http://203.0.113.67/"+testNs.Name+"/"+llmSvc.Name)
		expectPerModelPathsDropped(ctx, llmSvc, "", "")
	})

	It("should clear the condition when the service is stopped", func(ctx SpecContext) {
		// given
		testNs := NewTestNamespace(ctx, envTest)
		gw := createGateway(ctx, testNs, "stop-model-only-gw", modelRoutingOnly, "203.0.113.74")
		llmSvc := createService(ctx, testNs, "test-stop-model-only",
			WithGatewayRefs(LLMGatewayRef(gw.Name, testNs.Name)),
		)
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()
		expectPerModelPathsDropped(ctx, llmSvc, corev1.ConditionTrue, "SetOnAllGateways")

		// when
		current := &v1alpha2.LLMInferenceService{}
		Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		patch := client.MergeFrom(current.DeepCopy())
		if current.Annotations == nil {
			current.Annotations = map[string]string{}
		}
		current.Annotations[constants.StopAnnotationKey] = "true"
		Expect(envTest.Client.Patch(ctx, current, patch)).To(Succeed())

		// then
		expectPerModelPathsDropped(ctx, llmSvc, "", "")
	})
})
