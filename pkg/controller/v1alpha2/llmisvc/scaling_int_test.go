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
	"encoding/json"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	wvav1alpha1 "github.com/llm-d/llm-d-workload-variant-autoscaler/api/v1alpha1"
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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller - Scaling", func() {
	Context("HPA scaling", func() {
		It("should create VariantAutoscaling and HPA when HPA scaling is configured", func(ctx SpecContext) {
			svcName := "test-hpa-scaling"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(2, 10)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}

			va := &wvav1alpha1.VariantAutoscaling{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, va)).To(Succeed())
				g.Expect(va.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(va.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
				g.Expect(va.Spec.ModelID).To(Equal("meta-llama/Llama-3.1-8B"))
				g.Expect(va).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, hpaKey, hpa)).To(Succeed())
				g.Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(hpa.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
				g.Expect(hpa.Spec.MinReplicas).To(Equal(ptr.To(int32(2))))
				g.Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
				g.Expect(hpa.Spec.Metrics).To(HaveLen(1))
				g.Expect(hpa.Spec.Metrics[0].Type).To(Equal(autoscalingv2.ExternalMetricSourceType))
				g.Expect(hpa.Spec.Metrics[0].External.Metric.Name).To(Equal("wva_desired_replicas"))
				g.Expect(hpa).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			// No KEDA ScaledObject should be created
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}
			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should update HPA when scaling spec changes", func(ctx SpecContext) {
			svcName := "test-hpa-update"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}

			// Wait for initial HPA creation
			Eventually(func(g Gomega, ctx context.Context) {
				hpa := &autoscalingv2.HorizontalPodAutoscaler{}
				g.Expect(envTest.Get(ctx, hpaKey, hpa)).To(Succeed())
				g.Expect(hpa.Spec.MaxReplicas).To(Equal(int32(5)))
			}).WithContext(ctx).Should(Succeed())

			// Update scaling spec
			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = HPAScaling(3, 20)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			// Verify HPA is updated
			Eventually(func(g Gomega, ctx context.Context) {
				hpa := &autoscalingv2.HorizontalPodAutoscaler{}
				g.Expect(envTest.Get(ctx, hpaKey, hpa)).To(Succeed())
				g.Expect(hpa.Spec.MinReplicas).To(Equal(ptr.To(int32(3))))
				g.Expect(hpa.Spec.MaxReplicas).To(Equal(int32(20)))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("KEDA scaling", func() {
		It("should create VariantAutoscaling and ScaledObject when KEDA scaling is configured", func(ctx SpecContext) {
			svcName := "test-keda-scaling"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(KEDAScaling(1, 8)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			va := &wvav1alpha1.VariantAutoscaling{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, va)).To(Succeed())
				g.Expect(va.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(va.Spec.ModelID).To(Equal("meta-llama/Llama-3.1-8B"))
				g.Expect(va).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			so := &kedav1alpha1.ScaledObject{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))
				g.Expect(so.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				g.Expect(so.Spec.Triggers[0].Type).To(Equal("prometheus"))
				g.Expect(so.Spec.Triggers[0].Metadata["serverAddress"]).To(Equal("http://prometheus.monitoring:9090"))
				g.Expect(so).To(BeOwnedBy(llmSvc))
			}).WithContext(ctx).Should(Succeed())

			// No HPA should be created
			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should update ScaledObject when scaling spec changes", func(ctx SpecContext) {
			svcName := "test-keda-update"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(5))))
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = KEDAScaling(2, 15)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, soKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(2))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(15))))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Scaling cleanup", func() {
		It("should delete HPA and VA when scaling is removed", func(ctx SpecContext) {
			svcName := "test-hpa-cleanup"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
				g.Expect(envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed())
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
				err := envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "VA should be deleted")

				err = envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "HPA should be deleted")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete ScaledObject and VA when scaling is removed", func(ctx SpecContext) {
			svcName := "test-keda-cleanup"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
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
				err := envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "VA should be deleted")

				err = envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "ScaledObject should be deleted")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Prefill scaling", func() {
		It("should create separate HPA scaling resources for decode and prefill workloads", func(ctx SpecContext) {
			svcName := "test-prefill-hpa"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
				WithPrefillScaling(HPAScaling(2, 8)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			decodeVAKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			decodeHPAKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			prefillVAKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-prefill-va"), Namespace: testNs.Name}
			prefillHPAKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-prefill-hpa"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				va := &wvav1alpha1.VariantAutoscaling{}
				g.Expect(envTest.Get(ctx, decodeVAKey, va)).To(Succeed())
				g.Expect(va.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))

				hpa := &autoscalingv2.HorizontalPodAutoscaler{}
				g.Expect(envTest.Get(ctx, decodeHPAKey, hpa)).To(Succeed())
				g.Expect(hpa.Spec.MinReplicas).To(Equal(ptr.To(int32(1))))
				g.Expect(hpa.Spec.MaxReplicas).To(Equal(int32(5)))
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega, ctx context.Context) {
				va := &wvav1alpha1.VariantAutoscaling{}
				g.Expect(envTest.Get(ctx, prefillVAKey, va)).To(Succeed())
				g.Expect(va.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve-prefill")))

				hpa := &autoscalingv2.HorizontalPodAutoscaler{}
				g.Expect(envTest.Get(ctx, prefillHPAKey, hpa)).To(Succeed())
				g.Expect(hpa.Spec.MinReplicas).To(Equal(ptr.To(int32(2))))
				g.Expect(hpa.Spec.MaxReplicas).To(Equal(int32(8)))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should create separate KEDA scaling resources for decode and prefill workloads", func(ctx SpecContext) {
			svcName := "test-prefill-keda"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(KEDAScaling(1, 5)),
				WithPrefillScaling(KEDAScaling(2, 8)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			decodeVAKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			decodeSOKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}
			prefillVAKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-prefill-va"), Namespace: testNs.Name}
			prefillSOKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-prefill-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				va := &wvav1alpha1.VariantAutoscaling{}
				g.Expect(envTest.Get(ctx, decodeVAKey, va)).To(Succeed())
				g.Expect(va.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve")))

				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, decodeSOKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(1))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(5))))
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega, ctx context.Context) {
				va := &wvav1alpha1.VariantAutoscaling{}
				g.Expect(envTest.Get(ctx, prefillVAKey, va)).To(Succeed())
				g.Expect(va.Spec.ScaleTargetRef.Name).To(Equal(kmeta.ChildName(svcName, "-kserve-prefill")))

				so := &kedav1alpha1.ScaledObject{}
				g.Expect(envTest.Get(ctx, prefillSOKey, so)).To(Succeed())
				g.Expect(so.Spec.MinReplicaCount).To(Equal(ptr.To(int32(2))))
				g.Expect(so.Spec.MaxReplicaCount).To(Equal(ptr.To(int32(8))))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Actuator switch", func() {
		It("should delete HPA and create ScaledObject when switching from HPA to KEDA", func(ctx SpecContext) {
			svcName := "test-hpa-to-keda"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}
			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed())
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = KEDAScaling(1, 5)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})).To(Succeed())

				err := envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "HPA should be deleted after switch to KEDA")
			}).WithContext(ctx).Should(Succeed())

			// VA should still exist
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete ScaledObject and create HPA when switching from KEDA to HPA", func(ctx SpecContext) {
			svcName := "test-keda-to-hpa"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}
			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})).To(Succeed())
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, llmSvc, func() error {
					llmSvc.Spec.Scaling = HPAScaling(1, 5)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed())

				err := envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "ScaledObject should be deleted after switch to HPA")
			}).WithContext(ctx).Should(Succeed())

			// VA should still exist
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("VA accelerator label", func() {
		It("should propagate accelerator label from workload labels to VA", func(ctx SpecContext) {
			svcName := "test-va-accel"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
				WithWorkloadLabels(map[string]string{
					"inference.optimization/acceleratorName": "nvidia-a100",
				}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				va := &wvav1alpha1.VariantAutoscaling{}
				g.Expect(envTest.Get(ctx, vaKey, va)).To(Succeed())
				g.Expect(va.Labels).To(HaveKeyWithValue("inference.optimization/acceleratorName", "nvidia-a100"))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should set accelerator label to unknown when workload has no accelerator label", func(ctx SpecContext) {
			svcName := "test-va-no-accel"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				va := &wvav1alpha1.VariantAutoscaling{}
				g.Expect(envTest.Get(ctx, vaKey, va)).To(Succeed())
				g.Expect(va.Labels).To(HaveKeyWithValue("inference.optimization/acceleratorName", "unknown"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("No scaling configured", func() {
		It("should not create any scaling resources when scaling is nil", func(ctx SpecContext) {
			svcName := "test-no-scaling"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Consistently(func(g Gomega, ctx context.Context) {
				err := envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())

				err = envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())

				err = envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Scaling with stop annotation", func() {
		It("should delete HPA scaling resources when stop annotation is set", func(ctx SpecContext) {
			svcName := "test-hpa-stop"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScaling(1, 5)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
				g.Expect(envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})).To(Succeed())
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
				err := envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "VA should be deleted when stopped")

				err = envTest.Get(ctx, hpaKey, &autoscalingv2.HorizontalPodAutoscaler{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "HPA should be deleted when stopped")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete KEDA scaling resources when stop annotation is set", func(ctx SpecContext) {
			svcName := "test-keda-stop"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}
			soKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-keda"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})).To(Succeed())
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
				err := envTest.Get(ctx, vaKey, &wvav1alpha1.VariantAutoscaling{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "VA should be deleted when stopped")

				err = envTest.Get(ctx, soKey, &kedav1alpha1.ScaledObject{})
				g.Expect(client.IgnoreNotFound(err)).To(Succeed())
				g.Expect(err).To(HaveOccurred(), "ScaledObject should be deleted when stopped")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("HPA Behavior propagation", func() {
		It("should propagate HPA behavior from scaling spec to the HPA resource", func(ctx SpecContext) {
			svcName := "test-hpa-behavior"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			behavior := &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleUp: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: ptr.To(int32(120)),
				},
				ScaleDown: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: ptr.To(int32(300)),
					Policies: []autoscalingv2.HPAScalingPolicy{
						{
							Type:          autoscalingv2.PercentScalingPolicy,
							Value:         10,
							PeriodSeconds: 60,
						},
					},
				},
			}

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(HPAScalingWithBehavior(2, 10, behavior)),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			hpaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-hpa"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				hpa := &autoscalingv2.HorizontalPodAutoscaler{}
				g.Expect(envTest.Get(ctx, hpaKey, hpa)).To(Succeed())
				g.Expect(hpa.Spec.Behavior).ToNot(BeNil())
				g.Expect(hpa.Spec.Behavior.ScaleUp).ToNot(BeNil())
				g.Expect(hpa.Spec.Behavior.ScaleUp.StabilizationWindowSeconds).To(Equal(ptr.To(int32(120))))
				g.Expect(hpa.Spec.Behavior.ScaleDown).ToNot(BeNil())
				g.Expect(hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds).To(Equal(ptr.To(int32(300))))
				g.Expect(hpa.Spec.Behavior.ScaleDown.Policies).To(HaveLen(1))
				g.Expect(hpa.Spec.Behavior.ScaleDown.Policies[0].Type).To(Equal(autoscalingv2.PercentScalingPolicy))
				g.Expect(hpa.Spec.Behavior.ScaleDown.Policies[0].Value).To(Equal(int32(10)))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("VA VariantCost propagation", func() {
		It("should propagate variantCost from scaling spec to VA", func(ctx SpecContext) {
			svcName := "test-va-cost"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://meta-llama/Llama-3.1-8B"),
				WithModelName("meta-llama/Llama-3.1-8B"),
				WithScaling(ScalingWithVariantCost(HPAScaling(1, 5), "42.5")),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			vaKey := types.NamespacedName{Name: kmeta.ChildName(svcName, "-kserve-va"), Namespace: testNs.Name}

			Eventually(func(g Gomega, ctx context.Context) {
				va := &wvav1alpha1.VariantAutoscaling{}
				g.Expect(envTest.Get(ctx, vaKey, va)).To(Succeed())
				g.Expect(va.Spec.VariantCost).To(Equal("42.5"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("KEDA Prometheus auth propagation", func() {
		It("should include auth config on ScaledObject trigger when configured", func(ctx SpecContext) {
			svcName := "test-keda-auth"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

			cfgMap := &corev1.ConfigMap{}
			cfgMapKey := types.NamespacedName{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			}
			Expect(envTest.Get(ctx, cfgMapKey, cfgMap)).To(Succeed())

			authCfg := llmisvc.WVAAutoscalingConfig{
				Prometheus: llmisvc.PrometheusConfig{
					URL:                   "https://thanos.monitoring:9091",
					TLSInsecureSkipVerify: true,
					AuthModes:             "bearer",
					TriggerAuthName:       "prom-bearer-auth",
					TriggerAuthKind:       "ClusterTriggerAuthentication",
				},
			}
			authJSON, err := json.Marshal(authCfg)
			Expect(err).ToNot(HaveOccurred())

			errRetry := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				_, errUpdate := ctrl.CreateOrUpdate(ctx, envTest.Client, cfgMap, func() error {
					cfgMap.Data["autoscaling-wva-controller-config"] = string(authJSON)
					return nil
				})
				return errUpdate
			})
			Expect(errRetry).ToNot(HaveOccurred())

			defer func() {
				// Restore original config
				_ = retry.RetryOnConflict(retry.DefaultRetry, func() error {
					_, e := ctrl.CreateOrUpdate(ctx, envTest.Client, cfgMap, func() error {
						cfgMap.Data["autoscaling-wva-controller-config"] = `{"prometheus":{"url":"http://prometheus.monitoring:9090"}}`
						return nil
					})
					return e
				})
			}()

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
				g.Expect(so.Spec.Triggers).To(HaveLen(1))
				trigger := so.Spec.Triggers[0]
				g.Expect(trigger.Metadata["serverAddress"]).To(Equal("https://thanos.monitoring:9091"))
				g.Expect(trigger.Metadata["unsafeSsl"]).To(Equal("true"))
				g.Expect(trigger.Metadata["authModes"]).To(Equal("bearer"))
				g.Expect(trigger.AuthenticationRef).ToNot(BeNil())
				g.Expect(trigger.AuthenticationRef.Name).To(Equal("prom-bearer-auth"))
				g.Expect(trigger.AuthenticationRef.Kind).To(Equal("ClusterTriggerAuthentication"))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
