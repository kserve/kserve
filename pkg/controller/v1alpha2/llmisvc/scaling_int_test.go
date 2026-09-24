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

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller - Scaling", func() {
	Context("Direct KEDA scaling", func() {
		It("should create ScaledObject with user triggers and without legacy annotations", func(ctx SpecContext) {
			svcName := "test-direct-keda-scaling"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 8,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
					kedav1alpha1.ScaleTriggers{
						Type: "memory",
						Metadata: map[string]string{
							"value": "70",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			so := &kedav1alpha1.ScaledObject{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(so.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.Triggers).To(HaveLen(2))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Spec.Triggers[1].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[1].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
				g.Expect(so).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should create ScaledObject targeting LeaderWorkerSet with user triggers and without legacy annotations when worker is set", func(ctx SpecContext) {
			svcName := "test-direct-keda-lws"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithParallelism(ParallelismSpec(
					WithDataParallelism(2),
					WithDataLocalParallelism(1),
				)),
				WithWorker(&corev1.PodSpec{
					Containers: []corev1.Container{{Name: "worker", Image: "vllm:latest"}},
				}),
				WithScaling(DirectKEDAScaling(1, 8,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
					kedav1alpha1.ScaleTriggers{
						Type: "memory",
						Metadata: map[string]string{
							"value": "70",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			so := &kedav1alpha1.ScaledObject{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.APIVersion).To(Equal(lwsapi.GroupVersion.String()))
				g.Expect(so.Spec.ScaleTargetRef.Kind).To(Equal("LeaderWorkerSet"))
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve-mn")))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.Triggers).To(HaveLen(2))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Spec.Triggers[1].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[1].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
				g.Expect(so).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should accept idleReplicaCount=0 for scale-to-zero", func(ctx SpecContext) {
			svcName := "test-direct-keda-scale-to-zero"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScalingWithIdleReplicaCount(1, 8, 0,
					kedav1alpha1.ScaleTriggers{
						Type:     "cpu",
						Metadata: map[string]string{"value": "80"},
					},
				)),
			)

			// Regression test: the CRD schema previously rejected idleReplicaCount=0
			// (minimum:1), which made true scale-to-zero unreachable.
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			so := &kedav1alpha1.ScaledObject{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.IdleReplicaCount).To(Equal(ptr.To(int32(0))))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should reject create when direct KEDA scaling has no triggers", func(ctx SpecContext) {
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService("test-direct-keda-no-triggers",
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithScaling(&v1alpha2.ScalingSpec{
					MaxReplicas: 5,
					KEDA: &v1alpha2.DirectKEDAScalingSpec{
						Triggers: []kedav1alpha1.ScaleTriggers{},
					},
				}),
			)

			errValidation := envTest.Create(ctx, llmSvc)

			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail when direct KEDA triggers are empty")
			Expect(errValidation.Error()).To(ContainSubstring("at least one trigger is required when using direct KEDA scaling"))
		})

		It("should update direct KEDA triggers in place without adding legacy annotations", func(ctx SpecContext) {
			svcName := "test-direct-keda-trigger-update"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(5))))
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = DirectKEDAScaling(2, 10,
						kedav1alpha1.ScaleTriggers{
							Type: "cpu",
							Metadata: map[string]string{
								"value": "60",
							},
						},
						kedav1alpha1.ScaleTriggers{
							Type: "memory",
							Metadata: map[string]string{
								"value": "70",
							},
						},
					)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(2))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(10))))
				g.Expect(so.Spec.Triggers).To(HaveLen(2))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("60"))
				g.Expect(so.Spec.Triggers[1].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[1].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete ScaledObject when direct KEDA scaling is removed", func(ctx SpecContext) {
			svcName := "test-direct-keda-cleanup"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = nil
					llmSvc.Spec.Replicas = ptr.To(int32(3))
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "ScaledObject should be deleted")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Prefill scaling", func() {
		It("should create separate direct KEDA scaling resources for decode and prefill workloads", func(ctx SpecContext) {
			svcName := "test-prefill-direct-keda"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(DirectKEDAScaling(1, 5,
					kedav1alpha1.ScaleTriggers{
						Type: "cpu",
						Metadata: map[string]string{
							"value": "80",
						},
					},
				)),
				WithPrefillScaling(DirectKEDAScaling(2, 8,
					kedav1alpha1.ScaleTriggers{
						Type: "memory",
						Metadata: map[string]string{
							"value": "70",
						},
					},
				)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			decodeSOKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}
			prefillSOKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-prefill-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, decodeSOKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(5))))
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("cpu"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("80"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, prefillSOKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve-prefill")))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(2))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("memory"))
				g.Expect(so.Spec.Triggers[0].Metadata["value"]).To(Equal("70"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/managed"))
				g.Expect(so.Annotations).NotTo(HaveKey("llm-d.ai/model-id"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("No scaling configured", func() {
		It("should not create any scaling resources when scaling is nil", func(ctx SpecContext) {
			svcName := "test-no-scaling"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())

				err = envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Scaling with stop annotation", func() {
		It("should delete KEDA scaling resources when stop annotation is set", func(ctx SpecContext) {
			svcName := "test-keda-stop"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(KEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					WithAnnotations(map[string]string{
						constants.StopAnnotationKey: "true",
					})(llmSvc)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "ScaledObject should be deleted when stopped")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("ScalingReady condition propagation", func() {
		It("should not have ScalingReady when no scaling is configured", func(ctx SpecContext) {
			svcName := "test-no-scaling-cond"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the controller to reconcile (WorkloadsReady should appear)
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.GetCondition(v1alpha2.WorkloadReady)).ToNot(BeNil())
			}).WithContext(ctx).Should(Succeed())

			// Verify ScalingReady is absent
			Consistently(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.GetCondition(v1alpha2.ScalingReady)).To(BeNil(),
					"ScalingReady should be absent when no scaling is configured")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should set ScalingReady=False when KEDA ScaledObject is not yet ready", func(ctx SpecContext) {
			svcName := "test-keda-cond-prog"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(KEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady condition should exist")
				g.Expect(scalingCond.IsFalse()).To(BeTrue(), "ScalingReady should be False when ScaledObject has no conditions")
				g.Expect(scalingCond.Reason).To(Equal("ScaledObjectProgressing"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should set ScalingReady=True when KEDA ScaledObject reports Ready", func(ctx SpecContext) {
			svcName := "test-keda-cond-ok"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(KEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Simulate KEDA setting ready conditions
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				so := &kedav1alpha1.ScaledObject{}
				if err := envTest.Get(ctx, soKey, so); err != nil {
					return err
				}
				so.Status.Conditions = kedav1alpha1.Conditions{
					{Type: kedav1alpha1.ConditionReady, Status: "True", Reason: "ScaledObjectReady"},
					{Type: kedav1alpha1.ConditionActive, Status: "True"},
					{Type: kedav1alpha1.ConditionFallback, Status: "False"},
					{Type: kedav1alpha1.ConditionPaused, Status: "False"},
				}
				return envTest.Status().Update(ctx, so)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady condition should exist")
				g.Expect(scalingCond.IsTrue()).To(BeTrue(), "ScalingReady should be True when ScaledObject is ready")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should transition ScalingReady from True to False when KEDA ScaledObject degrades", func(ctx SpecContext) {
			svcName := "test-keda-true-false"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(KEDAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Phase 1: set ScaledObject to ready
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				so := &kedav1alpha1.ScaledObject{}
				if err := envTest.Get(ctx, soKey, so); err != nil {
					return err
				}
				so.Status.Conditions = kedav1alpha1.Conditions{
					{Type: kedav1alpha1.ConditionReady, Status: "True", Reason: "ScaledObjectReady"},
					{Type: kedav1alpha1.ConditionActive, Status: "True"},
					{Type: kedav1alpha1.ConditionFallback, Status: "False"},
					{Type: kedav1alpha1.ConditionPaused, Status: "False"},
				}
				return envTest.Status().Update(ctx, so)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil())
				g.Expect(scalingCond.IsTrue()).To(BeTrue(), "ScalingReady should be True initially")
			}).WithContext(ctx).Should(Succeed())

			// Phase 2: degrade ScaledObject (trigger error)
			errRetry = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				so := &kedav1alpha1.ScaledObject{}
				if err := envTest.Get(ctx, soKey, so); err != nil {
					return err
				}
				so.Status.Conditions = kedav1alpha1.Conditions{
					{Type: kedav1alpha1.ConditionReady, Status: "False", Reason: "TriggerError", Message: "prometheus query failed"},
					{Type: kedav1alpha1.ConditionActive, Status: "False"},
					{Type: kedav1alpha1.ConditionFallback, Status: "False"},
					{Type: kedav1alpha1.ConditionPaused, Status: "False"},
				}
				return envTest.Status().Update(ctx, so)
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady condition should still exist")
				g.Expect(scalingCond.IsFalse()).To(BeTrue(), "ScalingReady should flip to False after ScaledObject degrades")
				g.Expect(scalingCond.Reason).To(Equal("TriggerError"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should set PrefillScalingReady when prefill scaling is configured", func(ctx SpecContext) {
			svcName := "test-prefill-scale"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
				WithPrefillScaling(HPAScaling(1, 3)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())

				scalingCond := current.Status.GetCondition(v1alpha2.ScalingReady)
				g.Expect(scalingCond).ToNot(BeNil(), "ScalingReady should exist for decode workload")

				prefillScalingCond := current.Status.GetCondition(v1alpha2.PrefillScalingReady)
				g.Expect(prefillScalingCond).ToNot(BeNil(), "PrefillScalingReady should exist for prefill workload")
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
