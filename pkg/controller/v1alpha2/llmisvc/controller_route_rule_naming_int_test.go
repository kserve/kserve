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
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

const routeRuleNamingKey = "routeRuleNaming"

func ruleNames(route *gwapiv1.HTTPRoute) []string {
	names := make([]string, 0, len(route.Spec.Rules))
	for _, rule := range route.Spec.Rules {
		names = append(names, string(ptr.Deref(rule.Name, "")))
	}
	return names
}

// ruleSuffix is derived independently of the production helper.
func ruleSuffix(svc *v1alpha2.LLMInferenceService) string {
	sum := sha256.Sum256([]byte(svc.Namespace + "/" + svc.Name))
	return "-" + hex.EncodeToString(sum[:])[:8]
}

func uniqueRuleNames(presetNames []string, svc *v1alpha2.LLMInferenceService) []string {
	names := make([]string, 0, len(presetNames))
	for _, name := range presetNames {
		if name != "" {
			name += ruleSuffix(svc)
		}
		names = append(names, name)
	}
	return names
}

// waitForReconcilesToSettle waits until no controller has finished a reconcile
// for one polling interval. A ConfigMap change made afterwards reaches a service
// only through the ConfigMap watch, not through the tail of an earlier burst.
func waitForReconcilesToSettle(ctx context.Context) {
	GinkgoHelper()
	previous := -1.0
	Eventually(func(g Gomega) {
		families, err := ctrlmetrics.Registry.Gather()
		g.Expect(err).NotTo(HaveOccurred())
		var total float64
		for _, family := range families {
			if family.GetName() == "controller_runtime_reconcile_total" {
				for _, metric := range family.GetMetric() {
					total += metric.GetCounter().GetValue()
				}
			}
		}
		g.Expect(total).To(BeNumerically(">", 0), "no reconcile counted, so nothing can be judged settled")
		settled := total == previous
		previous = total
		g.Expect(settled).To(BeTrue(), "reconciles still running")
	}).WithContext(ctx).WithPolling(2 * time.Second).Within(time.Minute).Should(Succeed())
}

var _ = Describe("Route rule naming", func() {
	create := func(ctx SpecContext, name string, opts ...LLMInferenceServiceOption) *v1alpha2.LLMInferenceService {
		GinkgoHelper()
		svc := LLMInferenceService(name,
			InNamespace[*v1alpha2.LLMInferenceService](NewTestNamespace(ctx, envTest).Name),
			WithModelURI("hf://facebook/opt-125m"), WithManagedRoute(), WithManagedGateway(), WithManagedScheduler())
		for _, opt := range opts {
			opt(svc)
		}
		Expect(envTest.Create(ctx, svc)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(client.IgnoreNotFound(envTest.Delete(ctx, svc))).To(Succeed())
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(apierrors.IsNotFound(envTest.Get(ctx, client.ObjectKeyFromObject(svc), &v1alpha2.LLMInferenceService{}))).To(BeTrue())
			}).WithContext(ctx).Should(Succeed())
		})
		return svc
	}

	setAnnotation := func(ctx SpecContext, svc *v1alpha2.LLMInferenceService, value *string) {
		GinkgoHelper()
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(svc), current); err != nil {
				return err
			}
			if value == nil {
				delete(current.Spec.Annotations, llmisvc.AnnotationRouteRuleNaming)
			} else {
				current.Spec.Annotations = map[string]string{llmisvc.AnnotationRouteRuleNaming: *value}
			}
			return envTest.Update(ctx, current)
		})).To(Succeed())
	}

	// currentRoute waits for the route to carry a rule named want, then returns
	// its rule names and UID.
	currentRoute := func(ctx context.Context, svc *v1alpha2.LLMInferenceService, want string) ([]string, types.UID) {
		GinkgoHelper()
		var names []string
		var uid types.UID
		eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
			names, uid = ruleNames(route), route.UID
			g.Expect(names).To(ContainElement(want))
		})
		return names, uid
	}

	// expectRuleNames also checks the UID: a rename has to update the route in
	// place, not replace it.
	expectRuleNames := func(ctx context.Context, svc *v1alpha2.LLMInferenceService, uid types.UID, want []string) {
		GinkgoHelper()
		eventuallyManagedRoute(ctx, svc, func(g Gomega, route *gwapiv1.HTTPRoute) {
			g.Expect(route.UID).To(Equal(uid))
			g.Expect(ruleNames(route)).To(Equal(want))
		})
	}

	mainDeploymentGeneration := func(ctx context.Context, svc *v1alpha2.LLMInferenceService) int64 {
		GinkgoHelper()
		deployment := &appsv1.Deployment{}
		Eventually(func(g Gomega, ctx context.Context) {
			g.Expect(envTest.Get(ctx, types.NamespacedName{Name: svc.Name + "-kserve", Namespace: svc.Namespace}, deployment)).To(Succeed())
		}).WithContext(ctx).Should(Succeed())
		return deployment.Generation
	}

	expectNoRollout := func(ctx context.Context, svc *v1alpha2.LLMInferenceService, generation int64) {
		GinkgoHelper()
		waitForReconcilesToSettle(ctx)
		Expect(mainDeploymentGeneration(ctx, svc)).To(Equal(generation), "the rule naming must not change the pod template")
	}

	Context("when the cluster-wide value changes", func() {
		var (
			svc         *v1alpha2.LLMInferenceService
			presetNames []string
			routeUID    types.UID
			generation  int64
		)

		BeforeEach(func(ctx SpecContext) {
			svc = create(ctx, "svc")
			presetNames, routeUID = currentRoute(ctx, svc, "v1-completions-path")
			waitForReconcilesToSettle(ctx)
			generation = mainDeploymentGeneration(ctx, svc)
			// Registered after create, so the ConfigMap is restored before the service is deleted.
			DeferCleanup(func(ctx SpecContext) { PatchIngressConfigKey(ctx, envTest.Client, routeRuleNamingKey, nil) })
		})

		It("should rename the existing route's rules when set to unique", func(ctx SpecContext) {
			PatchIngressConfigKey(ctx, envTest.Client, routeRuleNamingKey, "unique")

			expectRuleNames(ctx, svc, routeUID, uniqueRuleNames(presetNames, svc))
			expectNoRollout(ctx, svc, generation)
		})

		It("should restore the preset names when the key is removed", func(ctx SpecContext) {
			PatchIngressConfigKey(ctx, envTest.Client, routeRuleNamingKey, "unique")
			expectRuleNames(ctx, svc, routeUID, uniqueRuleNames(presetNames, svc))

			PatchIngressConfigKey(ctx, envTest.Client, routeRuleNamingKey, nil)

			expectRuleNames(ctx, svc, routeUID, presetNames)
			expectNoRollout(ctx, svc, generation)
		})

		It("should mark HTTPRoutes not ready and keep the route on an unsupported value", func(ctx SpecContext) {
			PatchIngressConfigKey(ctx, envTest.Client, routeRuleNamingKey, "unique")
			uniqueNames := uniqueRuleNames(presetNames, svc)
			expectRuleNames(ctx, svc, routeUID, uniqueNames)

			PatchIngressConfigKey(ctx, envTest.Client, routeRuleNamingKey, "suffixed")

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())
				condition := current.Status.GetCondition(v1alpha2.HTTPRoutesReady)
				g.Expect(condition).NotTo(BeNil())
				g.Expect(condition.IsFalse()).To(BeTrue())
				g.Expect(condition.Reason).To(Equal("RoutingPreconditionNotMet"))
				g.Expect(condition.Message).To(ContainSubstring(
					`unsupported routeRuleNaming "suffixed" from the ` + constants.InferenceServiceConfigMapName + " ConfigMap"))
			}).WithContext(ctx).Should(Succeed())
			waitForReconcilesToSettle(ctx)
			expectRuleNames(ctx, svc, routeUID, uniqueNames)
		})
	})

	Context("when the annotation changes on a live service", func() {
		It("should rename the rules when the annotation is added", func(ctx SpecContext) {
			svc := create(ctx, "svc")
			presetNames, routeUID := currentRoute(ctx, svc, "v1-completions-path")
			waitForReconcilesToSettle(ctx)
			generation := mainDeploymentGeneration(ctx, svc)

			setAnnotation(ctx, svc, ptr.To("unique"))

			expectRuleNames(ctx, svc, routeUID, uniqueRuleNames(presetNames, svc))
			expectNoRollout(ctx, svc, generation)
		})

		It("should restore the preset names when the annotation is removed", func(ctx SpecContext) {
			svc := create(ctx, "svc", WithSpecAnnotations(map[string]string{llmisvc.AnnotationRouteRuleNaming: "unique"}))
			uniqueNames, routeUID := currentRoute(ctx, svc, "v1-completions-path"+ruleSuffix(svc))
			waitForReconcilesToSettle(ctx)
			generation := mainDeploymentGeneration(ctx, svc)

			setAnnotation(ctx, svc, nil)

			presetNames := make([]string, 0, len(uniqueNames))
			for _, name := range uniqueNames {
				presetNames = append(presetNames, strings.TrimSuffix(name, ruleSuffix(svc)))
			}
			expectRuleNames(ctx, svc, routeUID, presetNames)
			expectNoRollout(ctx, svc, generation)
		})
	})

	It("should reject an unsupported annotation value at admission", func(ctx SpecContext) {
		svc := LLMInferenceService("unsupported",
			InNamespace[*v1alpha2.LLMInferenceService](NewTestNamespace(ctx, envTest).Name),
			WithModelURI("hf://facebook/opt-125m"), WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
			WithSpecAnnotations(map[string]string{llmisvc.AnnotationRouteRuleNaming: "suffixed"}))

		Expect(envTest.Create(ctx, svc)).To(MatchError(ContainSubstring(llmisvc.AnnotationRouteRuleNaming)))
	})
})
