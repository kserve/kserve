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

package v1alpha2

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
)

func TestParentRefsMatchGatewayRefs(t *testing.T) {
	tests := []struct {
		name       string
		parentRefs []gwapiv1.ParentReference
		gwRefs     []UntypedObjectReference
		want       bool
	}{
		{
			name:       "both empty",
			parentRefs: nil,
			gwRefs:     nil,
			want:       true,
		},
		{
			name: "single ref, matching",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
			},
			want: true,
		},
		{
			name: "single ref, different name",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-other", Namespace: "ns-a"},
			},
			want: false,
		},
		{
			name: "single ref, different namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-b"},
			},
			want: false,
		},
		{
			name: "parentRef with nil namespace matches empty namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1"},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: ""},
			},
			want: true,
		},
		{
			name: "parentRef with nil namespace does not match non-empty namespace",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1"},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
			},
			want: false,
		},
		{
			name: "different lengths",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
			},
			want: false,
		},
		{
			name: "multiple refs, same order",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
				{Name: "gw-2", Namespace: "ns-b"},
			},
			want: true,
		},
		{
			name: "multiple refs, different order",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-2", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
				{Name: "gw-2", Namespace: "ns-b"},
			},
			want: true,
		},
		{
			name: "multiple refs, one mismatch",
			parentRefs: []gwapiv1.ParentReference{
				{Name: "gw-1", Namespace: ptr.To(gwapiv1.Namespace("ns-a"))},
				{Name: "gw-3", Namespace: ptr.To(gwapiv1.Namespace("ns-b"))},
			},
			gwRefs: []UntypedObjectReference{
				{Name: "gw-1", Namespace: "ns-a"},
				{Name: "gw-2", Namespace: "ns-b"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parentRefsMatchGatewayRefs(tt.parentRefs, tt.gwRefs)
			if got != tt.want {
				t.Errorf("parentRefsMatchGatewayRefs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newBaseLLMInferenceServiceV1Alpha2() *LLMInferenceService {
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
		{
			name: "valid: scaling and worker both set with HPA (multi-node autoscaling)",
			workload: &WorkloadSpec{
				Worker: &corev1.PodSpec{},
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
			name: "valid: scaling and worker both set with KEDA (multi-node autoscaling)",
			workload: &WorkloadSpec{
				Worker: &corev1.PodSpec{},
				Scaling: &ScalingSpec{
					MaxReplicas: 3,
					WVA: &WVASpec{
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{},
						},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name: "valid: worker set with replicas (no scaling) - multi-node with static replicas",
			workload: &WorkloadSpec{
				Worker:   &corev1.PodSpec{},
				Replicas: ptr.To(int32(3)),
			},
			wantErrCount: 0,
		},
		{
			name:         "valid: worker set with no replicas and no scaling - multi-node defaults",
			workload:     &WorkloadSpec{Worker: &corev1.PodSpec{}},
			wantErrCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateWorkloadScaling(field.NewPath("spec"), tt.workload)
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
		svc := newBaseLLMInferenceServiceV1Alpha2()
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
		svc := newBaseLLMInferenceServiceV1Alpha2()
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
		svc := newBaseLLMInferenceServiceV1Alpha2()
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
		svc := newBaseLLMInferenceServiceV1Alpha2()
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
		svc := newBaseLLMInferenceServiceV1Alpha2()
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
		svc := newBaseLLMInferenceServiceV1Alpha2()
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
		svc := newBaseLLMInferenceServiceV1Alpha2()
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

func TestValidateActuatorConsistency(t *testing.T) {
	validator := &LLMInferenceServiceValidator{}

	t.Run("valid: decode HPA, no prefill scaling", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{HPA: &HPAScalingSpec{}}},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{Replicas: ptr.To(int32(2))}
		errs := validator.validateScaling(svc)
		assert.Empty(t, errs)
	})

	t.Run("valid: no prefill at all", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{HPA: &HPAScalingSpec{}}},
			},
		}
		errs := validator.validateScaling(svc)
		assert.Empty(t, errs)
	})

	t.Run("valid: decode scaling only, prefill has no scaling", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{KEDA: &KEDAScalingSpec{}}},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{Replicas: ptr.To(int32(3))}
		errs := validator.validateScaling(svc)
		assert.Empty(t, errs)
	})

	t.Run("valid: prefill scaling only, decode has no scaling", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{Replicas: ptr.To(int32(2))}
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{HPA: &HPAScalingSpec{}}},
			},
		}
		errs := validator.validateScaling(svc)
		assert.Empty(t, errs)
	})

	t.Run("error: decode HPA, prefill KEDA", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{HPA: &HPAScalingSpec{}}},
			},
		}
		svc.Spec.Prefill = &WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{KEDA: &KEDAScalingSpec{}}},
			},
		}

		errs := validator.validateScaling(svc)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Field, "spec.prefill.scaling.wva")
		assert.Contains(t, errs[0].Detail, "decode uses hpa but prefill uses keda")
	})

	t.Run("error: decode KEDA, prefill HPA", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{KEDA: &KEDAScalingSpec{}}},
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
		assert.Contains(t, errs[0].Field, "spec.prefill.scaling.wva")
		assert.Contains(t, errs[0].Detail, "decode uses keda but prefill uses hpa")
	})

	t.Run("valid: scaling+worker on decode workload (multi-node autoscaling)", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{
			Worker: &corev1.PodSpec{},
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{HPA: &HPAScalingSpec{}}},
			},
		}

		errs := validator.validateScaling(svc)
		require.Empty(t, errs)
	})

	t.Run("valid: scaling+worker on prefill workload (multi-node autoscaling)", func(t *testing.T) {
		svc := newBaseLLMInferenceServiceV1Alpha2()
		svc.Spec.WorkloadSpec = WorkloadSpec{Replicas: ptr.To(int32(2))}
		svc.Spec.Prefill = &WorkloadSpec{
			Worker: &corev1.PodSpec{},
			Scaling: &ScalingSpec{
				MaxReplicas: 5,
				WVA:         &WVASpec{ActuatorSpec: ActuatorSpec{KEDA: &KEDAScalingSpec{}}},
			},
		}

		errs := validator.validateScaling(svc)
		require.Empty(t, errs)
	})
}
