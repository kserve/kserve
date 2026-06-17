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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Controller - Speculative Decoding", func() {
	Context("Single node with HF speculator model", func() {
		It("should create speculator-initializer and inject --speculative-config", func(ctx SpecContext) {
			svcName := "test-llm-spec-eagle3"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("hf://RedHatAI/Qwen3-32B-FP8-dynamic")
			Expect(err).ToNot(HaveOccurred())

			speculatorURL, err := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					Speculator: &v1alpha2.SpeculatorSpec{
						Model: &v1alpha2.LLMModelSpec{
							URI: *speculatorURL,
						},
						Config: map[string]string{
							"method":                 "eagle3",
							"num_speculative_tokens": "3",
						},
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())

			validateSpeculatorInitializerIsConfigured(deployment)
			validateSpeculativeDecodingArgsInjected(deployment, "eagle3")
		})
	})

	Context("Single node with ngram (no model)", func() {
		It("should inject --speculative-config without speculator-initializer", func(ctx SpecContext) {
			svcName := "test-llm-spec-ngram"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("hf://google/gemma-3-4b-it")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					Speculator: &v1alpha2.SpeculatorSpec{
						Config: map[string]string{
							"method":                 "ngram",
							"num_speculative_tokens": "4",
							"prompt_lookup_max":      "5",
						},
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())

			// Ngram doesn't need a speculator-initializer
			validateNoSpeculatorInitializer(deployment)
			validateSpeculativeDecodingArgsInjected(deployment, "ngram")
		})
	})

	Context("Single node without speculator", func() {
		It("should not inject speculator artifacts when speculator is nil", func(ctx SpecContext) {
			svcName := "test-llm-no-spec"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("hf://meta-llama/Llama-3.1-8B")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())

			validateNoSpeculatorInitializer(deployment)
			validateNoSpeculativeDecodingArgs(deployment)
		})
	})

	Context("Disaggregated with speculator on decode only", func() {
		It("should attach speculator to decode deployment but not prefill", func(ctx SpecContext) {
			svcName := "test-llm-spec-disagg"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("hf://RedHatAI/Qwen3-32B-FP8-dynamic")
			Expect(err).ToNot(HaveOccurred())

			speculatorURL, err := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					Speculator: &v1alpha2.SpeculatorSpec{
						Model: &v1alpha2.LLMModelSpec{
							URI: *speculatorURL,
						},
						Config: map[string]string{
							"method":                 "eagle3",
							"num_speculative_tokens": "6",
						},
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Decode deployment should have speculator
			decodeDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, decodeDeployment)
			}).WithContext(ctx).Should(Succeed())
			validateSpeculatorInitializerIsConfigured(decodeDeployment)
			validateSpeculativeDecodingArgsInjected(decodeDeployment, "eagle3")

			// Prefill deployment should NOT have speculator
			prefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: testNs.Name,
				}, prefillDeployment)
			}).WithContext(ctx).Should(Succeed())
			validateNoSpeculatorInitializer(prefillDeployment)
			validateNoSpeculativeDecodingArgs(prefillDeployment)
		})
	})

	Context("Multi-node with speculator on leader and workers", func() {
		It("should inject speculator on both leader and worker templates", func(ctx SpecContext) {
			svcName := "test-llm-spec-mn"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			speculatorURL, err := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://RedHatAI/Qwen3-32B-FP8-dynamic"),
				WithReplicas(1),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
					WithTensorParallelism(2),
				)),
				WithTemplate(SimpleWorkerPodSpec()),
				WithWorker(SimpleWorkerPodSpec()),
				WithSpeculator(&v1alpha2.SpeculatorSpec{
					Model: &v1alpha2.LLMModelSpec{
						URI: *speculatorURL,
					},
					Config: map[string]string{
						"method":                 "eagle3",
						"num_speculative_tokens": "3",
					},
				}),
				WithManagedRoute(),
				WithManagedGateway(),
			)

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

			// Leader template should have speculator
			Expect(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate).ToNot(BeNil())
			validateSpeculatorOnPodSpec(expectedLWS.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec, "eagle3")

			// Worker template should also have speculator
			validateSpeculatorOnPodSpec(expectedLWS.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec, "eagle3")
		})
	})

	Context("Speculator removal", func() {
		It("should remove speculator init container, volume, and args when speculator is set to nil", func(ctx SpecContext) {
			svcName := "test-llm-spec-removal"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("hf://meta-llama/Llama-3.1-8B")
			Expect(err).ToNot(HaveOccurred())

			speculatorURL, err := apis.ParseURL("hf://RedHatAI/Llama-3.1-8B-speculator.eagle3")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					Speculator: &v1alpha2.SpeculatorSpec{
						Model: &v1alpha2.LLMModelSpec{
							URI: *speculatorURL,
						},
						Config: map[string]string{
							"method":                 "eagle3",
							"num_speculative_tokens": "3",
						},
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for deployment with speculator init container and args
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())
			validateSpeculatorInitializerIsConfigured(deployment)
			validateSpeculativeDecodingArgsInjected(deployment, "eagle3")

			// Remove speculator
			current := &v1alpha2.LLMInferenceService{}
			Expect(envTest.Get(ctx, types.NamespacedName{Name: svcName, Namespace: testNs.Name}, current)).To(Succeed())
			current.Spec.Speculator = nil
			Expect(envTest.Update(ctx, current)).To(Succeed())

			// Verify all speculator artifacts are removed
			Eventually(func(g Gomega, ctx context.Context) {
				updated := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, updated)).To(Succeed())

				podSpec := updated.Spec.Template.Spec

				// Init container must be gone
				initContainer := findInitContainerByName(podSpec.InitContainers, constants.SpeculatorInitializerContainerName)
				g.Expect(initContainer).To(BeNil(), "speculator-initializer should be removed")

				// Volume must be gone
				for _, v := range podSpec.Volumes {
					g.Expect(v.Name).ToNot(Equal(constants.SpeculatorVolumeName),
						"speculator volume should be removed")
				}

				// --speculative-config must be gone
				mainContainer := findContainerByName(podSpec.Containers, "main")
				g.Expect(mainContainer).ToNot(BeNil())
				for _, env := range mainContainer.Env {
					if env.Name == "VLLM_ADDITIONAL_ARGS" {
						g.Expect(env.Value).ToNot(ContainSubstring("--speculative-config"),
							"--speculative-config should be removed after speculator is nil")
					}
				}
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

func validateSpeculatorInitializerIsConfigured(deployment *appsv1.Deployment) {
	GinkgoHelper()

	podSpec := deployment.Spec.Template.Spec

	initContainer := findInitContainerByName(podSpec.InitContainers, constants.SpeculatorInitializerContainerName)
	Expect(initContainer).ToNot(BeNil(), "expected speculator-initializer init container")

	Expect(initContainer.Args).To(ContainElement(constants.DefaultSpeculatorLocalMountPath),
		"speculator-initializer should download to %s", constants.DefaultSpeculatorLocalMountPath)

	hasSpeculatorVolume := false
	for _, v := range podSpec.Volumes {
		if v.Name == constants.SpeculatorVolumeName {
			hasSpeculatorVolume = true
		}
	}
	Expect(hasSpeculatorVolume).To(BeTrue(), "expected speculator volume %s", constants.SpeculatorVolumeName)
}

func validateNoSpeculatorInitializer(deployment *appsv1.Deployment) {
	GinkgoHelper()

	initContainer := findInitContainerByName(deployment.Spec.Template.Spec.InitContainers, constants.SpeculatorInitializerContainerName)
	Expect(initContainer).To(BeNil(), "speculator-initializer should not be present")
}

func validateSpeculativeDecodingArgsInjected(deployment *appsv1.Deployment, expectedMethod string) {
	GinkgoHelper()

	mainContainer := findContainerByName(deployment.Spec.Template.Spec.Containers, "main")
	Expect(mainContainer).ToNot(BeNil(), "expected main container")

	var vllmArgs string
	for _, env := range mainContainer.Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}

	Expect(vllmArgs).To(ContainSubstring("--speculative-config"), "expected --speculative-config in VLLM_ADDITIONAL_ARGS")

	// Parse the JSON config and verify the method
	jsonStr := extractJSONFromSpeculativeConfig(vllmArgs)
	var config map[string]interface{}
	Expect(json.Unmarshal([]byte(jsonStr), &config)).To(Succeed())
	Expect(config["method"]).To(Equal(expectedMethod))
}

func validateNoSpeculativeDecodingArgs(deployment *appsv1.Deployment) {
	GinkgoHelper()

	mainContainer := findContainerByName(deployment.Spec.Template.Spec.Containers, "main")
	if mainContainer == nil {
		return
	}

	for _, env := range mainContainer.Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			Expect(env.Value).ToNot(ContainSubstring("--speculative-config"),
				"expected no --speculative-config in VLLM_ADDITIONAL_ARGS")
		}
	}
}

func findInitContainerByName(initContainers []corev1.Container, name string) *corev1.Container { //nolint:unparam
	for i := range initContainers {
		if initContainers[i].Name == name {
			return &initContainers[i]
		}
	}
	return nil
}

func findContainerByName(containers []corev1.Container, name string) *corev1.Container { //nolint:unparam
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}

func validateSpeculatorOnPodSpec(podSpec corev1.PodSpec, expectedMethod string) {
	GinkgoHelper()

	initContainer := findInitContainerByName(podSpec.InitContainers, constants.SpeculatorInitializerContainerName)
	Expect(initContainer).ToNot(BeNil(), "expected speculator-initializer init container on pod spec")

	Expect(initContainer.Args).To(ContainElement(constants.DefaultSpeculatorLocalMountPath),
		"speculator-initializer should download to %s", constants.DefaultSpeculatorLocalMountPath)

	hasSpeculatorVolume := false
	for _, v := range podSpec.Volumes {
		if v.Name == constants.SpeculatorVolumeName {
			hasSpeculatorVolume = true
		}
	}
	Expect(hasSpeculatorVolume).To(BeTrue(), "expected speculator volume %s", constants.SpeculatorVolumeName)

	mainContainer := findContainerByName(podSpec.Containers, "main")
	Expect(mainContainer).ToNot(BeNil(), "expected main container")

	var vllmArgs string
	for _, env := range mainContainer.Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
		}
	}

	Expect(vllmArgs).To(ContainSubstring("--speculative-config"), "expected --speculative-config in VLLM_ADDITIONAL_ARGS")

	jsonStr := extractJSONFromSpeculativeConfig(vllmArgs)
	var config map[string]interface{}
	Expect(json.Unmarshal([]byte(jsonStr), &config)).To(Succeed())
	Expect(config["method"]).To(Equal(expectedMethod))
}

func extractJSONFromSpeculativeConfig(vllmArgs string) string {
	start := -1
	end := -1
	for i, c := range vllmArgs {
		if c == '\'' && start == -1 {
			start = i + 1
		} else if c == '\'' && start != -1 {
			end = i
			break
		}
	}
	if start >= 0 && end > start {
		return vllmArgs[start:end]
	}
	return ""
}
