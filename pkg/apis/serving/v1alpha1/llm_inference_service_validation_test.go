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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/constants"
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

func TestValidateUpdate_DeletionBypass(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	oldSvc := newBaseLLMInferenceService()
	newSvc := newBaseLLMInferenceService()
	newSvc.Spec.Worker = &corev1.PodSpec{}

	// Without DeletionTimestamp, this should be rejected (worker without parallelism)
	warnings, err := validator.ValidateUpdate(t.Context(), oldSvc, newSvc)
	assert.Empty(t, warnings)
	assert.Error(t, err)

	// With DeletionTimestamp set, the same object should be accepted
	deletingSvc := newSvc.DeepCopy()
	now := metav1.Now()
	deletingSvc.DeletionTimestamp = &now
	warnings, err = validator.ValidateUpdate(t.Context(), oldSvc, deletingSvc)
	assert.Empty(t, warnings)
	assert.NoError(t, err)
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
			name: "valid: WVA KEDA idleReplicaCount=0 (scale-to-zero)",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 10,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								IdleReplicaCount: ptr.To(int32(0)),
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
			name: "error: scaling without wva or keda",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"either wva or keda must be specified when scaling is configured"},
		},
		{
			name: "error: scaling with both WVA and direct KEDA",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
					KEDA: &DirectKEDAScalingSpec{
						Triggers: []kedav1alpha1.ScaleTriggers{
							{Type: "cpu", Metadata: map[string]string{"value": "80"}},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"wva and keda are mutually exclusive"},
		},
		{
			name: "valid: scaling with direct KEDA",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 5,
					KEDA: &DirectKEDAScalingSpec{
						KEDAScalingSpec: KEDAScalingSpec{
							PollingInterval: ptr.To(int32(30)),
						},
						Triggers: []kedav1alpha1.ScaleTriggers{
							{
								Type: "cpu",
								Metadata: map[string]string{
									"value": "80",
								},
							},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: direct KEDA idleReplicaCount=0 (scale-to-zero)",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 5,
					KEDA: &DirectKEDAScalingSpec{
						KEDAScalingSpec: KEDAScalingSpec{
							IdleReplicaCount: ptr.To(int32(0)),
						},
						Triggers: []kedav1alpha1.ScaleTriggers{
							{Type: "cpu", Metadata: map[string]string{"value": "80"}},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: direct KEDA with scalingModifiers",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					KEDA: &DirectKEDAScalingSpec{
						KEDAScalingSpec: KEDAScalingSpec{
							Advanced: &kedav1alpha1.AdvancedConfig{
								ScalingModifiers: kedav1alpha1.ScalingModifiers{
									Formula: "trig0 + trig1",
									Target:  "10",
								},
							},
						},
						Triggers: []kedav1alpha1.ScaleTriggers{
							{Type: "cpu", Metadata: map[string]string{"value": "80"}},
							{Type: "memory", Metadata: map[string]string{"value": "70"}},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "error: direct KEDA without triggers",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 5,
					KEDA:        &DirectKEDAScalingSpec{},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"at least one trigger is required when using direct KEDA scaling"},
		},
		{
			name: "error: direct KEDA idleReplicaCount without minReplicas",
			workload: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MaxReplicas: 10,
					KEDA: &DirectKEDAScalingSpec{
						KEDAScalingSpec: KEDAScalingSpec{
							IdleReplicaCount: ptr.To(int32(1)),
						},
						Triggers: []kedav1alpha1.ScaleTriggers{
							{Type: "cpu", Metadata: map[string]string{"value": "80"}},
						},
					},
				},
			},
			wantErrCount:   1,
			wantErrStrings: []string{"minReplicas is required when idleReplicaCount is set"},
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

	t.Run("both decode and prefill with matching direct KEDA scaling modes", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				KEDA: &DirectKEDAScalingSpec{
					Triggers: []kedav1alpha1.ScaleTriggers{
						{Type: "cpu", Metadata: map[string]string{"value": "80"}},
					},
				},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 8,
				KEDA: &DirectKEDAScalingSpec{
					KEDAScalingSpec: KEDAScalingSpec{
						IdleReplicaCount: ptr.To(int32(1)),
					},
					Triggers: []kedav1alpha1.ScaleTriggers{
						{Type: "memory", Metadata: map[string]string{"value": "70"}},
					},
				},
				MinReplicas: ptr.To(int32(2)),
			},
		}

		errs := validator.validateScaling(svc)
		assert.Empty(t, errs, "expected no errors when both workloads use direct KEDA")
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

	t.Run("error: decode direct KEDA, prefill WVA", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				KEDA: &DirectKEDAScalingSpec{
					Triggers: []kedav1alpha1.ScaleTriggers{
						{Type: "cpu", Metadata: map[string]string{"value": "80"}},
					},
				},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{HPA: &HPAScalingSpec{}}},
			},
		}

		errs := validator.validateScaling(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.prefill.scaling")
		assert.Contains(t, errs[0].Detail, "decode uses direct keda but prefill uses wva")
	})

	t.Run("error: decode WVA, prefill direct KEDA", func(t *testing.T) {
		svc := newBaseLLMInferenceService()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{KEDA: &KEDAScalingSpec{}}},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				KEDA: &DirectKEDAScalingSpec{
					Triggers: []kedav1alpha1.ScaleTriggers{
						{Type: "memory", Metadata: map[string]string{"value": "70"}},
					},
				},
			},
		}

		errs := validator.validateScaling(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.prefill.scaling")
		assert.Contains(t, errs[0].Detail, "decode uses wva but prefill uses direct keda")
	})
}

func TestValidateTrafficFields_V1Alpha1(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	tests := []struct {
		name      string
		llmSvc    *LLMInferenceService
		wantErr   bool
		errFields []string
	}{
		{
			name: "valid: group + weight + http",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Group:  ptr.To("llama-70b"),
							Weight: ptr.To(int32(9)),
							HTTP:   &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid: weight without group",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Weight: ptr.To(int32(9)),
							HTTP:   &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
					},
				},
			},
			wantErr:   true,
			errFields: []string{"spec.router.route.group"},
		},
		{
			name: "invalid: group without weight",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Group: ptr.To("llama-70b"),
							HTTP:  &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
					},
				},
			},
			wantErr:   true,
			errFields: []string{"spec.router.route.weight"},
		},
		{
			name: "invalid: group + weight with ingress",
			llmSvc: &LLMInferenceService{
				Spec: LLMInferenceServiceSpec{
					Router: &RouterSpec{
						Route: &GatewayRoutesSpec{
							Group:  ptr.To("llama-70b"),
							Weight: ptr.To(int32(9)),
							HTTP:   &HTTPRouteSpec{Spec: &gwapiv1.HTTPRouteSpec{}},
						},
						Ingress: &IngressSpec{},
					},
				},
			},
			wantErr:   true,
			errFields: []string{"spec.router.route.group"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateTrafficFields(tt.llmSvc)
			if tt.wantErr {
				require.NotEmpty(t, errs, "expected validation errors")
				for _, expectedField := range tt.errFields {
					found := false
					for _, err := range errs {
						if err.Field == expectedField {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error on field %s, got errors: %v", expectedField, errs)
				}
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateLoRAAdapters_V1Alpha1(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	makeAdapter := func(name, uri string) LLMModelSpec {
		return LLMModelSpec{URI: apis.URL{Scheme: "hf", Host: uri}, Name: ptr.To(name)}
	}

	makeSvc := func(modelName string, loraSpec *LoRASpec) *LLMInferenceService {
		svc := newBaseLLMInferenceService()
		svc.Spec.Model.Name = ptr.To(modelName)
		svc.Spec.Model.LoRA = loraSpec
		return svc
	}

	tests := []struct {
		name           string
		svc            *LLMInferenceService
		wantErrCount   int
		wantErrStrings []string
	}{
		{
			name:         "no lora",
			svc:          makeSvc("base", nil),
			wantErrCount: 0,
		},
		{
			name: "valid single adapter",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{makeAdapter("adapter-1", "adapter-1")},
			}),
			wantErrCount: 0,
		},
		{
			name: "adapter name missing",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{{URI: apis.URL{Scheme: "hf", Host: "adapter-1"}}},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"spec.model.lora.adapters[0].name"},
		},
		{
			name: "path traversal rejected",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{makeAdapter("..", "adapter-dotdot")},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"path traversal"},
		},
		{
			name: "duplicate adapter names",
			svc: makeSvc("base", &LoRASpec{
				Adapters: []LLMModelSpec{
					makeAdapter("dup", "adapter-1"),
					makeAdapter("dup", "adapter-2"),
				},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"duplicate"},
		},
		{
			name: "adapter name same as base model",
			svc: makeSvc("base-model", &LoRASpec{
				Adapters: []LLMModelSpec{makeAdapter("base-model", "adapter-1")},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"adapter name must differ from base model name"},
		},
		{
			name: "maxRank zero invalid",
			svc: makeSvc("base", &LoRASpec{
				MaxRank:  ptr.To(int32(0)),
				Adapters: []LLMModelSpec{makeAdapter("a", "a")},
			}),
			wantErrCount:   1,
			wantErrStrings: []string{"maxRank"},
		},
		{
			name: "all lora params valid",
			svc: makeSvc("base", &LoRASpec{
				MaxRank:        ptr.To(int32(128)),
				MaxAdapters:    ptr.To(int32(4)),
				MaxCpuAdapters: ptr.To(int32(8)),
				Adapters: []LLMModelSpec{
					makeAdapter("adapter-1", "adapter-1"),
					makeAdapter("adapter-2", "adapter-2"),
				},
			}),
			wantErrCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateLoRAAdapters(tt.svc)
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

func TestValidateManagedDRAAnnotations_V1Alpha1(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	tests := []struct {
		name         string
		annotations  map[string]string
		wantErrCount int
		wantErrField string
	}{
		{
			name:         "no DRA annotations",
			annotations:  nil,
			wantErrCount: 0,
		},
		{
			name: "valid: device class only",
			annotations: map[string]string{
				constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			},
			wantErrCount: 0,
		},
		{
			name: "invalid: device count without device class",
			annotations: map[string]string{
				constants.ManagedDRADeviceCountAnnotationKey: "2",
			},
			wantErrCount: 1,
			wantErrField: constants.ManagedDRADeviceClassAnnotationKey,
		},
		{
			name: "invalid: empty device class",
			annotations: map[string]string{
				constants.ManagedDRADeviceClassAnnotationKey: "   ",
			},
			wantErrCount: 1,
			wantErrField: constants.ManagedDRADeviceClassAnnotationKey,
		},
		{
			name: "invalid: non-numeric device count",
			annotations: map[string]string{
				constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
				constants.ManagedDRADeviceCountAnnotationKey: "abc",
			},
			wantErrCount: 1,
			wantErrField: constants.ManagedDRADeviceCountAnnotationKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newBaseLLMInferenceService()
			svc.Annotations = tt.annotations

			// Exercise the full validate() path to ensure DRA validation is wired in.
			err := validator.validate(t.Context(), nil, svc)
			if tt.wantErrCount == 0 {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrField)
			}
		})
	}
}
