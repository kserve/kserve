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

package v1alpha1

import (
	"strings"
	"testing"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
)

func newBaseLLMInferenceService() *LLMInferenceService {
	return &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI: apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
			},
		},
	}
}

func TestValidateWorkloadScaling(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	tests := []struct {
		name           string
		workload       *WorkloadSpec
		wantErrCount   int
		wantErrStrings []string
	}{
		{
			name: "valid: scaling with WVA + HPA",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 5,
					WVA: &WVASpec{
						VariantCost: "10.0",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: scaling with WVA + KEDA and idleReplicaCount",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 10,
					WVA: &WVASpec{
						VariantCost: "5.0",
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								PollingInterval:  ptr.To(int32(30)),
								CooldownPeriod:   ptr.To(int32(60)),
								IdleReplicaCount: ptr.To(int32(1)),
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name:         "valid: no scaling configured",
			workload:     &WorkloadSpec{},
			wantErrCount: 0,
		},
		{
			name: "valid: scaling with only maxReplicas (no minReplicas)",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: variantCost integer format",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						VariantCost: "10",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: variantCost decimal format",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						VariantCost: "0.5",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: HPA with behavior configured",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{
								Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
									ScaleUp: &autoscalingv2.HPAScalingRules{
										StabilizationWindowSeconds: ptr.To(int32(60)),
									},
								},
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: empty variantCost (uses default)",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						VariantCost: "",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: idleReplicaCount=1 minReplicas=2 (minimum valid combo)",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 10,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								IdleReplicaCount: ptr.To(int32(1)),
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: initialCooldownPeriod set",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								InitialCooldownPeriod: ptr.To(int32(60)),
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: fallback set",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								Fallback: &kedav1alpha1.Fallback{
									FailureThreshold: 3,
									Replicas:         2,
								},
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: restoreToOriginalReplicaCount set in advanced",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								Advanced: &kedav1alpha1.AdvancedConfig{
									RestoreToOriginalReplicaCount: true,
								},
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "error: scalingModifiers target set",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								Advanced: &kedav1alpha1.AdvancedConfig{
									ScalingModifiers: kedav1alpha1.ScalingModifiers{
										Target: "10",
									},
								},
							},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"scalingModifiers must not be set"},
		},
		{
			name: "error: scalingModifiers activationTarget set",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								Advanced: &kedav1alpha1.AdvancedConfig{
									ScalingModifiers: kedav1alpha1.ScalingModifiers{
										ActivationTarget: "1",
									},
								},
							},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"scalingModifiers must not be set"},
		},
		{
			name: "error: scalingModifiers metricType set",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								Advanced: &kedav1alpha1.AdvancedConfig{
									ScalingModifiers: kedav1alpha1.ScalingModifiers{
										MetricType: autoscalingv2.AverageValueMetricType,
									},
								},
							},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"scalingModifiers must not be set"},
		},
		{
			name: "error: both scalingModifiers and hpa name set (2 errors)",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								Advanced: &kedav1alpha1.AdvancedConfig{
									ScalingModifiers: kedav1alpha1.ScalingModifiers{
										Formula: "wva_desired_replicas",
									},
									HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
										Name: "my-hpa",
									},
								},
							},
						},
					},
				},
			},
			wantErrCount:   2,
			wantErrStrings: []string{"scalingModifiers must not be set", "horizontalPodAutoscalerConfig.name must not be set"},
		},
		{
			name: "error: replicas and scaling both set",
			workload: &WorkloadSpec{
				Replicas: ptr.To(int32(3)),
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"scaling and replicas are mutually exclusive"},
		},
		{
			name: "error: minReplicas > maxReplicas",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(10)),
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"minReplicas (10) cannot exceed maxReplicas (5)"},
		},
		{
			name: "error: scaling without WVA",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"wva is required when scaling is configured"},
		},
		{
			name: "error: WVA with both HPA and KEDA",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							HPA:  &HPAScalingSpec{},
							KEDA: &KEDAScalingSpec{},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"hpa and keda are mutually exclusive"},
		},
		{
			name: "error: WVA with neither HPA nor KEDA",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA:         &WVASpec{},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"either hpa or keda must be specified"},
		},
		{
			name: "error: invalid variantCost - alphabetic",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						VariantCost: "abc",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"variantCost must be a non-negative numeric string"},
		},
		{
			name: "error: invalid variantCost - negative",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						VariantCost: "-1",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"variantCost must be a non-negative numeric string"},
		},
		{
			name: "error: invalid variantCost - multiple dots",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						VariantCost: "10.0.1",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"variantCost must be a non-negative numeric string"},
		},
		{
			name: "error: KEDA idleReplicaCount without minReplicas",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 10,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								IdleReplicaCount: ptr.To(int32(1)),
							},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"minReplicas is required when idleReplicaCount is set"},
		},
		{
			name: "error: KEDA idleReplicaCount >= minReplicas",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 10,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								IdleReplicaCount: ptr.To(int32(3)),
							},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"idleReplicaCount (3) must be less than minReplicas (2)"},
		},
		{
			name: "error: KEDA idleReplicaCount == minReplicas",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 10,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								IdleReplicaCount: ptr.To(int32(2)),
							},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"idleReplicaCount (2) must be less than minReplicas (2)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateWorkloadScaling(field.NewPath("spec"), tt.workload)
			assert.Len(t, errs, tt.wantErrCount, "expected %d errors, got %d: %v", tt.wantErrCount, len(errs), errs)
			for _, wantStr := range tt.wantErrStrings {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Error(), wantStr) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error containing %q, got: %v", wantStr, errs)
			}
		})
	}
}

func TestValidateScaling_PrefillWorkload(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	t.Run("error on prefill scaling uses correct field path", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.Prefill = &WorkloadSpec{
			Replicas: ptr.To(int32(3)),
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						HPA: &HPAScalingSpec{},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.prefill.scaling",
			"error should reference spec.prefill.scaling path")
		assert.Contains(t, errs[0].Detail, "scaling and replicas are mutually exclusive")
	})

	t.Run("both decode and prefill with matching HPA backends", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 5,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						HPA: &HPAScalingSpec{},
					},
				},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MinReplicas: ptr.To(int32(2)),
				MaxReplicas: 8,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						HPA: &HPAScalingSpec{},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		assert.Empty(t, errs, "expected no errors when both workloads use HPA")
	})

	t.Run("both decode and prefill with matching KEDA backends", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 5,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						KEDA: &KEDAScalingSpec{},
					},
				},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MinReplicas: ptr.To(int32(2)),
				MaxReplicas: 8,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						KEDA: &KEDAScalingSpec{
							IdleReplicaCount: ptr.To(int32(1)),
						},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		assert.Empty(t, errs, "expected no errors when both workloads use KEDA")
	})

	t.Run("scalingModifiers set - rejected", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						KEDA: &KEDAScalingSpec{
							Advanced: &kedav1alpha1.AdvancedConfig{
								ScalingModifiers: kedav1alpha1.ScalingModifiers{
									Formula: "wva_desired_replicas",
								},
							},
						},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeForbidden, errs[0].Type)
		assert.Contains(t, errs[0].Field, "scalingModifiers")
	})

	t.Run("horizontalPodAutoscalerConfig name set - rejected", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						KEDA: &KEDAScalingSpec{
							Advanced: &kedav1alpha1.AdvancedConfig{
								HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
									Name: "my-hpa",
								},
							},
						},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		require.Len(t, errs, 1)
		assert.Equal(t, field.ErrorTypeForbidden, errs[0].Type)
		assert.Contains(t, errs[0].Field, "horizontalPodAutoscalerConfig")
	})

	t.Run("keda advanced with only behavior - allowed", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						KEDA: &KEDAScalingSpec{
							Advanced: &kedav1alpha1.AdvancedConfig{
								HorizontalPodAutoscalerConfig: &kedav1alpha1.HorizontalPodAutoscalerConfig{
									Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{},
								},
							},
						},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		assert.Empty(t, errs, "expected no errors when only behavior is set in advanced")
	})

	t.Run("errors on both decode and prefill are reported", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		// Decode: missing WVA
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
			},
		}
		// Prefill: minReplicas > maxReplicas
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MinReplicas: ptr.To(int32(10)),
				MaxReplicas: 5,
				WVA: &WVASpec{
					ActuatorSpec: ActuatorSpec{
						HPA: &HPAScalingSpec{},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		require.Len(t, errs, 2, "expected errors from both decode and prefill workloads")

		// Check that decode error is on spec.scaling path
		foundDecodeErr := false
		foundPrefillErr := false
		for _, e := range errs {
			if strings.Contains(e.Field, "spec.scaling") && !strings.Contains(e.Field, "prefill") {
				foundDecodeErr = true
			}
			if strings.Contains(e.Field, "spec.prefill.scaling") {
				foundPrefillErr = true
			}
		}
		assert.True(t, foundDecodeErr, "expected error on spec.scaling path for decode workload")
		assert.True(t, foundPrefillErr, "expected error on spec.prefill.scaling path for prefill workload")
	})
}
