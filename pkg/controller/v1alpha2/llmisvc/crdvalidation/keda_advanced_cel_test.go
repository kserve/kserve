/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

    10|Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package crdvalidation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

// These tests exercise CRD CEL (x-kubernetes-validations) for WVA+KEDA advanced
// fields. They catch optional-field dereference bugs that webhook unit tests miss
// (see https://github.com/kserve/kserve/issues/5936).
var _ = Describe("LLMInferenceService KEDA advanced CEL", func() {
	var ns string

	BeforeEach(func(ctx SpecContext) {
		ns = fixture.NewTestNamespace(ctx, envTest).Name
	})

	baseLLMSvc := func(name string, scaling *v1alpha2.ScalingSpec) *v1alpha2.LLMInferenceService {
		return fixture.LLMInferenceService(name,
			fixture.WithModelURI("hf://facebook/opt-125m"),
			fixture.WithScaling(scaling),
		)
	}

	It("should accept advanced HPA behavior when optional scalingModifiers and name are omitted", func(ctx SpecContext) {
		// Reproduces #5936: advanced present with only behavior set.
		llmSvc := baseLLMSvc("keda-advanced-behavior-ok", &v1alpha2.ScalingSpec{
			MinReplicas: ptr.To(int32(1)),
			MaxReplicas: 6,
			WVA: &v1alpha2.WVASpec{
				ActuatorSpec: v1alpha2.ActuatorSpec{
					KEDA: &v1alpha2.KEDAScalingSpec{
						PollingInterval: ptr.To(int32(10)),
						Advanced: &kedav1alpha1.AdvancedConfig{
							HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
								Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
									ScaleDown: &autoscalingv2.HPAScalingRules{
										StabilizationWindowSeconds: ptr.To(int32(120)),
										Policies: []autoscalingv2.HPAScalingPolicy{{
											Type:          autoscalingv2.PodsScalingPolicy,
											Value:         1,
											PeriodSeconds: 120,
										}},
									},
								},
							},
						},
					},
				},
			},
		})
		llmSvc.Namespace = ns

		Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed(),
			"CRD CEL should accept omitted optional advanced.scalingModifiers and horizontalPodAutoscalerConfig.name")
		DeferCleanup(func(ctx SpecContext) {
			Expect(envTest.Client.Delete(ctx, llmSvc)).To(Succeed())
		})
	})

	It("should accept advanced with only restoreToOriginalReplicaCount set", func(ctx SpecContext) {
		llmSvc := baseLLMSvc("keda-advanced-restore-ok", &v1alpha2.ScalingSpec{
			MaxReplicas: 5,
			WVA: &v1alpha2.WVASpec{
				ActuatorSpec: v1alpha2.ActuatorSpec{
					KEDA: &v1alpha2.KEDAScalingSpec{
						Advanced: &kedav1alpha1.AdvancedConfig{
							RestoreToOriginalReplicaCount: true,
						},
					},
				},
			},
		})
		llmSvc.Namespace = ns

		Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) {
			Expect(envTest.Client.Delete(ctx, llmSvc)).To(Succeed())
		})
	})

	It("should reject WVA KEDA scalingModifiers when set", func(ctx SpecContext) {
		llmSvc := baseLLMSvc("keda-advanced-modifiers-bad", &v1alpha2.ScalingSpec{
			MaxReplicas: 5,
			WVA: &v1alpha2.WVASpec{
				ActuatorSpec: v1alpha2.ActuatorSpec{
					KEDA: &v1alpha2.KEDAScalingSpec{
						Advanced: &kedav1alpha1.AdvancedConfig{
							ScalingModifiers: kedav1alpha1.ScalingModifiers{
								Formula: "trig0 + trig1",
							},
						},
					},
				},
			},
		})
		llmSvc.Namespace = ns

		err := envTest.Client.Create(ctx, llmSvc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("scalingModifiers must not be set"))
	})

	It("should reject horizontalPodAutoscalerConfig.name when set", func(ctx SpecContext) {
		llmSvc := baseLLMSvc("keda-advanced-hpa-name-bad", &v1alpha2.ScalingSpec{
			MaxReplicas: 5,
			WVA: &v1alpha2.WVASpec{
				ActuatorSpec: v1alpha2.ActuatorSpec{
					KEDA: &v1alpha2.KEDAScalingSpec{
						Advanced: &kedav1alpha1.AdvancedConfig{
							HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
								Name: "user-chosen-hpa",
							},
						},
					},
				},
			},
		})
		llmSvc.Namespace = ns

		err := envTest.Client.Create(ctx, llmSvc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("horizontalPodAutoscalerConfig.name must not be set"))
	})

	It("should accept LLMInferenceServiceConfig with the same omitted-optional advanced shape", func(ctx SpecContext) {
		cfg := &v1alpha2.LLMInferenceServiceConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "keda-advanced-behavior-cfg-ok",
				Namespace: ns,
			},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Scaling: &v1alpha2.ScalingSpec{
						MinReplicas: ptr.To(int32(1)),
						MaxReplicas: 6,
						WVA: &v1alpha2.WVASpec{
							ActuatorSpec: v1alpha2.ActuatorSpec{
								KEDA: &v1alpha2.KEDAScalingSpec{
									PollingInterval: ptr.To(int32(10)),
									Advanced: &kedav1alpha1.AdvancedConfig{
										HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
											Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
												ScaleDown: &autoscalingv2.HPAScalingRules{
													StabilizationWindowSeconds: ptr.To(int32(120)),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		Expect(envTest.Client.Create(ctx, cfg)).To(Succeed(),
			"Config CRD shares KEDAScalingSpec/WVASpec CEL and must accept omitted optional advanced fields")
		DeferCleanup(func(ctx SpecContext) {
			Expect(envTest.Client.Delete(ctx, cfg)).To(Succeed())
		})
	})
})
