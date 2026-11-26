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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("ListenerSet route parents", func() {
	// createListenerSetIngress creates a Programmed Gateway and a ListenerSet adding a
	// tenant listener to it. The Gateway's own listener differs in port and hostname,
	// so a URL built from the wrong listener shows.
	createListenerSetIngress := func(ctx context.Context, ns string) (*gwapiv1.Gateway, *gwapiv1.ListenerSet) {
		gateway := Gateway("shared-gateway",
			InNamespace[*gwapiv1.Gateway](ns),
			WithListeners(gwapiv1.Listener{
				Name:     "gateway-fallback",
				Port:     8443,
				Protocol: gwapiv1.HTTPSProtocolType,
				Hostname: ptr.To(gwapiv1.Hostname("fallback.example.com")),
			}),
			WithAllowedListeners(gwapiv1.NamespacesFromSame),
		)
		Expect(envTest.Create(ctx, gateway)).To(Succeed())
		WithAddresses("203.0.113.10")(gateway)
		WithGatewayReadyStatus()(gateway)
		Expect(envTest.Client.Status().Update(ctx, gateway)).To(Succeed())

		listenerSet := ListenerSet("tenant-https", gateway.Name, ns,
			InNamespace[*gwapiv1.ListenerSet](ns),
			WithListenerEntries(gwapiv1.ListenerEntry{
				Name:     "tenant-https",
				Port:     443,
				Protocol: gwapiv1.HTTPSProtocolType,
				Hostname: ptr.To(gwapiv1.Hostname("*.tenant.example.com")),
			}),
		)
		Expect(envTest.Create(ctx, listenerSet)).To(Succeed())
		return gateway, listenerSet
	}

	// createListenerSetRoutedService creates an HTTPRoute parented to listenerSet and
	// a service with a managed scheduler that references it.
	createListenerSetRoutedService := func(ctx context.Context, name string, listenerSet *gwapiv1.ListenerSet, routeOpts ...HTTPRouteOption) (*gwapiv1.HTTPRoute, *v1alpha2.LLMInferenceService) {
		route := HTTPRoute(name, append([]HTTPRouteOption{
			InNamespace[*gwapiv1.HTTPRoute](listenerSet.Namespace),
			WithParentRef(ListenerSetParentRef(listenerSet.Name, listenerSet.Namespace)),
			WithHTTPRouteRule(HTTPRouteRule(WithBackendRefs(BackendRefInferencePool(name + "-inference-pool")))),
		}, routeOpts...)...)
		Expect(envTest.Create(ctx, route)).To(Succeed())
		ensureHTTPRouteReady(ctx, envTest.Client, route)

		llmSvc := LLMInferenceService(name,
			InNamespace[*v1alpha2.LLMInferenceService](listenerSet.Namespace),
			WithModelURI("hf://facebook/opt-125m"),
			WithHTTPRouteRefs(HTTPRouteRef(route.Name)),
			WithManagedScheduler(),
		)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		return route, llmSvc
	}

	// markPoolAcceptedBy writes the pool status a Gateway controller writes. For a route
	// attached through a ListenerSet the pool parent is the Gateway.
	markPoolAcceptedBy := func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, gatewayName, gatewayNamespace string) {
		Eventually(func(g Gomega, ctx context.Context) {
			pools, err := managedInferencePools(ctx, llmSvc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(pools.Items).To(HaveLen(1))
			pool := pools.Items[0].DeepCopy()
			WithInferencePoolReadyStatus(igwapi.ParentReference{
				Group:     ptr.To(igwapi.Group(gwapiv1.GroupName)),
				Kind:      "Gateway",
				Name:      igwapi.ObjectName(gatewayName),
				Namespace: igwapi.Namespace(gatewayNamespace),
			})(pool)
			g.Expect(envTest.Client.Status().Update(ctx, pool)).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
	}

	// markWorkloadsReady stands in for the Deployment controller.
	markWorkloadsReady := func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) {
		Eventually(func(g Gomega, ctx context.Context) {
			g.Expect(ensureSchedulerDeploymentReady(ctx, envTest.Client, llmSvc)).To(Succeed())
			g.Expect(ensureMainDeploymentAvailable(ctx, envTest.Client, llmSvc)).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
	}

	// waitForSettledStatus waits until the service's resourceVersion holds still, so a
	// reconcile queued by an earlier write cannot run after the next step.
	waitForSettledStatus := func(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) {
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			resourceVersion := current.ResourceVersion
			g.Consistently(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.ResourceVersion).To(Equal(resourceVersion))
			}).WithContext(ctx).WithTimeout(2 * time.Second).Should(Succeed())
		}).WithContext(ctx).WithTimeout(time.Minute).Should(Succeed())
	}

	statusURL := func(llmSvc *v1alpha2.LLMInferenceService) func(g Gomega, ctx context.Context) string {
		return func(g Gomega, ctx context.Context) string {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			return current.Status.URL.String()
		}
	}

	It("reaches RouterReady with a referenced HTTPRoute parented to a ListenerSet", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		gateway, listenerSet := createListenerSetIngress(ctx, testNs.Name)
		defer testNs.DeleteAndWait(ctx, gateway)
		defer testNs.DeleteAndWait(ctx, listenerSet)
		route, llmSvc := createListenerSetRoutedService(ctx, "listenerset-route-ref", listenerSet,
			WithHostnames("model.tenant.example.com"))
		defer testNs.DeleteAndWait(ctx, route)
		defer testNs.DeleteAndWait(ctx, llmSvc)

		markWorkloadsReady(ctx, llmSvc)
		markPoolAcceptedBy(ctx, llmSvc, gateway.Name, gateway.Namespace)

		Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
			g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"))
			g.Expect(current.Status.URL.String()).To(Equal("https://model.tenant.example.com/"))
			g.Expect(current.Status.Router.Gateways).To(ConsistOf(And(
				HaveField("Kind", gwapiv1.Kind("ListenerSet")),
				HaveField("Name", gwapiv1.ObjectName(listenerSet.Name)),
			)))
		})).WithContext(ctx).Should(Succeed())
	})

	It("stays WaitingForGateway until the ListenerSet's Gateway accepts the pool", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		gateway, listenerSet := createListenerSetIngress(ctx, testNs.Name)
		defer testNs.DeleteAndWait(ctx, gateway)
		defer testNs.DeleteAndWait(ctx, listenerSet)
		route, llmSvc := createListenerSetRoutedService(ctx, "listenerset-pool-scope", listenerSet)
		defer testNs.DeleteAndWait(ctx, route)
		defer testNs.DeleteAndWait(ctx, llmSvc)

		markPoolAcceptedBy(ctx, llmSvc, "other-gateway", testNs.Name)

		waitingForGateway := func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			cond := current.Status.GetCondition(v1alpha2.InferencePoolReady)
			g.Expect(cond).ToNot(BeNil())
			g.Expect(cond.IsFalse()).To(BeTrue())
			g.Expect(cond.Reason).To(Equal("WaitingForGateway"))
		}
		Eventually(waitingForGateway).WithContext(ctx).Should(Succeed())
		waitForSettledStatus(ctx, llmSvc)
		Consistently(waitingForGateway).WithContext(ctx).WithTimeout(3 * time.Second).Should(Succeed())

		markPoolAcceptedBy(ctx, llmSvc, gateway.Name, gateway.Namespace)

		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"))
		}).WithContext(ctx).Should(Succeed())
	})

	It("picks up the Gateway's address when it is published after the service reconciled", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		gateway, listenerSet := createListenerSetIngress(ctx, testNs.Name)
		defer testNs.DeleteAndWait(ctx, gateway)
		defer testNs.DeleteAndWait(ctx, listenerSet)
		gateway.Status.Addresses = nil
		Expect(envTest.Client.Status().Update(ctx, gateway)).To(Succeed())
		route, llmSvc := createListenerSetRoutedService(ctx, "listenerset-late-address", listenerSet,
			WithHostnames("late.tenant.example.com"))
		defer testNs.DeleteAndWait(ctx, route)
		defer testNs.DeleteAndWait(ctx, llmSvc)

		markWorkloadsReady(ctx, llmSvc)
		markPoolAcceptedBy(ctx, llmSvc, gateway.Name, gateway.Namespace)
		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			g.Expect(current.Status).To(HaveCondition(string(v1alpha2.InferencePoolReady), "True"))
			g.Expect(current.Status.URL).To(BeNil())
		}).WithContext(ctx).Should(Succeed())
		waitForSettledStatus(ctx, llmSvc)

		WithAddresses("203.0.113.20")(gateway)
		Expect(envTest.Client.Status().Update(ctx, gateway)).To(Succeed())

		Eventually(statusURL(llmSvc)).WithContext(ctx).Should(Equal("https://late.tenant.example.com/"))
	})

	It("follows a ListenerSet listener's hostname into status.url", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		gateway, listenerSet := createListenerSetIngress(ctx, testNs.Name)
		defer testNs.DeleteAndWait(ctx, gateway)
		defer testNs.DeleteAndWait(ctx, listenerSet)
		// No route hostnames, so the URL is built from the listener's hostname.
		route, llmSvc := createListenerSetRoutedService(ctx, "listenerset-hostname", listenerSet)
		defer testNs.DeleteAndWait(ctx, route)
		defer testNs.DeleteAndWait(ctx, llmSvc)

		Eventually(statusURL(llmSvc)).WithContext(ctx).Should(Equal("https://inference.tenant.example.com/"))
		waitForSettledStatus(ctx, llmSvc)

		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(listenerSet), listenerSet); err != nil {
				return err
			}
			listenerSet.Spec.Listeners[0].Hostname = ptr.To(gwapiv1.Hostname("*.renamed.example.com"))
			return envTest.Update(ctx, listenerSet)
		})).To(Succeed())

		Eventually(statusURL(llmSvc)).WithContext(ctx).Should(Equal("https://inference.renamed.example.com/"))
	})

	It("reaches RouterReady with a ListenerSet parent on the managed HTTPRoute spec", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		gateway, listenerSet := createListenerSetIngress(ctx, testNs.Name)
		defer testNs.DeleteAndWait(ctx, gateway)
		defer testNs.DeleteAndWait(ctx, listenerSet)

		llmSvc := LLMInferenceService("listenerset-managed-route",
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedRoute(),
			WithHTTPRouteSpec(&gwapiv1.HTTPRouteSpec{
				CommonRouteSpec: gwapiv1.CommonRouteSpec{
					ParentRefs: []gwapiv1.ParentReference{ListenerSetParentRef(listenerSet.Name, testNs.Name)},
				},
				Hostnames: []gwapiv1.Hostname{"managed.tenant.example.com"},
			}),
			WithManagedScheduler(),
		)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer testNs.DeleteAndWait(ctx, llmSvc)

		var routes []gwapiv1.HTTPRoute
		Eventually(func(g Gomega, ctx context.Context) {
			var err error
			routes, err = managedRoutes(ctx, llmSvc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(routes).To(HaveLen(1))
		}).WithContext(ctx).Should(Succeed())
		// Only the route: ensureRouterManagedResourcesAreReady would write a pool
		// parent named after the ListenerSet, which a wrong pool scope would accept.
		ensureHTTPRouteReady(ctx, envTest.Client, &routes[0])
		markWorkloadsReady(ctx, llmSvc)
		markPoolAcceptedBy(ctx, llmSvc, gateway.Name, gateway.Namespace)

		Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
			g.Expect(current.Status.URL.String()).To(Equal("https://managed.tenant.example.com/"))
		})).WithContext(ctx).Should(Succeed())
	})
})
