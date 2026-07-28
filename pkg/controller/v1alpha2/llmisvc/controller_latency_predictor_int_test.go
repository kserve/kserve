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
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Controller — Latency Predictor", func() {
	Context("When predicted-latency-producer plugin is in the scheduler config", func() {
		It("should inject training and prediction sidecar containers into the scheduler deployment", func(ctx SpecContext) {
			svcName := "test-llm-lp-sidecars"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(`plugins:
- type: predicted-latency-producer
- type: latency-scorer
- type: weighted-random-picker
`),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				deployments := &appsv1.DeploymentList{}
				g.Expect(envTest.Client.List(ctx, deployments, &client.ListOptions{
					Namespace:     testNs.Name,
					LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
				})).To(Succeed())
				g.Expect(deployments.Items).To(HaveLen(1))

				dep := deployments.Items[0]
				containerNames := make([]string, len(dep.Spec.Template.Spec.Containers))
				for i, c := range dep.Spec.Template.Spec.Containers {
					containerNames[i] = c.Name
				}
				g.Expect(containerNames).To(ContainElement("training-server"))
				g.Expect(containerNames).To(ContainElement("prediction-server"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("When predicted-latency-producer plugin is NOT in the scheduler config", func() {
		It("should not inject sidecar containers", func(ctx SpecContext) {
			svcName := "test-llm-no-lp"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(`plugins:
- type: queue-scorer
- type: max-score-picker
`),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				deployments := &appsv1.DeploymentList{}
				g.Expect(envTest.Client.List(ctx, deployments, &client.ListOptions{
					Namespace:     testNs.Name,
					LabelSelector: labels.SelectorFromSet(llmisvc.SchedulerLabels(llmSvc)),
				})).To(Succeed())
				g.Expect(deployments.Items).To(HaveLen(1))

				dep := deployments.Items[0]
				containerNames := make([]string, len(dep.Spec.Template.Spec.Containers))
				for i, c := range dep.Spec.Template.Spec.Containers {
					containerNames[i] = c.Name
				}
				g.Expect(containerNames).NotTo(ContainElement("training-server"))
				g.Expect(containerNames).NotTo(ContainElement("prediction-server"))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
