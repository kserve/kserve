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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/constants"

	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

var _ = Describe("LLMInferenceService LocalModel Propagation", func() {
	Context("Single-Node Workloads", func() {
		It("should propagate local model labels and annotations to the deployment", func(ctx SpecContext) {
			svcName := "test-llm-localmodel"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithLabels(map[string]string{
					constants.LocalModelLabel:          "my-cache",
					constants.LocalModelNamespaceLabel: "default",
				}),
				WithAnnotations(map[string]string{
					constants.LocalModelSourceUriAnnotationKey: "hf://meta-llama/Llama-3-8b",
					constants.LocalModelPVCNameAnnotationKey:   "my-cache-gpu",
				}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, expectedDeployment)
			}).WithContext(ctx).Should(Succeed())

			By("checking the Deployment's top-level metadata")
			Expect(expectedDeployment.Labels).To(HaveKeyWithValue(constants.LocalModelLabel, "my-cache"))
			Expect(expectedDeployment.Labels).To(HaveKeyWithValue(constants.LocalModelNamespaceLabel, "default"))
			Expect(expectedDeployment.Annotations).To(HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "hf://meta-llama/Llama-3-8b"))
			Expect(expectedDeployment.Annotations).To(HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, "my-cache-gpu"))

			By("checking the Deployment's pod template metadata")
			Expect(expectedDeployment.Spec.Template.Labels).To(HaveKeyWithValue(constants.LocalModelLabel, "my-cache"))
			Expect(expectedDeployment.Spec.Template.Labels).To(HaveKeyWithValue(constants.LocalModelNamespaceLabel, "default"))
			Expect(expectedDeployment.Spec.Template.Annotations).To(HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "hf://meta-llama/Llama-3-8b"))
			Expect(expectedDeployment.Spec.Template.Annotations).To(HaveKeyWithValue(constants.LocalModelPVCNameAnnotationKey, "my-cache-gpu"))
		})
	})

	Context("Multi-Node Workloads", func() {
		It("should propagate local model labels and annotations to the LeaderWorkerSet", func(ctx SpecContext) {
			svcName := "test-llm-localmodel-mn"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(1),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{}),
				WithManagedRoute(),
				WithManagedGateway(),
				WithLabels(map[string]string{
					constants.LocalModelLabel: "my-cache",
				}),
				WithAnnotations(map[string]string{
					constants.LocalModelSourceUriAnnotationKey: "hf://meta-llama/Llama-3-8b",
					constants.LocalModelPVCNameAnnotationKey:   "my-cache-gpu",
				}),
			)

			Expect(llmSvc.Spec.Parallelism.IsDataParallel()).To(BeTrue())
			Expect(llmSvc.Spec.Worker).To(Not(BeNil()))

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			expectedLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: testNs.Name,
				}, expectedLWS)
			}).WithContext(ctx).Should(Succeed())

			By("checking the LeaderWorkerSet's top-level metadata")
			Expect(expectedLWS.Labels).To(HaveKeyWithValue(constants.LocalModelLabel, "my-cache"))
			Expect(expectedLWS.Annotations).To(HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "hf://meta-llama/Llama-3-8b"))

			By("checking the leader pod template metadata")
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Labels).To(HaveKeyWithValue(constants.LocalModelLabel, "my-cache"))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Annotations).To(HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "hf://meta-llama/Llama-3-8b"))

			By("checking the worker pod template metadata")
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Labels).To(HaveKeyWithValue(constants.LocalModelLabel, "my-cache"))
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations).To(HaveKeyWithValue(constants.LocalModelSourceUriAnnotationKey, "hf://meta-llama/Llama-3-8b"))
		})
	})
})
