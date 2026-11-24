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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

// generationHold is how long a spec checks that the EPP Service does not move.
const generationHold = 3 * time.Second

var _ = Describe("LLMInferenceService Scheduler Generations", func() {
	It("runs a new scheduler generation next to the serving one, moves the EPP Service once it is available, and rolls back without starting a pod", func(ctx SpecContext) {
		// given
		svcName := "test-llm-sched-generations"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := LLMInferenceService(svcName,
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedRoute(),
			WithManagedGateway(),
			WithManagedScheduler(),
			WithSchedulerGenerations(v1alpha2.SchedulerGenerationsSpec{}),
		)

		// when
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		// then: the first generation serves at once, since nothing else could
		first := waitForSchedulerGenerations(ctx, llmSvc, 1)[0]
		Expect(first.Name).To(Equal(kmeta.ChildName(svcName, "-kserve-router-scheduler-"+first.Labels[llmisvc.SchedulerGenerationLabelKey])))
		Expect(first.Spec.Selector.MatchLabels).To(HaveKey(llmisvc.SchedulerGenerationLabelKey))
		firstGen := first.Labels[llmisvc.SchedulerGenerationLabelKey]
		expectEPPServiceSelects(ctx, llmSvc, firstGen)
		Expect(envTest.Get(ctx, types.NamespacedName{Namespace: testNs.Name, Name: kmeta.ChildName(svcName, "-kserve-router-scheduler")}, &appsv1.Deployment{})).
			To(Satisfy(func(err error) bool { return client.IgnoreNotFound(err) == nil && err != nil }), "no scheduler Deployment besides the generation")
		markSchedulerGenerationAvailable(ctx, &first)

		// when: the scheduler changes
		updateLLMISvc(ctx, llmSvc, func(curr *v1alpha2.LLMInferenceService) {
			curr.Spec.Router.Scheduler.Annotations = map[string]string{"test.kserve.io/release": "2"}
		})

		// then: a second generation runs next to the first, which keeps serving until it is available
		generations := waitForSchedulerGenerations(ctx, llmSvc, 2)
		second := otherGeneration(generations, firstGen)
		secondGen := second.Labels[llmisvc.SchedulerGenerationLabelKey]
		Expect(second.Spec.Template.Annotations).To(HaveKeyWithValue("test.kserve.io/release", "2"))
		Consistently(func(g Gomega, ctx context.Context) {
			g.Expect(eppServiceSelector(ctx, llmSvc)).To(HaveKeyWithValue(llmisvc.SchedulerGenerationLabelKey, firstGen))
		}).WithContext(ctx).WithTimeout(generationHold).Should(Succeed())

		// when: the second generation is available
		markSchedulerGenerationAvailable(ctx, &second)

		// then: the Service moves to it and the first keeps running
		expectEPPServiceSelects(ctx, llmSvc, secondGen)
		Expect(waitForSchedulerGenerations(ctx, llmSvc, 2)).To(HaveLen(2))
		Eventually(func(g Gomega, ctx context.Context) {
			curr := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), curr)).To(Succeed())
			g.Expect(curr.Status.Workloads).NotTo(BeNil())
			g.Expect(curr.Status.Workloads.Scheduler).NotTo(BeNil())
			g.Expect(curr.Status.Workloads.Scheduler.Name).To(Equal(second.Name))
			g.Expect(curr.Status.Workloads.SchedulerGenerations).To(ConsistOf(
				And(HaveField("Name", secondGen), HaveField("Serving", true), HaveField("Current", true)),
				And(HaveField("Name", firstGen), HaveField("Serving", false), HaveField("Current", false)),
			))
		}).WithContext(ctx).Should(Succeed())

		// when: rolling back by naming the first generation
		updateLLMISvc(ctx, llmSvc, func(curr *v1alpha2.LLMInferenceService) {
			curr.Spec.Router.Scheduler.RolloutStrategy.Generations = &v1alpha2.SchedulerGenerationsSpec{Serving: firstGen}
		})

		// then: the Service is back on it, and no generation was started or deleted for it
		expectEPPServiceSelects(ctx, llmSvc, firstGen)
		Consistently(func(g Gomega, ctx context.Context) {
			g.Expect(listSchedulerGenerations(ctx, llmSvc)).To(HaveLen(2))
		}).WithContext(ctx).WithTimeout(generationHold).Should(Succeed())

		// when: the rollback is dropped and the spec reverted to the first release
		updateLLMISvc(ctx, llmSvc, func(curr *v1alpha2.LLMInferenceService) {
			curr.Spec.Router.Scheduler.Annotations = nil
			curr.Spec.Router.Scheduler.RolloutStrategy.Generations = nil
		})

		// then: the reverted spec is the first generation again: it keeps serving, and
		// the second stays as the one to roll back to
		Consistently(func(g Gomega, ctx context.Context) {
			g.Expect(eppServiceSelector(ctx, llmSvc)).To(HaveKeyWithValue(llmisvc.SchedulerGenerationLabelKey, firstGen))
			g.Expect(listSchedulerGenerations(ctx, llmSvc)).To(HaveLen(2))
		}).WithContext(ctx).WithTimeout(generationHold).Should(Succeed())

		// when: leaving generations for Recreate
		updateLLMISvc(ctx, llmSvc, func(curr *v1alpha2.LLMInferenceService) {
			curr.Spec.Router.Scheduler.RolloutStrategy = nil
		})

		// then: the scheduler Deployment comes back, the Service selects it, and the
		// generations go only once it is available
		legacyKey := types.NamespacedName{Namespace: testNs.Name, Name: kmeta.ChildName(svcName, "-kserve-router-scheduler")}
		legacy := &appsv1.Deployment{}
		Eventually(func(g Gomega, ctx context.Context) {
			g.Expect(envTest.Get(ctx, legacyKey, legacy)).To(Succeed())
			g.Expect(eppServiceSelector(ctx, llmSvc)).NotTo(HaveKey(llmisvc.SchedulerGenerationLabelKey))
		}).WithContext(ctx).Should(Succeed())
		Consistently(func(g Gomega, ctx context.Context) {
			g.Expect(listSchedulerGenerations(ctx, llmSvc)).To(HaveLen(2))
		}).WithContext(ctx).WithTimeout(generationHold).Should(Succeed())

		markSchedulerGenerationAvailable(ctx, legacy)
		Eventually(func(g Gomega, ctx context.Context) {
			g.Expect(listSchedulerGenerations(ctx, llmSvc)).To(BeEmpty())
		}).WithContext(ctx).Should(Succeed())
	})

	It("keeps the EPP Service on the serving generation under Manual promotion until generations.serving names the new one", func(ctx SpecContext) {
		// given
		svcName := "test-llm-sched-gen-manual"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := LLMInferenceService(svcName,
			InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
			WithModelURI("hf://facebook/opt-125m"),
			WithManagedRoute(),
			WithManagedGateway(),
			WithManagedScheduler(),
			WithSchedulerReplicas(2),
			WithSchedulerGenerations(v1alpha2.SchedulerGenerationsSpec{Promotion: v1alpha2.SchedulerGenerationPromotionManual}),
		)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() {
			testNs.DeleteAndWait(ctx, llmSvc)
		}()

		first := waitForSchedulerGenerations(ctx, llmSvc, 1)[0]
		firstGen := first.Labels[llmisvc.SchedulerGenerationLabelKey]
		expectEPPServiceSelects(ctx, llmSvc, firstGen)
		Expect(mainContainerArgs(&first)).To(ContainElements("--ha-enable-leader-election", "--ha-lease-name="+kmeta.ChildName(svcName, "-epp-"+firstGen)+".llm-d.ai"),
			"leader-elected generations each elect their own leader")
		markSchedulerGenerationAvailable(ctx, &first)

		// when: the scheduler changes and its new generation is available
		updateLLMISvc(ctx, llmSvc, func(curr *v1alpha2.LLMInferenceService) {
			curr.Spec.Router.Scheduler.Annotations = map[string]string{"test.kserve.io/release": "2"}
		})
		second := otherGeneration(waitForSchedulerGenerations(ctx, llmSvc, 2), firstGen)
		secondGen := second.Labels[llmisvc.SchedulerGenerationLabelKey]
		markSchedulerGenerationAvailable(ctx, &second)

		// then: the Service stays on the first
		Consistently(func(g Gomega, ctx context.Context) {
			g.Expect(eppServiceSelector(ctx, llmSvc)).To(HaveKeyWithValue(llmisvc.SchedulerGenerationLabelKey, firstGen))
		}).WithContext(ctx).WithTimeout(generationHold).Should(Succeed())

		// when: promoted
		updateLLMISvc(ctx, llmSvc, func(curr *v1alpha2.LLMInferenceService) {
			curr.Spec.Router.Scheduler.RolloutStrategy.Generations.Serving = secondGen
		})

		// then
		expectEPPServiceSelects(ctx, llmSvc, secondGen)
	})
})

func listSchedulerGenerations(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) ([]appsv1.Deployment, error) {
	hasGeneration, err := labels.NewRequirement(llmisvc.SchedulerGenerationLabelKey, selection.Exists, nil)
	if err != nil {
		return nil, err
	}
	list := &appsv1.DeploymentList{}
	err = envTest.List(ctx, list, client.InNamespace(llmSvc.Namespace),
		client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)).Add(*hasGeneration)})
	var running []appsv1.Deployment
	for _, d := range list.Items {
		if d.DeletionTimestamp.IsZero() {
			running = append(running, d)
		}
	}
	return running, err
}

func waitForSchedulerGenerations(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, n int) []appsv1.Deployment {
	var generations []appsv1.Deployment
	Eventually(func(g Gomega, ctx context.Context) {
		var err error
		generations, err = listSchedulerGenerations(ctx, llmSvc)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(generations).To(HaveLen(n))
	}).WithContext(ctx).Should(Succeed())
	return generations
}

func otherGeneration(generations []appsv1.Deployment, generation string) appsv1.Deployment {
	for _, d := range generations {
		if d.Labels[llmisvc.SchedulerGenerationLabelKey] != generation {
			return d
		}
	}
	Fail("no generation besides " + generation)
	return appsv1.Deployment{}
}

func eppServiceSelector(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (map[string]string, error) {
	svc := &corev1.Service{}
	err := envTest.Get(ctx, types.NamespacedName{Namespace: llmSvc.Namespace, Name: kmeta.ChildName(llmSvc.Name, "-epp-service")}, svc)
	return svc.Spec.Selector, err
}

func expectEPPServiceSelects(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, generation string) {
	GinkgoHelper()
	Eventually(func(g Gomega, ctx context.Context) {
		g.Expect(eppServiceSelector(ctx, llmSvc)).To(And(
			HaveKeyWithValue(llmisvc.SchedulerGenerationLabelKey, generation),
			HaveKeyWithValue("app.kubernetes.io/name", llmSvc.Name),
		))
	}).WithContext(ctx).Should(Succeed())
}

// markSchedulerGenerationAvailable stands in for the Deployment controller that envtest
// does not run.
func markSchedulerGenerationAvailable(ctx context.Context, d *appsv1.Deployment) {
	GinkgoHelper()
	Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curr := &appsv1.Deployment{}
		if err := envTest.Get(ctx, client.ObjectKeyFromObject(d), curr); err != nil {
			return err
		}
		replicas := ptr.Deref(curr.Spec.Replicas, 1)
		curr.Status.ObservedGeneration = curr.Generation
		curr.Status.Replicas = replicas
		curr.Status.UpdatedReplicas = replicas
		curr.Status.ReadyReplicas = replicas
		curr.Status.AvailableReplicas = replicas
		curr.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}
		return envTest.Status().Update(ctx, curr)
	})).To(Succeed())
}

func updateLLMISvc(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, mutate func(*v1alpha2.LLMInferenceService)) {
	GinkgoHelper()
	Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		curr := &v1alpha2.LLMInferenceService{}
		if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), curr); err != nil {
			return err
		}
		mutate(curr)
		return envTest.Update(ctx, curr)
	})).To(Succeed())
}

func mainContainerArgs(d *appsv1.Deployment) []string {
	for _, c := range d.Spec.Template.Spec.Containers {
		if c.Name == "main" {
			return c.Args
		}
	}
	return nil
}
