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
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller — Standalone Tokenizer", func() {
	Context("When tokenizer is explicitly configured via spec.router.scheduler.tokenizer", func() {
		It("should create tokenizer Deployment and Service from well-known config", func(ctx SpecContext) {
			svcName := "test-llm-tokenizer-explicit"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Verify tokenizer Deployment is created with correct properties
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())

				g.Expect(dep).To(BeOwnedBy(llmSvc))

				// Well-known config should provide the vllm-render container
				containerNames := containerNameList(dep)
				g.Expect(containerNames).To(ContainElement("vllm-render"))
			}).WithContext(ctx).Should(Succeed())

			// Verify tokenizer Service is created with correct port
			Eventually(func(g Gomega, ctx context.Context) {
				svc := &corev1.Service{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, svc)).To(Succeed())

				g.Expect(svc.Spec.Ports).To(HaveLen(1))
				g.Expect(svc.Spec.Ports[0].Name).To(Equal("render-http"))
				g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8000)))
				g.Expect(svc.Spec.Selector).To(Equal(llmisvc.TokenizerLabels(llmSvc)))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should generate 3-plugin pipeline in default scheduler config", func(ctx SpecContext) {
			svcName := "test-llm-tok-3plugin"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Verify scheduler config contains the 3-plugin pipeline
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-router-scheduler"),
					Namespace: testNs.Name,
				}, expectedDeployment); err != nil {
					return err
				}

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected --config-text in scheduler deployment")
				g.Expect(configText).To(ContainSubstring("token-producer"))
				g.Expect(configText).To(ContainSubstring("precise-prefix-cache-producer"))
				g.Expect(configText).To(ContainSubstring("prefix-cache-scorer"))
				g.Expect(configText).To(ContainSubstring(kmeta.ChildName(svcName, "-tokenizer")))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should report TokenizerReady status condition", func(ctx SpecContext) {
			svcName := "test-llm-tok-status"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for tokenizer deployment to exist, then simulate readiness
			ensureTokenizerDeploymentReady(ctx, envTest.Client, llmSvc)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.TokenizerReady), "True"))
			})).WithContext(ctx).Should(Succeed())
		})
	})

	Context("When tokenizer is not configured", func() {
		It("should not create tokenizer resources", func(ctx SpecContext) {
			svcName := "test-llm-no-tokenizer"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the scheduler to be created (proves reconciler ran)
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-router-scheduler"),
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Deployment should not exist
			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")
			Consistently(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should not exist")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Legacy migration — precise-prefix-cache-scorer triggers tokenizer", func() {
		It("should create tokenizer Deployment when precise-prefix-cache-scorer is in inline config", func(ctx SpecContext) {
			svcName := "test-llm-tok-legacy"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			legacyConfig := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 16
- type: queue-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(legacyConfig),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Tokenizer Deployment should be auto-provisioned
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())

				g.Expect(dep).To(BeOwnedBy(llmSvc))
				containerNames := containerNameList(dep)
				g.Expect(containerNames).To(ContainElement("vllm-render"))
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Service should also exist
			Eventually(func(g Gomega, ctx context.Context) {
				svc := &corev1.Service{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, svc)).To(Succeed())
				g.Expect(svc.Spec.Ports[0].Name).To(Equal("render-http"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Tokenizer cleanup on spec change", func() {
		It("should delete tokenizer resources when tokenizer is removed from spec", func(ctx SpecContext) {
			svcName := "test-llm-tok-cleanup"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Wait for tokenizer Deployment to exist
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Remove the tokenizer from the spec
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &v1alpha2.LLMInferenceService{}
				Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				current.Spec.Router.Scheduler.Tokenizer = nil
				return envTest.Update(ctx, current)
			})).To(Succeed())

			// Tokenizer Deployment should be deleted
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should be deleted")
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Service should also be deleted
			Eventually(func(g Gomega, ctx context.Context) {
				svc := &corev1.Service{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, svc)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Service should be deleted")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("External InferencePool ref", func() {
		It("should not create tokenizer when scheduler has external pool ref", func(ctx SpecContext) {
			svcName := "test-llm-tok-extpool"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithInferencePoolRef("external-pool"),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for reconciliation to process (workload should exist)
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve"),
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer should not exist with external pool ref
			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")
			Consistently(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should not exist with external pool ref")
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

func containerNameList(dep *appsv1.Deployment) []string {
	names := make([]string, len(dep.Spec.Template.Spec.Containers))
	for i, c := range dep.Spec.Template.Spec.Containers {
		names[i] = c.Name
	}
	return names
}

func ensureTokenizerDeploymentReady(ctx context.Context, c client.Client, llmSvc *v1alpha2.LLMInferenceService) {
	if envTest.UsingExistingCluster() {
		return
	}

	gomega.Eventually(func(g gomega.Gomega, ctx context.Context) {
		deployments := &appsv1.DeploymentList{}
		g.Expect(c.List(ctx, deployments, &client.ListOptions{
			Namespace:     llmSvc.Namespace,
			LabelSelector: labels.SelectorFromSet(llmisvc.TokenizerLabels(llmSvc)),
		})).To(Succeed())
		g.Expect(deployments.Items).NotTo(BeEmpty())

		for _, d := range deployments.Items {
			dep := d.DeepCopy()
			dep.Status.Replicas = 1
			dep.Status.ReadyReplicas = 1
			dep.Status.AvailableReplicas = 1
			dep.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionTrue,
				},
			}
			g.Expect(c.Status().Update(ctx, dep)).To(Succeed())
		}
	}).WithContext(ctx).Should(gomega.Succeed())
}
