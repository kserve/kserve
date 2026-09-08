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
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

const (
	loraRoutingStrategyKey = "loraModelRoutingStrategy"
	loraBaseModel          = "base-model"
)

// eventuallyManagedRoute polls until the service's single managed HTTPRoute
// exists and satisfies inspect, collapsing the fetch-and-assert boilerplate so
// specs read in terms of the route under test.
func eventuallyManagedRoute(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, inspect func(g Gomega, route *gwapiv1.HTTPRoute)) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) {
		routes, err := managedRoutes(ctx, llmSvc)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(routes).To(HaveLen(1))
		inspect(g, &routes[0])
	}).WithContext(ctx).Should(Succeed())
}

// unrecognizedModelMatchRoute is a user-supplied route whose model-routing
// match is not the generated base-model identity, so the regex strategy cannot
// rewrite it: the route-shape precondition failure a live cluster can reach.
func unrecognizedModelMatchRoute(namespace string) *v1alpha2.HTTPRouteSpec {
	return &v1alpha2.HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{
		Rules: []gwapiv1.HTTPRouteRule{{Matches: []gwapiv1.HTTPRouteMatch{{
			Path:    &gwapiv1.HTTPPathMatch{Type: ptr.To(gwapiv1.PathMatchExact), Value: ptr.To("/v1/completions")},
			Headers: []gwapiv1.HTTPHeaderMatch{{Name: "X-Gateway-Model-Name", Value: publisherModel(namespace, "other")}},
		}}}},
	}}
}

func patchIngressLoraRoutingStrategy(ctx context.Context, strategy string) {
	PatchIngressConfigKey(ctx, envTest.Client, loraRoutingStrategyKey, strategy)
}

func restoreIngressLoraRoutingStrategy(ctx context.Context) {
	PatchIngressConfigKey(ctx, envTest.Client, loraRoutingStrategyKey, nil)
}

// loraRegexPattern builds the expected rendered pattern for the services this
// file creates, independently of the production renderer. Only valid for names
// without regex metacharacters.
func loraRegexPattern(namespace string, sortedAdapters ...string) string {
	return "^publishers/" + namespace + "/models/(" + loraBaseModel + "|" + strings.Join(sortedAdapters, "|") + ")$"
}

// modelRoutingHeaderMatches collects every header match for the model-routing
// header across all rules of the route.
func modelRoutingHeaderMatches(route *gwapiv1.HTTPRoute) []gwapiv1.HTTPHeaderMatch {
	const headerName = "X-Gateway-Model-Name"
	var out []gwapiv1.HTTPHeaderMatch
	for _, rule := range route.Spec.Rules {
		for _, match := range rule.Matches {
			for _, h := range match.Headers {
				if string(h.Name) == headerName {
					out = append(out, h)
				}
			}
		}
	}
	return out
}

var _ = Describe("LoRA model routing strategy", func() {
	const headerName = "X-Gateway-Model-Name"

	// Group cleanup error classification is covered by the group cleanup
	// isolation specs; this only pins what an unsupported value does once it
	// reaches the reconciler. The fake client runs no admission webhook, so an
	// annotation the webhook would reject gets through; a ConfigMap typo never
	// does, it fails config loading (see TestLoadConfigLoRAModelRoutingStrategy).
	It("should treat an unsupported strategy that bypassed admission as a precondition failure, not a retry", func(ctx SpecContext) {
		ns := NewTestNamespace(ctx, envTest)
		svc := LLMInferenceService("unsupported-strategy",
			InNamespace[*v1alpha2.LLMInferenceService](ns.Name),
			WithModelURI("hf://facebook/opt-125m"), WithModelName(loraBaseModel),
			WithLoRAAdapters("adapter"), WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
			WithSpecAnnotations(map[string]string{llmisvc.AnnotationLoRAModelRoutingStrategy: "unsupported"}))
		cm := InferenceServiceCfgMap(constants.KServeNamespace)
		presets := SharedConfigPresets(constants.KServeNamespace)
		objects := make([]client.Object, 0, 4+len(presets))
		objects = append(objects, svc, cm, DefaultGateway(constants.KServeNamespace), DefaultGatewayClass())
		for _, preset := range presets {
			objects = append(objects, preset)
		}
		c := fake.NewClientBuilder().WithScheme(envTest.Client.Scheme()).
			WithRESTMapper(envTest.Client.RESTMapper()).
			WithStatusSubresource(&v1alpha2.LLMInferenceService{}).WithObjects(objects...).Build()
		r := &llmisvc.LLMISVCReconciler{
			Client: c, Clientset: kubernetesfake.NewClientset(cm),
			EventRecorder: record.NewFakeRecorder(100),
		}
		// Repeat the same inputs: an unsupported value is terminal, never a backoff retry.
		for range 2 {
			result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(svc)})
			Expect(errors.Is(err, reconcile.TerminalError(nil))).To(BeTrue(), "got: %v", err)
			Expect(result.IsZero()).To(BeTrue())
			for _, suffix := range []string{"-kserve", "-kserve-router-scheduler"} {
				Expect(c.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: svc.Name + suffix}, &appsv1.Deployment{})).To(Succeed())
			}
			current := &v1alpha2.LLMInferenceService{}
			Expect(c.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())
			condition := current.Status.GetCondition(v1alpha2.HTTPRoutesReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.IsFalse()).To(BeTrue())
			// Distinct from HTTPRouteReconcileError: the strategy cannot be
			// fixed by retrying, only by correcting the annotation.
			Expect(condition.Reason).To(Equal("RoutingPreconditionNotMet"))
			Expect(condition.Message).To(ContainSubstring("unsupported loraModelRoutingStrategy"))
			Expect(condition.Message).To(ContainSubstring(llmisvc.AnnotationLoRAModelRoutingStrategy))
		}
	})

	create := func(ctx SpecContext, ns *TestNamespace, name string, opts ...LLMInferenceServiceOption) *v1alpha2.LLMInferenceService {
		GinkgoHelper()
		svc := LLMInferenceService(name,
			InNamespace[*v1alpha2.LLMInferenceService](ns.Name),
			WithModelURI("hf://facebook/opt-125m"), WithModelName(loraBaseModel),
			WithLoRAAdapters("adapter"), WithManagedRoute(), WithManagedGateway(), WithManagedScheduler())
		for _, opt := range opts {
			opt(svc)
		}
		Expect(envTest.Create(ctx, svc)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { ns.DeleteAndWait(ctx, svc) })
		return svc
	}

	update := func(ctx SpecContext, svc *v1alpha2.LLMInferenceService, mutate func(*v1alpha2.LLMInferenceService)) {
		GinkgoHelper()
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(svc), current); err != nil {
				return err
			}
			mutate(current)
			return envTest.Update(ctx, current)
		})).To(Succeed())
	}

	It("should fan out exact -> regex -> exact without replacing routes", func(ctx SpecContext) {
		DeferCleanup(restoreIngressLoraRoutingStrategy)
		ns := NewTestNamespace(ctx, envTest)
		services := []*v1alpha2.LLMInferenceService{
			create(ctx, ns, "strategy-a"), create(ctx, ns, "strategy-b"),
		}
		identities := map[string]types.UID{}
		for _, svc := range services {
			eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
				g.Expect(route).To(HaveHeaderMatch(headerName, publisherModel(ns.Name, "adapter")))
				identities[svc.Name] = route.UID
			})
		}
		for _, strategy := range []string{"regex", "exact"} {
			patchIngressLoraRoutingStrategy(ctx, strategy)
			for _, svc := range services {
				eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
					g.Expect(route.UID).To(Equal(identities[svc.Name]))
					want := publisherModel(ns.Name, "adapter")
					matchType := gwapiv1.HeaderMatchExact
					if strategy == "regex" {
						want = loraRegexPattern(ns.Name, "adapter")
						matchType = gwapiv1.HeaderMatchRegularExpression
					}
					g.Expect(route).To(HaveHeaderMatch(headerName, want))
					for _, header := range modelRoutingHeaderMatches(route) {
						g.Expect(ptr.Deref(header.Type, gwapiv1.HeaderMatchExact)).To(Equal(matchType))
					}
				})
			}
		}
	})

	It("should return to base-only Exact when the final adapter is removed", func(ctx SpecContext) {
		patchIngressLoraRoutingStrategy(ctx, "regex")
		DeferCleanup(restoreIngressLoraRoutingStrategy)
		ns := NewTestNamespace(ctx, envTest)
		svc := create(ctx, ns, "adapter-removal")
		eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route).To(HaveHeaderMatch(headerName, loraRegexPattern(ns.Name, "adapter")))
		})
		update(ctx, svc, func(current *v1alpha2.LLMInferenceService) { current.Spec.Model.LoRA = nil })
		eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route).To(HaveHeaderMatch(headerName, publisherModel(ns.Name, loraBaseModel)))
			for _, header := range modelRoutingHeaderMatches(route) {
				g.Expect(ptr.Deref(header.Type, gwapiv1.HeaderMatchExact)).To(Equal(gwapiv1.HeaderMatchExact))
				g.Expect(header.Value).NotTo(ContainSubstring("adapter"))
			}
		})
	})

	It("should strip model matches when model-based routing is disabled", func(ctx SpecContext) {
		patchIngressLoraRoutingStrategy(ctx, "regex")
		PatchIngressConfigKey(ctx, envTest.Client, "modelBasedRoutingMode", "disabled")
		DeferCleanup(func(ctx SpecContext) {
			PatchIngressConfigKey(ctx, envTest.Client, "modelBasedRoutingMode", "enabled")
			restoreIngressLoraRoutingStrategy(ctx)
		})
		ns := NewTestNamespace(ctx, envTest)
		svc := create(ctx, ns, "routing-disabled")
		eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(modelRoutingHeaderMatches(route)).To(BeEmpty())
		})
	})

	for _, failure := range []string{"unrecognized-model-match", "oversized-pattern"} {
		It("should report "+failure+" without blocking workload reconciliation, and recover", func(ctx SpecContext) {
			patchIngressLoraRoutingStrategy(ctx, "regex")
			DeferCleanup(restoreIngressLoraRoutingStrategy)
			ns := NewTestNamespace(ctx, envTest)
			svc := create(ctx, ns, "invalid-routing")
			var retained *gwapiv1.HTTPRoute
			eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
				g.Expect(modelRoutingHeaderMatches(route)).NotTo(BeEmpty())
				retained = route.DeepCopy()
			})
			// A precondition failure is terminal; a route the API server
			// rejects (the pattern past the 4096-character header limit) is a
			// plain error and keeps retrying.
			reason := "HTTPRouteReconcileError"
			update(ctx, svc, func(current *v1alpha2.LLMInferenceService) {
				current.Spec.Replicas = ptr.To[int32](2)
				switch failure {
				case "unrecognized-model-match":
					reason = "RoutingPreconditionNotMet"
					current.Spec.Router.Route.HTTP = unrecognizedModelMatchRoute(ns.Name)
				case "oversized-pattern":
					for i := range 9 {
						current.Spec.Model.LoRA.Adapters = append(current.Spec.Model.LoRA.Adapters,
							v1alpha2.LLMModelSpec{Name: ptr.To(fmt.Sprintf("huge-%d-%s", i, strings.Repeat("x", 600))), URI: current.Spec.Model.URI})
					}
				}
			})
			Eventually(func(g Gomega) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())
				condition := current.Status.GetCondition(v1alpha2.HTTPRoutesReady)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.IsFalse()).To(BeTrue())
				g.Expect(condition.Reason).To(Equal(reason))
				g.Expect(condition.Message).To(ContainSubstring("HTTPRoute"))
				deployment := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: svc.Name + "-kserve"}, deployment)).To(Succeed())
				g.Expect(ptr.Deref(deployment.Spec.Replicas, 0)).To(Equal(int32(2)))
			}).WithContext(ctx).Should(Succeed())

			// The last accepted route keeps serving untouched while the candidate is rejected.
			Consistently(func(g Gomega) {
				route := &gwapiv1.HTTPRoute{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(retained), route)).To(Succeed())
				g.Expect(route.ResourceVersion).To(Equal(retained.ResourceVersion))
				g.Expect(modelRoutingHeaderMatches(route)).To(Equal(modelRoutingHeaderMatches(retained)))
			}).WithContext(ctx).WithTimeout(2 * time.Second).Should(Succeed())

			// Route failure must not disable repair of independent dependencies.
			service := &corev1.Service{}
			key := client.ObjectKey{Namespace: ns.Name, Name: svc.Name + "-kserve-workload-svc"}
			Expect(envTest.Get(ctx, key, service)).To(Succeed())
			Expect(envTest.Delete(ctx, service)).To(Succeed())
			Eventually(func() error { return envTest.Get(ctx, key, &corev1.Service{}) }).WithContext(ctx).Should(Succeed())

			update(ctx, svc, func(current *v1alpha2.LLMInferenceService) {
				current.Spec.Router.Route.HTTP = nil // back to the managed template route
				current.Spec.Model.LoRA.Adapters = current.Spec.Model.LoRA.Adapters[:1]
			})
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, svc)
			Eventually(func(g Gomega) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())
				g.Expect(current.Status.GetCondition(v1alpha2.HTTPRoutesReady).IsTrue()).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())
		})
	}

	It("should let a service annotation pin its strategy under a different cluster-wide one", func(ctx SpecContext) {
		patchIngressLoraRoutingStrategy(ctx, "regex")
		DeferCleanup(restoreIngressLoraRoutingStrategy)
		ns := NewTestNamespace(ctx, envTest)
		pinned := create(ctx, ns, "pinned-exact",
			WithSpecAnnotations(map[string]string{llmisvc.AnnotationLoRAModelRoutingStrategy: "exact"}))
		inherited := create(ctx, ns, "inherited-regex")
		// A typo never reaches the reconciler: the webhook rejects it at admission.
		typo := LLMInferenceService("typo",
			InNamespace[*v1alpha2.LLMInferenceService](ns.Name),
			WithModelURI("hf://facebook/opt-125m"), WithModelName(loraBaseModel), WithLoRAAdapters("adapter"),
			WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
			WithSpecAnnotations(map[string]string{llmisvc.AnnotationLoRAModelRoutingStrategy: "regexp"}))
		Expect(envTest.Create(ctx, typo)).To(MatchError(ContainSubstring(llmisvc.AnnotationLoRAModelRoutingStrategy)))
		eventuallyManagedRoute(ctx, pinned, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route).To(HaveHeaderMatch(headerName, publisherModel(ns.Name, "adapter")))
			for _, header := range modelRoutingHeaderMatches(route) {
				g.Expect(ptr.Deref(header.Type, gwapiv1.HeaderMatchExact)).To(Equal(gwapiv1.HeaderMatchExact))
			}
		})
		eventuallyManagedRoute(ctx, inherited, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route).To(HaveHeaderMatch(headerName, loraRegexPattern(ns.Name, "adapter")))
		})
	})

	It("should not rewrite the route when adapters are reordered", func(ctx SpecContext) {
		patchIngressLoraRoutingStrategy(ctx, "regex")
		DeferCleanup(restoreIngressLoraRoutingStrategy)
		ns := NewTestNamespace(ctx, envTest)
		svc := create(ctx, ns, "route-churn", WithLoRAAdapters("adapter-b", "adapter-a"))
		var stable *gwapiv1.HTTPRoute
		eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route).To(HaveHeaderMatch(headerName, loraRegexPattern(ns.Name, "adapter", "adapter-a", "adapter-b")))
			stable = route.DeepCopy()
		})
		routeUnchanged := func() {
			GinkgoHelper()
			Consistently(func(g Gomega) {
				route := &gwapiv1.HTTPRoute{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(stable), route)).To(Succeed())
				g.Expect(route.ResourceVersion).To(Equal(stable.ResourceVersion))
			}).WithContext(ctx).WithTimeout(2 * time.Second).Should(Succeed())
		}

		// Same adapters in a different order must not produce a write. The replica
		// bump proves the reconcile for this generation ran before the route is checked.
		update(ctx, svc, func(current *v1alpha2.LLMInferenceService) {
			slices.Reverse(current.Spec.Model.LoRA.Adapters)
			current.Spec.Replicas = ptr.To[int32](2)
		})
		Eventually(func(g Gomega) {
			deployment := &appsv1.Deployment{}
			g.Expect(envTest.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: svc.Name + "-kserve"}, deployment)).To(Succeed())
			g.Expect(ptr.Deref(deployment.Spec.Replicas, 0)).To(Equal(int32(2)))
		}).WithContext(ctx).Should(Succeed())
		routeUnchanged()

		// A new adapter rewrites the pattern once, then the route settles again.
		update(ctx, svc, func(current *v1alpha2.LLMInferenceService) {
			current.Spec.Model.LoRA.Adapters = append(current.Spec.Model.LoRA.Adapters,
				v1alpha2.LLMModelSpec{Name: ptr.To("adapter-c"), URI: current.Spec.Model.URI})
		})
		eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route).To(HaveHeaderMatch(headerName, loraRegexPattern(ns.Name, "adapter", "adapter-a", "adapter-b", "adapter-c")))
			stable = route.DeepCopy()
		})
		routeUnchanged()
	})

	It("should preserve weighted group routing and allow deletion after a peer's route becomes invalid", func(ctx SpecContext) {
		patchIngressLoraRoutingStrategy(ctx, "regex")
		DeferCleanup(restoreIngressLoraRoutingStrategy)
		ns := NewTestNamespace(ctx, envTest)
		a := create(ctx, ns, "group-a", WithGroup("regex-group"), WithWeight(80))
		b := create(ctx, ns, "group-b", WithGroup("regex-group"), WithWeight(20))
		// Delete A first during cleanup if an assertion fails while A is invalid.
		DeferCleanup(func(ctx SpecContext) { Expect(client.IgnoreNotFound(envTest.Delete(ctx, a))).To(Succeed()) })
		ensureRouterManagedResourcesAreReady(ctx, envTest.Client, a)
		ensureRouterManagedResourcesAreReady(ctx, envTest.Client, b)
		var stableRV string
		eventuallyManagedRoute(ctx, a, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route).To(HaveHeaderMatch(headerName, loraRegexPattern(ns.Name, "adapter")))
			refs := groupRoutingBackendRefs(route, a)
			g.Expect(refs).To(HaveLen(2))
			g.Expect([]int32{ptr.Deref(refs[0].Weight, 1), ptr.Deref(refs[1].Weight, 1)}).To(ConsistOf(int32(80), int32(20)))
			stableRV = route.ResourceVersion
		})
		Consistently(func(g Gomega) {
			route := &gwapiv1.HTTPRoute{}
			g.Expect(envTest.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: a.Name + "-kserve-route"}, route)).To(Succeed())
			g.Expect(route.ResourceVersion).To(Equal(stableRV))
		}).WithContext(ctx).WithTimeout(time.Second).Should(Succeed())

		update(ctx, a, func(current *v1alpha2.LLMInferenceService) {
			current.Spec.Router.Route.HTTP = unrecognizedModelMatchRoute(ns.Name)
		})
		Eventually(func(g Gomega) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(a), current)).To(Succeed())
			g.Expect(current.Status.GetCondition(v1alpha2.HTTPRoutesReady).IsFalse()).To(BeTrue())
		}).WithContext(ctx).Should(Succeed())
		Expect(envTest.Delete(ctx, b)).To(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(envTest.Get(ctx, client.ObjectKeyFromObject(b), &v1alpha2.LLMInferenceService{}))
		}).WithContext(ctx).WithTimeout(20 * time.Second).Should(BeTrue())
	})
})
