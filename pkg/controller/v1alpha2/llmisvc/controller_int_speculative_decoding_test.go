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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Controller - Speculative Decoding", func() {
	Context("Single node with HF speculator model (eagle3)", func() {
		It("should create speculator-initializer init container and inject --speculative-config", func(ctx SpecContext) {
			svcName := "test-llm-spec-decoding-eagle3"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			modelURL, err := apis.ParseURL("hf://Qwen/Qwen3-32B-FP8")
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
						Name: ptr.To("qwen3-32b"),
						URI:  *modelURL,
					},
					Speculator: &v1alpha2.SpeculatorSpec{
						Model: &v1alpha2.LLMModelSpec{URI: *speculatorURL},
						Config: map[string]string{
							"method":                     "eagle3",
							"num_speculative_tokens":     "6",
							"draft_tensor_parallel_size": "1",
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

			podSpec := deployment.Spec.Template.Spec

			// Verify speculator-initializer init container exists
			var speculatorInit *corev1.Container
			for i := range podSpec.InitContainers {
				if podSpec.InitContainers[i].Name == constants.SpeculatorInitializerContainerName {
					speculatorInit = &podSpec.InitContainers[i]
					break
				}
			}
			Expect(speculatorInit).ToNot(BeNil(), "speculator-initializer init container should exist")
			Expect(speculatorInit.Args).To(ContainElement(speculatorURL.String()))
			Expect(speculatorInit.Args).To(ContainElement(constants.DefaultSpeculatorLocalMountPath))

			// Verify speculator volume mount on init container (read-write)
			var initMount *corev1.VolumeMount
			for i := range speculatorInit.VolumeMounts {
				if speculatorInit.VolumeMounts[i].Name == constants.SpeculatorVolumeName {
					initMount = &speculatorInit.VolumeMounts[i]
					break
				}
			}
			Expect(initMount).ToNot(BeNil())
			Expect(initMount.ReadOnly).To(BeFalse())
			Expect(initMount.MountPath).To(Equal(constants.DefaultSpeculatorLocalMountPath))

			// Verify speculator volume mount on main container (read-only)
			mainContainer := podSpec.Containers[0]
			var mainMount *corev1.VolumeMount
			for i := range mainContainer.VolumeMounts {
				if mainContainer.VolumeMounts[i].Name == constants.SpeculatorVolumeName {
					mainMount = &mainContainer.VolumeMounts[i]
					break
				}
			}
			Expect(mainMount).ToNot(BeNil())
			Expect(mainMount.ReadOnly).To(BeTrue())
			Expect(mainMount.MountPath).To(Equal(constants.DefaultSpeculatorLocalMountPath))

			// Verify VLLM_ADDITIONAL_ARGS with --speculative-config
			var vllmArgs string
			for _, env := range mainContainer.Env {
				if env.Name == "VLLM_ADDITIONAL_ARGS" {
					vllmArgs = env.Value
					break
				}
			}
			Expect(vllmArgs).To(ContainSubstring("--speculative-config"))
			Expect(vllmArgs).To(ContainSubstring(constants.DefaultSpeculatorLocalMountPath))
			Expect(vllmArgs).To(ContainSubstring("eagle3"))
		})
	})

	Context("Single node with ngram (no speculator model)", func() {
		It("should inject --speculative-config without creating speculator init container", func(ctx SpecContext) {
			svcName := "test-llm-spec-decoding-ngram"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			modelURL, err := apis.ParseURL("hf://meta-llama/Llama-2-7b")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("llama-2-7b"),
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

			podSpec := deployment.Spec.Template.Spec

			// Verify NO speculator-initializer init container
			for _, ic := range podSpec.InitContainers {
				Expect(ic.Name).ToNot(Equal(constants.SpeculatorInitializerContainerName),
					"ngram should not create speculator-initializer")
			}

			// Verify VLLM_ADDITIONAL_ARGS is set with ngram config
			mainContainer := podSpec.Containers[0]
			var vllmArgs string
			for _, env := range mainContainer.Env {
				if env.Name == "VLLM_ADDITIONAL_ARGS" {
					vllmArgs = env.Value
					break
				}
			}
			Expect(vllmArgs).To(ContainSubstring("--speculative-config"))
			Expect(vllmArgs).To(ContainSubstring("ngram"))
			Expect(vllmArgs).ToNot(ContainSubstring(constants.DefaultSpeculatorLocalMountPath),
				"ngram should not reference speculator mount path")
		})
	})

	Context("Single node with PVC speculator model", func() {
		It("should mount PVC for speculator and inject --speculative-config", func(ctx SpecContext) {
			svcName := "test-llm-spec-decoding-pvc"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			modelURL, err := apis.ParseURL("hf://meta-llama/Llama-2-7b")
			Expect(err).ToNot(HaveOccurred())
			speculatorURL, err := apis.ParseURL("pvc://speculator-pvc/eagle3-model")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("llama-2-7b"),
						URI:  *modelURL,
					},
					Speculator: &v1alpha2.SpeculatorSpec{
						Model: &v1alpha2.LLMModelSpec{URI: *speculatorURL},
						Config: map[string]string{
							"method":                 "draft_model",
							"num_speculative_tokens": "5",
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

			podSpec := deployment.Spec.Template.Spec

			// Verify PVC volume for speculator
			var hasPVCVolume bool
			for _, vol := range podSpec.Volumes {
				if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == "speculator-pvc" {
					hasPVCVolume = true
					break
				}
			}
			Expect(hasPVCVolume).To(BeTrue(), "PVC volume for speculator should be attached")

			// Verify --speculative-config
			mainContainer := podSpec.Containers[0]
			var vllmArgs string
			for _, env := range mainContainer.Env {
				if env.Name == "VLLM_ADDITIONAL_ARGS" {
					vllmArgs = env.Value
					break
				}
			}
			Expect(vllmArgs).To(ContainSubstring("--speculative-config"))
			Expect(vllmArgs).To(ContainSubstring("draft_model"))
		})
	})

	Context("Single node without speculator", func() {
		It("should not inject any speculative decoding config", func(ctx SpecContext) {
			svcName := "test-llm-no-spec-decoding"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			modelURL, err := apis.ParseURL("hf://meta-llama/Llama-2-7b")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("llama-2-7b"),
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

			podSpec := deployment.Spec.Template.Spec

			// No speculator-initializer
			for _, ic := range podSpec.InitContainers {
				Expect(ic.Name).ToNot(Equal(constants.SpeculatorInitializerContainerName))
			}

			// No --speculative-config in VLLM_ADDITIONAL_ARGS
			mainContainer := podSpec.Containers[0]
			for _, env := range mainContainer.Env {
				if env.Name == "VLLM_ADDITIONAL_ARGS" {
					Expect(env.Value).ToNot(ContainSubstring("--speculative-config"))
				}
			}
		})
	})
})
