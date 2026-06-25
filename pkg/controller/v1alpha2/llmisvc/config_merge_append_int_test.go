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
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("Merge Append Lists", func() {
	Context("when an LLMInferenceServiceConfig carries the merge-append-fields annotation", func() {
		It("should append container args from the override config to the base config args", func(ctx SpecContext) {
			// given
			svcName := "append-args"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			baseConfig := LLMInferenceServiceConfig("base-workload",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigManagedRouter(),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "kserve-container",
						Image: "quay.io/test/vllm:latest",
						Args:  []string{"--served-model-name=facebook/opt-125m", "--max-model-len=2048"},
					}},
				}),
			)

			appendConfig := LLMInferenceServiceConfig("extra-flags",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigAnnotations(map[string]string{
					v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=kserve-container].args",
				}),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "kserve-container",
						Args: []string{"--enable-lora"},
					}},
				}),
			)

			Expect(envTest.Client.Create(ctx, baseConfig)).To(Succeed())
			Expect(envTest.Client.Create(ctx, appendConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "base-workload"},
					corev1.LocalObjectReference{Name: "extra-flags"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

			// then: the deployment should have base args + appended args
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)).To(Succeed())

				containers := deployment.Spec.Template.Spec.Containers
				g.Expect(containers).NotTo(BeEmpty())

				var kserveContainer *corev1.Container
				for i := range containers {
					if containers[i].Name == "kserve-container" {
						kserveContainer = &containers[i]
						break
					}
				}
				g.Expect(kserveContainer).NotTo(BeNil(), "kserve-container not found in deployment")
				g.Expect(kserveContainer.Args).To(ContainElements(
					"--served-model-name=facebook/opt-125m",
					"--max-model-len=2048",
					"--enable-lora",
				))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should replace args when the override config has no merge-append annotation", func(ctx SpecContext) {
			// given
			svcName := "replace-args"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			baseConfig := LLMInferenceServiceConfig("base-with-args",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigManagedRouter(),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "kserve-container",
						Image: "quay.io/test/vllm:latest",
						Args:  []string{"--old-flag"},
					}},
				}),
			)

			// No merge-append annotation: standard replace behavior
			replaceConfig := LLMInferenceServiceConfig("override-flags",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "kserve-container",
						Args: []string{"--new-flag"},
					}},
				}),
			)

			Expect(envTest.Client.Create(ctx, baseConfig)).To(Succeed())
			Expect(envTest.Client.Create(ctx, replaceConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "base-with-args"},
					corev1.LocalObjectReference{Name: "override-flags"},
				),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

			// then: standard replace, only override's args should be present
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)).To(Succeed())

				containers := deployment.Spec.Template.Spec.Containers
				g.Expect(containers).NotTo(BeEmpty())

				var kserveContainer *corev1.Container
				for i := range containers {
					if containers[i].Name == "kserve-container" {
						kserveContainer = &containers[i]
						break
					}
				}
				g.Expect(kserveContainer).NotTo(BeNil())
				g.Expect(kserveContainer.Args).To(ContainElement("--new-flag"))
				g.Expect(kserveContainer.Args).NotTo(ContainElement("--old-flag"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should handle a 3-config chain with mixed append and replace", func(ctx SpecContext) {
			// given
			svcName := "chain-mixed"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			baseConfig := LLMInferenceServiceConfig("chain-base",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigManagedRouter(),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "kserve-container",
						Image: "quay.io/test/vllm:latest",
						Args:  []string{"--base-flag"},
					}},
				}),
			)

			// Second config appends
			appendConfig := LLMInferenceServiceConfig("chain-append",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigAnnotations(map[string]string{
					v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=kserve-container].args",
				}),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "kserve-container",
						Args: []string{"--appended-flag"},
					}},
				}),
			)

			Expect(envTest.Client.Create(ctx, baseConfig)).To(Succeed())
			Expect(envTest.Client.Create(ctx, appendConfig)).To(Succeed())

			// The LLMInferenceService's own spec also appends
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithAnnotations(map[string]string{
					v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=kserve-container].args",
				}),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "chain-base"},
					corev1.LocalObjectReference{Name: "chain-append"},
				),
				WithTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "kserve-container",
						Args: []string{"--svc-flag"},
					}},
				}),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

			// then: all three layers of args should be present in order
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)).To(Succeed())

				containers := deployment.Spec.Template.Spec.Containers
				g.Expect(containers).NotTo(BeEmpty())

				var kserveContainer *corev1.Container
				for i := range containers {
					if containers[i].Name == "kserve-container" {
						kserveContainer = &containers[i]
						break
					}
				}
				g.Expect(kserveContainer).NotTo(BeNil())
				g.Expect(kserveContainer.Args).To(ContainElements(
					"--base-flag",
					"--appended-flag",
					"--svc-flag",
				))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should preserve other fields when appending args", func(ctx SpecContext) {
			// given
			svcName := "append-preserve"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			baseConfig := LLMInferenceServiceConfig("preserve-base",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigModelName("facebook/opt-125m"),
				WithConfigModelURI("hf://facebook/opt-125m"),
				WithConfigManagedRouter(),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "kserve-container",
						Image: "quay.io/test/vllm:latest",
						Args:  []string{"--base-flag"},
					}},
				}),
			)

			appendConfig := LLMInferenceServiceConfig("preserve-append",
				InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name),
				WithConfigAnnotations(map[string]string{
					v1alpha2.MergeAppendFieldsAnnotation: "template.containers.[name=kserve-container].args",
				}),
				WithConfigWorkloadTemplate(&corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "kserve-container",
						Args: []string{"--extra-flag"},
					}},
				}),
			)

			Expect(envTest.Client.Create(ctx, baseConfig)).To(Succeed())
			Expect(envTest.Client.Create(ctx, appendConfig)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithBaseRefs(
					corev1.LocalObjectReference{Name: "preserve-base"},
					corev1.LocalObjectReference{Name: "preserve-append"},
				),
				WithReplicas(2),
			)

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

			// then: args appended, replicas and image preserved through merge
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)).To(Succeed())

				g.Expect(deployment.Spec.Replicas).To(Equal(ptr.To[int32](2)))

				containers := deployment.Spec.Template.Spec.Containers
				g.Expect(containers).NotTo(BeEmpty())

				var kserveContainer *corev1.Container
				for i := range containers {
					if containers[i].Name == "kserve-container" {
						kserveContainer = &containers[i]
						break
					}
				}
				g.Expect(kserveContainer).NotTo(BeNil())
				g.Expect(kserveContainer.Image).To(Equal("quay.io/test/vllm:latest"))
				g.Expect(kserveContainer.Args).To(ContainElements("--base-flag", "--extra-flag"))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
