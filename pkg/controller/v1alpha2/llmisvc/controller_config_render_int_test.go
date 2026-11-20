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
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("Recursive config render gate", func() {
	// workloadGenerations maps each Deployment and LeaderWorkerSet in the namespace
	// to its generation, which moves only when the controller changes its spec.
	workloadGenerations := func(g Gomega, ctx context.Context, namespace string) map[string]int64 {
		generations := map[string]int64{}

		deployments := &appsv1.DeploymentList{}
		g.Expect(envTest.List(ctx, deployments, client.InNamespace(namespace))).To(Succeed())
		for _, deployment := range deployments.Items {
			generations["Deployment/"+deployment.Name] = deployment.Generation
		}

		groups := &lwsapi.LeaderWorkerSetList{}
		g.Expect(envTest.List(ctx, groups, client.InNamespace(namespace))).To(Succeed())
		for _, group := range groups.Items {
			generations["LeaderWorkerSet/"+group.Name] = group.Generation
		}

		return generations
	}

	expectPresetsCombined := func(g Gomega, ctx context.Context, svc *v1alpha2.LLMInferenceService) {
		current := &v1alpha2.LLMInferenceService{}
		g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(svc), current)).To(Succeed())
		g.Expect(current.Status).To(HaveCondition(string(v1alpha2.PresetsCombined), "True"))
	}

	DescribeTable("should leave shipped-preset workloads unchanged when the gate is turned on",
		func(ctx SpecContext, opts ...LLMInferenceServiceOption) {
			testNs := NewTestNamespace(ctx, envTest)
			svc := LLMInferenceService("render-gate",
				append([]LLMInferenceServiceOption{
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("hf://facebook/opt-125m"),
					WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
				}, opts...)...)
			Expect(envTest.Create(ctx, svc)).To(Succeed())
			defer testNs.DeleteAndWait(ctx, svc)
			// Registered after create, so the gate is reset however the spec ends.
			DeferCleanup(func(ctx context.Context) { PatchLLMISVCFeatureGates(ctx, envTest.Client, nil) })

			Eventually(func(g Gomega, ctx context.Context) {
				expectPresetsCombined(g, ctx, svc)
				g.Expect(workloadGenerations(g, ctx, testNs.Name)).NotTo(BeEmpty())
			}).WithContext(ctx).Should(Succeed())
			waitForReconcilesToSettle(ctx)
			before := workloadGenerations(Default, ctx, testNs.Name)
			reconciles := reconcileTotal(Default)

			PatchLLMISVCFeatureGates(ctx, envTest.Client, map[string]bool{"recursiveConfigRender": true})

			// The ConfigMap watch requeues every service, so a rising count shows the
			// change reached the controller before the workloads are compared.
			Eventually(func(g Gomega) {
				g.Expect(reconcileTotal(g)).To(BeNumerically(">", reconciles))
			}).WithContext(ctx).Should(Succeed())
			waitForReconcilesToSettle(ctx)

			Expect(workloadGenerations(Default, ctx, testNs.Name)).To(Equal(before),
				"the recursive renderer must render the shipped presets as the JSON renderer does")
			Eventually(func(g Gomega, ctx context.Context) {
				expectPresetsCombined(g, ctx, svc)
			}).WithContext(ctx).Should(Succeed())
		},
		Entry("single node"),
		Entry("prefill and decode", WithPrefill(SimpleWorkerPodSpec())),
		Entry("multi-node data parallel",
			WithWorker(SimpleWorkerPodSpec()),
			WithParallelism(ParallelismSpec(WithDataParallelism(2), WithDataLocalParallelism(1))),
		),
	)
})
