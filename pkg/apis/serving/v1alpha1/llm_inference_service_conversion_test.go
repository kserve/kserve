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
	"testing"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	igwapiv1 "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"

	autoscalingv2 "k8s.io/api/autoscaling/v2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestLLMInferenceServiceConversion_PreservesCriticality(t *testing.T) {
	criticality := Critical
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:         apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name:        &modelName,
				Criticality: &criticality,
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify the criticality is stored in annotations
	assert.NotNil(t, dst.Annotations)
	assert.Equal(t, string(criticality), dst.Annotations[ModelCriticalityAnnotationKey])

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify the criticality is restored
	assert.NotNil(t, restored.Spec.Model.Criticality)
	assert.Equal(t, criticality, *restored.Spec.Model.Criticality)

	// Verify the annotation is cleaned up
	_, hasAnnotation := restored.Annotations[ModelCriticalityAnnotationKey]
	assert.False(t, hasAnnotation, "Annotation should be cleaned up after restoration")
}

func TestLLMInferenceServiceConversion_PreservesLoRACriticalities(t *testing.T) {
	modelCriticality := Critical
	adapter1Criticality := Standard
	adapter2Criticality := Sheddable
	modelName := "base-model"
	adapter1Name := "adapter-1"
	adapter2Name := "adapter-2"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-lora",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:         apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name:        &modelName,
				Criticality: &modelCriticality,
				LoRA: &LoRASpec{
					Adapters: []LLMModelSpec{
						{
							URI:         apis.URL{Scheme: "hf", Host: "adapter-1"},
							Name:        &adapter1Name,
							Criticality: &adapter1Criticality,
						},
						{
							URI:         apis.URL{Scheme: "hf", Host: "adapter-2"},
							Name:        &adapter2Name,
							Criticality: &adapter2Criticality,
						},
					},
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify criticalities are stored in annotations
	assert.NotNil(t, dst.Annotations)
	assert.Equal(t, string(modelCriticality), dst.Annotations[ModelCriticalityAnnotationKey])
	assert.Contains(t, dst.Annotations, LoRACriticalitiesAnnotationKey)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify the model criticality is restored
	assert.NotNil(t, restored.Spec.Model.Criticality)
	assert.Equal(t, modelCriticality, *restored.Spec.Model.Criticality)

	// Verify LoRA adapter criticalities are restored
	assert.NotNil(t, restored.Spec.Model.LoRA)
	assert.Len(t, restored.Spec.Model.LoRA.Adapters, 2)
	assert.NotNil(t, restored.Spec.Model.LoRA.Adapters[0].Criticality)
	assert.Equal(t, adapter1Criticality, *restored.Spec.Model.LoRA.Adapters[0].Criticality)
	assert.NotNil(t, restored.Spec.Model.LoRA.Adapters[1].Criticality)
	assert.Equal(t, adapter2Criticality, *restored.Spec.Model.LoRA.Adapters[1].Criticality)

	// Verify annotations are cleaned up
	_, hasModelAnnotation := restored.Annotations[ModelCriticalityAnnotationKey]
	assert.False(t, hasModelAnnotation, "Model criticality annotation should be cleaned up")
	_, hasLoRAAnnotation := restored.Annotations[LoRACriticalitiesAnnotationKey]
	assert.False(t, hasLoRAAnnotation, "LoRA criticalities annotation should be cleaned up")
}

func TestLLMInferenceServiceConversion_NoCriticality(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-no-crit",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
				// No criticality set
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify no criticality annotation is created
	if dst.Annotations != nil {
		_, hasAnnotation := dst.Annotations[ModelCriticalityAnnotationKey]
		assert.False(t, hasAnnotation, "No annotation should be created when criticality is not set")
	}

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify criticality remains nil
	assert.Nil(t, restored.Spec.Model.Criticality)
}

func TestLLMInferenceServiceConfigConversion_PreservesCriticality(t *testing.T) {
	criticality := Standard
	modelName := "config-model"

	src := &LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-config",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:         apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name:        &modelName,
				Criticality: &criticality,
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceServiceConfig{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify the criticality is stored in annotations
	assert.NotNil(t, dst.Annotations)
	assert.Equal(t, string(criticality), dst.Annotations[ModelCriticalityAnnotationKey])

	// Convert back to v1alpha1
	restored := &LLMInferenceServiceConfig{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify the criticality is restored
	assert.NotNil(t, restored.Spec.Model.Criticality)
	assert.Equal(t, criticality, *restored.Spec.Model.Criticality)
}

func TestLLMInferenceServiceConversion_PreservesExistingAnnotations(t *testing.T) {
	criticality := Critical
	modelName := "test-model"
	existingAnnotation := "existing-value"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-with-annotations",
			Namespace: "default",
			Annotations: map[string]string{
				"existing-annotation": existingAnnotation,
			},
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:         apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name:        &modelName,
				Criticality: &criticality,
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify both annotations exist
	assert.Equal(t, existingAnnotation, dst.Annotations["existing-annotation"])
	assert.Equal(t, string(criticality), dst.Annotations[ModelCriticalityAnnotationKey])

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify the criticality is restored
	assert.NotNil(t, restored.Spec.Model.Criticality)
	assert.Equal(t, criticality, *restored.Spec.Model.Criticality)

	// Verify existing annotation is preserved
	assert.Equal(t, existingAnnotation, restored.Annotations["existing-annotation"])

	// Verify criticality annotation is cleaned up
	_, hasAnnotation := restored.Annotations[ModelCriticalityAnnotationKey]
	assert.False(t, hasAnnotation, "Criticality annotation should be cleaned up")
}

func TestLLMInferenceServiceConversion_PreservesInferencePoolSpec(t *testing.T) {
	modelName := "test-model"
	eppGroup := igwapiv1alpha2.Group("")
	eppKind := igwapiv1alpha2.Kind("Service")
	eppPort := igwapiv1alpha2.PortNumber(9002)
	eppFailureMode := igwapiv1alpha2.ExtensionFailureMode("FailClose")

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-pool",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			Router: &RouterSpec{
				Scheduler: &SchedulerSpec{
					Replicas: ptr.To(int32(1)),
					Pool: &InferencePoolSpec{
						Spec: &igwapiv1alpha2.InferencePoolSpec{
							Selector: map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{
								"app": "vllm",
							},
							TargetPortNumber: 8000,
							ExtensionRef: igwapiv1alpha2.Extension{
								Group:       &eppGroup,
								Kind:        &eppKind,
								Name:        "my-epp",
								PortNumber:  &eppPort,
								FailureMode: &eppFailureMode,
							},
						},
					},
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify the pool spec was converted to GIE v1 format
	require.NotNil(t, dst.Spec.Router)
	require.NotNil(t, dst.Spec.Router.Scheduler)
	require.NotNil(t, dst.Spec.Router.Scheduler.Pool)
	require.NotNil(t, dst.Spec.Router.Scheduler.Pool.Spec, "Pool.Spec must not be nil after conversion")

	v1Spec := dst.Spec.Router.Scheduler.Pool.Spec
	assert.Equal(t, igwapiv1.PortNumber(8000), v1Spec.TargetPorts[0].Number)
	assert.Equal(t, igwapiv1.LabelValue("vllm"), v1Spec.Selector.MatchLabels["app"])
	assert.Equal(t, igwapiv1.ObjectName("my-epp"), v1Spec.EndpointPickerRef.Name)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify the pool spec round-trips correctly back to GIE v1alpha2 format
	require.NotNil(t, restored.Spec.Router)
	require.NotNil(t, restored.Spec.Router.Scheduler)
	require.NotNil(t, restored.Spec.Router.Scheduler.Pool)
	require.NotNil(t, restored.Spec.Router.Scheduler.Pool.Spec, "Pool.Spec must not be nil after round-trip")

	v1a2Spec := restored.Spec.Router.Scheduler.Pool.Spec
	assert.Equal(t, int32(8000), v1a2Spec.TargetPortNumber)
	assert.Equal(t, igwapiv1alpha2.LabelValue("vllm"), v1a2Spec.Selector["app"])
	assert.Equal(t, igwapiv1alpha2.ObjectName("my-epp"), v1a2Spec.ExtensionRef.Name)
}

func TestLLMInferenceServiceConversion_PreservesPoolRef(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-pool-ref",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			Router: &RouterSpec{
				Scheduler: &SchedulerSpec{
					Pool: &InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{
							Name: "external-pool",
						},
					},
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	require.NotNil(t, dst.Spec.Router.Scheduler.Pool)
	assert.Nil(t, dst.Spec.Router.Scheduler.Pool.Spec, "Pool.Spec must be nil when using Ref")
	require.NotNil(t, dst.Spec.Router.Scheduler.Pool.Ref)
	assert.Equal(t, "external-pool", dst.Spec.Router.Scheduler.Pool.Ref.Name)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	require.NotNil(t, restored.Spec.Router.Scheduler.Pool)
	assert.Nil(t, restored.Spec.Router.Scheduler.Pool.Spec, "Pool.Spec must remain nil after round-trip")
	require.NotNil(t, restored.Spec.Router.Scheduler.Pool.Ref)
	assert.Equal(t, "external-pool", restored.Spec.Router.Scheduler.Pool.Ref.Name)
}

func TestLLMInferenceServiceConversion_PreservesSchedulerConfig(t *testing.T) {
	modelName := "test-model"
	eppConfig := `{"scheduling":"least-load"}`

	tests := []struct {
		name   string
		config *SchedulerConfigSpec
	}{
		{
			name: "inline config",
			config: &SchedulerConfigSpec{
				Inline: &runtime.RawExtension{Raw: []byte(eppConfig)},
			},
		},
		{
			name: "config ref",
			config: &SchedulerConfigSpec{
				Ref: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "my-scheduler-config"},
					Key:                  "epp",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-isvc-scheduler-config",
					Namespace: "default",
				},
				Spec: LLMInferenceServiceSpec{
					Model: LLMModelSpec{
						URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
						Name: &modelName,
					},
					Router: &RouterSpec{
						Scheduler: &SchedulerSpec{
							Config: tt.config,
						},
					},
				},
			}

			// Convert to v1alpha2 (hub)
			dst := &v1alpha2.LLMInferenceService{}
			err := src.ConvertTo(dst)
			require.NoError(t, err)

			require.NotNil(t, dst.Spec.Router)
			require.NotNil(t, dst.Spec.Router.Scheduler)
			require.NotNil(t, dst.Spec.Router.Scheduler.Config,
				"Scheduler.Config must not be lost during conversion to v1alpha2")

			if tt.config.Inline != nil {
				assert.Equal(t, tt.config.Inline.Raw, dst.Spec.Router.Scheduler.Config.Inline.Raw)
			}
			if tt.config.Ref != nil {
				assert.Equal(t, tt.config.Ref.Name, dst.Spec.Router.Scheduler.Config.Ref.Name)
				assert.Equal(t, tt.config.Ref.Key, dst.Spec.Router.Scheduler.Config.Ref.Key)
			}

			// Convert back to v1alpha1
			restored := &LLMInferenceService{}
			err = restored.ConvertFrom(dst)
			require.NoError(t, err)

			require.NotNil(t, restored.Spec.Router)
			require.NotNil(t, restored.Spec.Router.Scheduler)
			require.NotNil(t, restored.Spec.Router.Scheduler.Config,
				"Scheduler.Config must not be lost during round-trip")

			if tt.config.Inline != nil {
				assert.Equal(t, tt.config.Inline.Raw, restored.Spec.Router.Scheduler.Config.Inline.Raw)
			}
			if tt.config.Ref != nil {
				assert.Equal(t, tt.config.Ref.Name, restored.Spec.Router.Scheduler.Config.Ref.Name)
				assert.Equal(t, tt.config.Ref.Key, restored.Spec.Router.Scheduler.Config.Ref.Key)
			}
		})
	}
}

func TestLLMInferenceServiceConversion_ScalingSpecWithHPA(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-scaling-hpa",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			WorkloadSpec: WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 10,
					WVA: &WVASpec{
						VariantCost: "15.0",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{
								Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
									ScaleUp: &autoscalingv2.HPAScalingRules{
										StabilizationWindowSeconds: ptr.To(int32(60)),
									},
									ScaleDown: &autoscalingv2.HPAScalingRules{
										StabilizationWindowSeconds: ptr.To(int32(300)),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify the scaling spec is converted
	require.NotNil(t, dst.Spec.Scaling)
	assert.Equal(t, int32(2), *dst.Spec.Scaling.MinReplicas)
	assert.Equal(t, int32(10), dst.Spec.Scaling.MaxReplicas)
	require.NotNil(t, dst.Spec.Scaling.WVA)
	assert.Equal(t, "15.0", dst.Spec.Scaling.WVA.VariantCost)
	require.NotNil(t, dst.Spec.Scaling.WVA.HPA)
	assert.Nil(t, dst.Spec.Scaling.WVA.KEDA)
	require.NotNil(t, dst.Spec.Scaling.WVA.HPA.Behavior)
	assert.Equal(t, int32(60), *dst.Spec.Scaling.WVA.HPA.Behavior.ScaleUp.StabilizationWindowSeconds)
	assert.Equal(t, int32(300), *dst.Spec.Scaling.WVA.HPA.Behavior.ScaleDown.StabilizationWindowSeconds)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify round-trip
	require.NotNil(t, restored.Spec.Scaling)
	assert.Equal(t, int32(2), *restored.Spec.Scaling.MinReplicas)
	assert.Equal(t, int32(10), restored.Spec.Scaling.MaxReplicas)
	require.NotNil(t, restored.Spec.Scaling.WVA)
	assert.Equal(t, "15.0", restored.Spec.Scaling.WVA.VariantCost)
	require.NotNil(t, restored.Spec.Scaling.WVA.HPA)
	assert.Nil(t, restored.Spec.Scaling.WVA.KEDA)
	assert.Equal(t, int32(60), *restored.Spec.Scaling.WVA.HPA.Behavior.ScaleUp.StabilizationWindowSeconds)
	assert.Equal(t, int32(300), *restored.Spec.Scaling.WVA.HPA.Behavior.ScaleDown.StabilizationWindowSeconds)
}

func TestLLMInferenceServiceConversion_ScalingSpecWithKEDA(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-scaling-keda",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			WorkloadSpec: WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(3)),
					MaxReplicas: 20,
					WVA: &WVASpec{
						VariantCost: "5.5",
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								PollingInterval:       ptr.To(int32(15)),
								CooldownPeriod:        ptr.To(int32(120)),
								InitialCooldownPeriod: ptr.To(int32(60)),
								IdleReplicaCount:      ptr.To(int32(1)),
								Fallback: &kedav1alpha1.Fallback{
									FailureThreshold: 3,
									Replicas:         2,
								},
							},
						},
					},
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify the scaling spec is converted
	require.NotNil(t, dst.Spec.Scaling)
	assert.Equal(t, int32(3), *dst.Spec.Scaling.MinReplicas)
	assert.Equal(t, int32(20), dst.Spec.Scaling.MaxReplicas)
	require.NotNil(t, dst.Spec.Scaling.WVA)
	assert.Equal(t, "5.5", dst.Spec.Scaling.WVA.VariantCost)
	assert.Nil(t, dst.Spec.Scaling.WVA.HPA)
	require.NotNil(t, dst.Spec.Scaling.WVA.KEDA)
	assert.Equal(t, int32(15), *dst.Spec.Scaling.WVA.KEDA.PollingInterval)
	assert.Equal(t, int32(120), *dst.Spec.Scaling.WVA.KEDA.CooldownPeriod)
	assert.Equal(t, int32(60), *dst.Spec.Scaling.WVA.KEDA.InitialCooldownPeriod)
	assert.Equal(t, int32(1), *dst.Spec.Scaling.WVA.KEDA.IdleReplicaCount)
	require.NotNil(t, dst.Spec.Scaling.WVA.KEDA.Fallback)
	assert.Equal(t, int32(3), dst.Spec.Scaling.WVA.KEDA.Fallback.FailureThreshold)
	assert.Equal(t, int32(2), dst.Spec.Scaling.WVA.KEDA.Fallback.Replicas)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify round-trip
	require.NotNil(t, restored.Spec.Scaling)
	assert.Equal(t, int32(3), *restored.Spec.Scaling.MinReplicas)
	assert.Equal(t, int32(20), restored.Spec.Scaling.MaxReplicas)
	require.NotNil(t, restored.Spec.Scaling.WVA)
	assert.Equal(t, "5.5", restored.Spec.Scaling.WVA.VariantCost)
	assert.Nil(t, restored.Spec.Scaling.WVA.HPA)
	require.NotNil(t, restored.Spec.Scaling.WVA.KEDA)
	assert.Equal(t, int32(15), *restored.Spec.Scaling.WVA.KEDA.PollingInterval)
	assert.Equal(t, int32(120), *restored.Spec.Scaling.WVA.KEDA.CooldownPeriod)
	assert.Equal(t, int32(60), *restored.Spec.Scaling.WVA.KEDA.InitialCooldownPeriod)
	assert.Equal(t, int32(1), *restored.Spec.Scaling.WVA.KEDA.IdleReplicaCount)
	require.NotNil(t, restored.Spec.Scaling.WVA.KEDA.Fallback)
	assert.Equal(t, int32(3), restored.Spec.Scaling.WVA.KEDA.Fallback.FailureThreshold)
	assert.Equal(t, int32(2), restored.Spec.Scaling.WVA.KEDA.Fallback.Replicas)
}

func TestLLMInferenceServiceConversion_ScalingSpecWithDirectKEDA(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-scaling-direct-keda",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			WorkloadSpec: WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 10,
					KEDA: &DirectKEDAScalingSpec{
						KEDAScalingSpec: KEDAScalingSpec{
							PollingInterval:  ptr.To(int32(30)),
							CooldownPeriod:   ptr.To(int32(60)),
							IdleReplicaCount: ptr.To(int32(1)),
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
		},
	}

	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	require.NotNil(t, dst.Spec.Scaling)
	assert.Nil(t, dst.Spec.Scaling.WVA)
	require.NotNil(t, dst.Spec.Scaling.KEDA)
	assert.Equal(t, int32(30), *dst.Spec.Scaling.KEDA.PollingInterval)
	assert.Equal(t, int32(60), *dst.Spec.Scaling.KEDA.CooldownPeriod)
	assert.Equal(t, int32(1), *dst.Spec.Scaling.KEDA.IdleReplicaCount)
	require.NotNil(t, dst.Spec.Scaling.KEDA.Advanced)
	assert.Equal(t, "trig0 + trig1", dst.Spec.Scaling.KEDA.Advanced.ScalingModifiers.Formula)
	assert.Equal(t, "10", dst.Spec.Scaling.KEDA.Advanced.ScalingModifiers.Target)
	require.Len(t, dst.Spec.Scaling.KEDA.Triggers, 2)
	assert.Equal(t, "cpu", dst.Spec.Scaling.KEDA.Triggers[0].Type)
	assert.Equal(t, "memory", dst.Spec.Scaling.KEDA.Triggers[1].Type)

	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	require.NotNil(t, restored.Spec.Scaling)
	assert.Nil(t, restored.Spec.Scaling.WVA)
	require.NotNil(t, restored.Spec.Scaling.KEDA)
	assert.Equal(t, int32(30), *restored.Spec.Scaling.KEDA.PollingInterval)
	assert.Equal(t, int32(60), *restored.Spec.Scaling.KEDA.CooldownPeriod)
	assert.Equal(t, int32(1), *restored.Spec.Scaling.KEDA.IdleReplicaCount)
	require.NotNil(t, restored.Spec.Scaling.KEDA.Advanced)
	assert.Equal(t, "trig0 + trig1", restored.Spec.Scaling.KEDA.Advanced.ScalingModifiers.Formula)
	assert.Equal(t, "10", restored.Spec.Scaling.KEDA.Advanced.ScalingModifiers.Target)
	require.Len(t, restored.Spec.Scaling.KEDA.Triggers, 2)
	assert.Equal(t, "cpu", restored.Spec.Scaling.KEDA.Triggers[0].Type)
	assert.Equal(t, "memory", restored.Spec.Scaling.KEDA.Triggers[1].Type)
}

func TestLLMInferenceServiceConversion_NilScalingSpec(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-no-scaling",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			WorkloadSpec: WorkloadSpec{
				Replicas: ptr.To(int32(3)),
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify scaling is nil
	assert.Nil(t, dst.Spec.Scaling, "Scaling must remain nil when not configured")
	assert.Equal(t, int32(3), *dst.Spec.Replicas)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify round-trip
	assert.Nil(t, restored.Spec.Scaling, "Scaling must remain nil after round-trip")
	assert.Equal(t, int32(3), *restored.Spec.Replicas)
}

func TestLLMInferenceServiceConversion_ScalingOnPrefill(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-prefill-scaling",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			WorkloadSpec: WorkloadSpec{
				Replicas: ptr.To(int32(2)),
			},
			Prefill: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 8,
					WVA: &WVASpec{
						VariantCost: "10.0",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Main workload should have no scaling
	assert.Nil(t, dst.Spec.Scaling, "Decode workload scaling should be nil")
	assert.Equal(t, int32(2), *dst.Spec.Replicas)

	// Prefill should have scaling
	require.NotNil(t, dst.Spec.Prefill)
	require.NotNil(t, dst.Spec.Prefill.Scaling)
	assert.Equal(t, int32(1), *dst.Spec.Prefill.Scaling.MinReplicas)
	assert.Equal(t, int32(8), dst.Spec.Prefill.Scaling.MaxReplicas)
	require.NotNil(t, dst.Spec.Prefill.Scaling.WVA)
	assert.Equal(t, "10.0", dst.Spec.Prefill.Scaling.WVA.VariantCost)
	require.NotNil(t, dst.Spec.Prefill.Scaling.WVA.HPA)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify round-trip
	assert.Nil(t, restored.Spec.Scaling, "Decode workload scaling should remain nil")
	assert.Equal(t, int32(2), *restored.Spec.Replicas)
	require.NotNil(t, restored.Spec.Prefill)
	require.NotNil(t, restored.Spec.Prefill.Scaling)
	assert.Equal(t, int32(1), *restored.Spec.Prefill.Scaling.MinReplicas)
	assert.Equal(t, int32(8), restored.Spec.Prefill.Scaling.MaxReplicas)
	require.NotNil(t, restored.Spec.Prefill.Scaling.WVA)
	require.NotNil(t, restored.Spec.Prefill.Scaling.WVA.HPA)
}

func TestLLMInferenceServiceConversion_DecodeAndPrefillWithDifferentScaling(t *testing.T) {
	modelName := "test-model"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-both-scaling",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
			},
			WorkloadSpec: WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(2)),
					MaxReplicas: 10,
					WVA: &WVASpec{
						VariantCost: "10.0",
						ActuatorSpec: ActuatorSpec{
							HPA: &HPAScalingSpec{},
						},
					},
				},
			},
			Prefill: &WorkloadSpec{
				Scaling: &ScalingSpec{
					MinReplicas: ptr.To(int32(4)),
					MaxReplicas: 20,
					WVA: &WVASpec{
						VariantCost: "5.0",
						ActuatorSpec: ActuatorSpec{
							KEDA: &KEDAScalingSpec{
								PollingInterval:       ptr.To(int32(10)),
								CooldownPeriod:        ptr.To(int32(60)),
								InitialCooldownPeriod: ptr.To(int32(30)),
								IdleReplicaCount:      ptr.To(int32(2)),
								Fallback: &kedav1alpha1.Fallback{
									FailureThreshold: 5,
									Replicas:         3,
								},
							},
						},
					},
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify decode scaling
	require.NotNil(t, dst.Spec.Scaling)
	assert.Equal(t, int32(2), *dst.Spec.Scaling.MinReplicas)
	assert.Equal(t, int32(10), dst.Spec.Scaling.MaxReplicas)
	assert.Equal(t, "10.0", dst.Spec.Scaling.WVA.VariantCost)
	require.NotNil(t, dst.Spec.Scaling.WVA.HPA)
	assert.Nil(t, dst.Spec.Scaling.WVA.KEDA)

	// Verify prefill scaling
	require.NotNil(t, dst.Spec.Prefill)
	require.NotNil(t, dst.Spec.Prefill.Scaling)
	assert.Equal(t, int32(4), *dst.Spec.Prefill.Scaling.MinReplicas)
	assert.Equal(t, int32(20), dst.Spec.Prefill.Scaling.MaxReplicas)
	assert.Equal(t, "5.0", dst.Spec.Prefill.Scaling.WVA.VariantCost)
	assert.Nil(t, dst.Spec.Prefill.Scaling.WVA.HPA)
	require.NotNil(t, dst.Spec.Prefill.Scaling.WVA.KEDA)
	assert.Equal(t, int32(10), *dst.Spec.Prefill.Scaling.WVA.KEDA.PollingInterval)
	assert.Equal(t, int32(30), *dst.Spec.Prefill.Scaling.WVA.KEDA.InitialCooldownPeriod)
	assert.Equal(t, int32(2), *dst.Spec.Prefill.Scaling.WVA.KEDA.IdleReplicaCount)
	require.NotNil(t, dst.Spec.Prefill.Scaling.WVA.KEDA.Fallback)
	assert.Equal(t, int32(5), dst.Spec.Prefill.Scaling.WVA.KEDA.Fallback.FailureThreshold)
	assert.Equal(t, int32(3), dst.Spec.Prefill.Scaling.WVA.KEDA.Fallback.Replicas)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify decode round-trip
	require.NotNil(t, restored.Spec.Scaling)
	assert.Equal(t, int32(2), *restored.Spec.Scaling.MinReplicas)
	assert.Equal(t, int32(10), restored.Spec.Scaling.MaxReplicas)
	assert.Equal(t, "10.0", restored.Spec.Scaling.WVA.VariantCost)
	require.NotNil(t, restored.Spec.Scaling.WVA.HPA)
	assert.Nil(t, restored.Spec.Scaling.WVA.KEDA)

	// Verify prefill round-trip
	require.NotNil(t, restored.Spec.Prefill)
	require.NotNil(t, restored.Spec.Prefill.Scaling)
	assert.Equal(t, int32(4), *restored.Spec.Prefill.Scaling.MinReplicas)
	assert.Equal(t, int32(20), restored.Spec.Prefill.Scaling.MaxReplicas)
	assert.Equal(t, "5.0", restored.Spec.Prefill.Scaling.WVA.VariantCost)
	assert.Nil(t, restored.Spec.Prefill.Scaling.WVA.HPA)
	require.NotNil(t, restored.Spec.Prefill.Scaling.WVA.KEDA)
	assert.Equal(t, int32(10), *restored.Spec.Prefill.Scaling.WVA.KEDA.PollingInterval)
	assert.Equal(t, int32(60), *restored.Spec.Prefill.Scaling.WVA.KEDA.CooldownPeriod)
	assert.Equal(t, int32(30), *restored.Spec.Prefill.Scaling.WVA.KEDA.InitialCooldownPeriod)
	assert.Equal(t, int32(2), *restored.Spec.Prefill.Scaling.WVA.KEDA.IdleReplicaCount)
	require.NotNil(t, restored.Spec.Prefill.Scaling.WVA.KEDA.Fallback)
	assert.Equal(t, int32(5), restored.Spec.Prefill.Scaling.WVA.KEDA.Fallback.FailureThreshold)
	assert.Equal(t, int32(3), restored.Spec.Prefill.Scaling.WVA.KEDA.Fallback.Replicas)
}

func TestLLMInferenceServiceConversion_PreservesLoRASpecFields(t *testing.T) {
	modelName := "base-model"
	adapterName := "my-adapter"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-lora-fields",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
				LoRA: &LoRASpec{
					Adapters: []LLMModelSpec{
						{
							URI:  apis.URL{Scheme: "hf", Host: "my-org/my-adapter"},
							Name: &adapterName,
						},
					},
					MaxRank:        ptr.To(int32(128)),
					MaxAdapters:    ptr.To(int32(4)),
					MaxCpuAdapters: ptr.To(int32(8)),
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify LoRA fields in v1alpha2
	require.NotNil(t, dst.Spec.Model.LoRA)
	assert.Len(t, dst.Spec.Model.LoRA.Adapters, 1)
	require.NotNil(t, dst.Spec.Model.LoRA.MaxRank)
	assert.Equal(t, int32(128), *dst.Spec.Model.LoRA.MaxRank)
	require.NotNil(t, dst.Spec.Model.LoRA.MaxAdapters)
	assert.Equal(t, int32(4), *dst.Spec.Model.LoRA.MaxAdapters)
	require.NotNil(t, dst.Spec.Model.LoRA.MaxCpuAdapters)
	assert.Equal(t, int32(8), *dst.Spec.Model.LoRA.MaxCpuAdapters)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify LoRA fields round-trip correctly
	require.NotNil(t, restored.Spec.Model.LoRA)
	assert.Len(t, restored.Spec.Model.LoRA.Adapters, 1)
	assert.Equal(t, adapterName, *restored.Spec.Model.LoRA.Adapters[0].Name)
	require.NotNil(t, restored.Spec.Model.LoRA.MaxRank)
	assert.Equal(t, int32(128), *restored.Spec.Model.LoRA.MaxRank)
	require.NotNil(t, restored.Spec.Model.LoRA.MaxAdapters)
	assert.Equal(t, int32(4), *restored.Spec.Model.LoRA.MaxAdapters)
	require.NotNil(t, restored.Spec.Model.LoRA.MaxCpuAdapters)
	assert.Equal(t, int32(8), *restored.Spec.Model.LoRA.MaxCpuAdapters)
}

func TestLLMInferenceServiceConversion_StatusRoundtrip_V1Alpha1ToV1Alpha2(t *testing.T) {
	externalName := "gateway-external"
	internalName := "gateway-internal"
	externalURL, _ := apis.ParseURL("https://example.com/ns/m")
	internalURL, _ := apis.ParseURL("https://gw.ns.svc.cluster.local/ns/m")

	addressSingular := &duckv1.Addressable{URL: externalURL}

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       LLMInferenceServiceSpec{Model: LLMModelSpec{URI: *apis.HTTPS("example.com")}},
		Status: LLMInferenceServiceStatus{
			URL: apis.HTTPS("example.com"),
			AddressStatus: duckv1.AddressStatus{
				Address: addressSingular,
				Addresses: []duckv1.Addressable{
					{Name: &externalName, URL: externalURL},
					{Name: &internalName, URL: internalURL},
				},
			},
		},
	}

	// v1alpha1 -> v1alpha2
	hub := &v1alpha2.LLMInferenceService{}
	require.NoError(t, src.ConvertTo(hub))

	assert.Equal(t, src.Status.URL.String(), hub.Status.URL.String())
	require.NotNil(t, hub.Status.Address, "Address (singular) should be preserved in ConvertTo") //nolint:staticcheck // testing deprecated field
	assert.Equal(t, externalURL.String(), hub.Status.Address.URL.String())                       //nolint:staticcheck // testing deprecated field
	require.Len(t, hub.Status.Addresses, 2)
	assert.Equal(t, "gateway-external", *hub.Status.Addresses[0].Name)
	assert.Equal(t, "https://example.com/ns/m", hub.Status.Addresses[0].URL.String())
	assert.Nil(t, hub.Status.Addresses[0].Origin, "Origin should be nil when coming from v1alpha1")
	assert.Nil(t, hub.Status.Addresses[1].Origin)

	// v1alpha2 -> v1alpha1 (roundtrip)
	restored := &LLMInferenceService{}
	require.NoError(t, restored.ConvertFrom(hub))

	assert.Equal(t, src.Status.URL.String(), restored.Status.URL.String())
	require.NotNil(t, restored.Status.Address, "Address (singular) should survive roundtrip")
	assert.Equal(t, externalURL.String(), restored.Status.Address.URL.String())
	require.Len(t, restored.Status.Addresses, 2)
	assert.Equal(t, "gateway-external", *restored.Status.AddressStatus.Addresses[0].Name)
	assert.Equal(t, src.Status.Addresses[0].URL.String(), restored.Status.Addresses[0].URL.String())
}

func TestLLMInferenceServiceConversion_NilLoRASpecFields(t *testing.T) {
	modelName := "base-model"
	adapterName := "my-adapter"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-lora-nil-fields",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-2-7b"},
				Name: &modelName,
				LoRA: &LoRASpec{
					Adapters: []LLMModelSpec{
						{
							URI:  apis.URL{Scheme: "hf", Host: "my-org/my-adapter"},
							Name: &adapterName,
						},
					},
					// MaxRank, MaxAdapters, MaxCpuAdapters all nil (defaults)
				},
			},
		},
	}

	// Convert to v1alpha2 (hub)
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	// Verify nil fields remain nil in v1alpha2
	require.NotNil(t, dst.Spec.Model.LoRA)
	assert.Nil(t, dst.Spec.Model.LoRA.MaxRank)
	assert.Nil(t, dst.Spec.Model.LoRA.MaxAdapters)
	assert.Nil(t, dst.Spec.Model.LoRA.MaxCpuAdapters)

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify nil fields remain nil after round-trip
	require.NotNil(t, restored.Spec.Model.LoRA)
	assert.Nil(t, restored.Spec.Model.LoRA.MaxRank)
	assert.Nil(t, restored.Spec.Model.LoRA.MaxAdapters)
	assert.Nil(t, restored.Spec.Model.LoRA.MaxCpuAdapters)
}

func TestLLMInferenceServiceConversion_StatusRoundtrip_V1Alpha2ToV1Alpha1(t *testing.T) {
	externalName := "gateway-external"
	externalURL, _ := apis.ParseURL("https://example.com/ns/m")

	addressSingular := &duckv1.Addressable{URL: externalURL}

	hub := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       v1alpha2.LLMInferenceServiceSpec{Model: v1alpha2.LLMModelSpec{URI: *apis.HTTPS("example.com")}},
		Status: v1alpha2.LLMInferenceServiceStatus{
			URL:     apis.HTTPS("example.com"),
			Address: addressSingular,
			Addresses: []v1alpha2.SourcedAddress{
				{
					Addressable: duckv1.Addressable{
						Name: &externalName,
						URL:  externalURL,
					},
					Origin: &gwapiv1.ObjectReference{
						Group:     "gateway.networking.k8s.io",
						Kind:      "Gateway",
						Name:      "my-gateway",
						Namespace: ptr.To(gwapiv1.Namespace("istio-system")),
					},
				},
			},
		},
	}

	// v1alpha2 -> v1alpha1
	spoke := &LLMInferenceService{}
	require.NoError(t, spoke.ConvertFrom(hub))

	assert.Equal(t, hub.Status.URL.String(), spoke.Status.URL.String())
	require.NotNil(t, spoke.Status.Address, "Address (singular) should be preserved in ConvertFrom")
	assert.Equal(t, externalURL.String(), spoke.Status.Address.URL.String())
	require.Len(t, spoke.Status.Addresses, 1)
	assert.Equal(t, "gateway-external", *spoke.Status.AddressStatus.Addresses[0].Name)
	assert.Equal(t, "https://example.com/ns/m", spoke.Status.Addresses[0].URL.String())

	// v1alpha1 -> v1alpha2 (roundtrip) - Origin is lost
	restored := &v1alpha2.LLMInferenceService{}
	require.NoError(t, spoke.ConvertTo(restored))

	assert.Equal(t, hub.Status.URL.String(), restored.Status.URL.String())
	require.NotNil(t, restored.Status.Address, "Address (singular) should survive roundtrip") //nolint:staticcheck // testing deprecated field
	assert.Equal(t, externalURL.String(), restored.Status.Address.URL.String())               //nolint:staticcheck // testing deprecated field
	require.Len(t, restored.Status.Addresses, 1)
	assert.Equal(t, "gateway-external", *restored.Status.Addresses[0].Name)
	assert.Equal(t, "https://example.com/ns/m", restored.Status.Addresses[0].URL.String())
	assert.Nil(t, restored.Status.Addresses[0].Origin, "Origin is lost on v1alpha2 -> v1alpha1 -> v1alpha2 roundtrip")
}

func TestLLMInferenceServiceConversion_PreservesTracing(t *testing.T) {
	endpoint := "http://my-collector:4317"
	sampler := "parentbased_traceidratio"
	samplerArg := "0.1"
	exporter := "otlp"

	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-tracing",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI: apis.URL{Scheme: "hf", Host: "Qwen/Qwen2.5-7B-Instruct"},
			},
			Tracing: &TracingSpec{
				ExporterEndpoint: &endpoint,
				Sampler:          &sampler,
				SamplerArg:       &samplerArg,
				Exporter:         &exporter,
			},
		},
	}

	// v1alpha1 -> v1alpha2
	hub := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(hub)
	require.NoError(t, err)

	require.NotNil(t, hub.Spec.Tracing)
	assert.Equal(t, endpoint, *hub.Spec.Tracing.ExporterEndpoint)
	assert.Equal(t, sampler, *hub.Spec.Tracing.Sampler)
	assert.Equal(t, samplerArg, *hub.Spec.Tracing.SamplerArg)
	assert.Equal(t, exporter, *hub.Spec.Tracing.Exporter)

	// v1alpha2 -> v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(hub)
	require.NoError(t, err)

	require.NotNil(t, restored.Spec.Tracing)
	assert.Equal(t, endpoint, *restored.Spec.Tracing.ExporterEndpoint)
	assert.Equal(t, sampler, *restored.Spec.Tracing.Sampler)
	assert.Equal(t, samplerArg, *restored.Spec.Tracing.SamplerArg)
	assert.Equal(t, exporter, *restored.Spec.Tracing.Exporter)
}

func TestLLMInferenceServiceConversion_TracingNilPreserved(t *testing.T) {
	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-no-tracing",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI: apis.URL{Scheme: "hf", Host: "Qwen/Qwen2.5-7B-Instruct"},
			},
		},
	}

	// v1alpha1 -> v1alpha2
	hub := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(hub)
	require.NoError(t, err)
	assert.Nil(t, hub.Spec.Tracing)

	// v1alpha2 -> v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(hub)
	require.NoError(t, err)
	assert.Nil(t, restored.Spec.Tracing)
}

func TestLLMInferenceServiceConversion_TracingEmptyStruct(t *testing.T) {
	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-tracing-defaults",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI: apis.URL{Scheme: "hf", Host: "Qwen/Qwen2.5-7B-Instruct"},
			},
			Tracing: &TracingSpec{},
		},
	}

	// v1alpha1 -> v1alpha2
	hub := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(hub)
	require.NoError(t, err)
	require.NotNil(t, hub.Spec.Tracing, "empty tracing struct should be preserved")

	// v1alpha2 -> v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(hub)
	require.NoError(t, err)
	require.NotNil(t, restored.Spec.Tracing, "empty tracing struct should survive roundtrip")
}

func TestLLMInferenceServiceConversion_PreservesTokenizer(t *testing.T) {
	tests := []struct {
		name      string
		tokenizer *TokenizerSpec
	}{
		{
			name:      "empty tokenizer spec",
			tokenizer: &TokenizerSpec{},
		},
		{
			name: "tokenizer with custom template",
			tokenizer: &TokenizerSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "custom-tokenizer:v1",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-isvc-tokenizer",
					Namespace: "default",
				},
				Spec: LLMInferenceServiceSpec{
					Model: LLMModelSpec{
						URI: apis.URL{Scheme: "hf", Host: "facebook/opt-125m"},
					},
					Router: &RouterSpec{
						Scheduler: &SchedulerSpec{
							Tokenizer: tt.tokenizer,
						},
					},
				},
			}

			// v1alpha1 -> v1alpha2
			dst := &v1alpha2.LLMInferenceService{}
			err := src.ConvertTo(dst)
			require.NoError(t, err)

			require.NotNil(t, dst.Spec.Router)
			require.NotNil(t, dst.Spec.Router.Scheduler)
			require.NotNil(t, dst.Spec.Router.Scheduler.Tokenizer,
				"Scheduler.Tokenizer must not be lost during conversion to v1alpha2")

			if tt.tokenizer.Template != nil {
				require.NotNil(t, dst.Spec.Router.Scheduler.Tokenizer.Template)
				assert.Equal(t, tt.tokenizer.Template.Containers[0].Image,
					dst.Spec.Router.Scheduler.Tokenizer.Template.Containers[0].Image)
			}

			// v1alpha2 -> v1alpha1
			restored := &LLMInferenceService{}
			err = restored.ConvertFrom(dst)
			require.NoError(t, err)

			require.NotNil(t, restored.Spec.Router)
			require.NotNil(t, restored.Spec.Router.Scheduler)
			require.NotNil(t, restored.Spec.Router.Scheduler.Tokenizer,
				"Scheduler.Tokenizer must not be lost during round-trip")

			if tt.tokenizer.Template != nil {
				require.NotNil(t, restored.Spec.Router.Scheduler.Tokenizer.Template)
				assert.Equal(t, tt.tokenizer.Template.Containers[0].Image,
					restored.Spec.Router.Scheduler.Tokenizer.Template.Containers[0].Image)
			}
		})
	}
}

func TestLLMInferenceServiceConversion_NilTokenizerPreserved(t *testing.T) {
	src := &LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-isvc-no-tokenizer",
			Namespace: "default",
		},
		Spec: LLMInferenceServiceSpec{
			Model: LLMModelSpec{
				URI: apis.URL{Scheme: "hf", Host: "facebook/opt-125m"},
			},
			Router: &RouterSpec{
				Scheduler: &SchedulerSpec{},
			},
		},
	}

	// v1alpha1 -> v1alpha2
	dst := &v1alpha2.LLMInferenceService{}
	err := src.ConvertTo(dst)
	require.NoError(t, err)

	require.NotNil(t, dst.Spec.Router)
	require.NotNil(t, dst.Spec.Router.Scheduler)
	assert.Nil(t, dst.Spec.Router.Scheduler.Tokenizer,
		"nil Tokenizer should remain nil after conversion")

	// v1alpha2 -> v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	require.NotNil(t, restored.Spec.Router)
	require.NotNil(t, restored.Spec.Router.Scheduler)
	assert.Nil(t, restored.Spec.Router.Scheduler.Tokenizer,
		"nil Tokenizer should remain nil after round-trip")
}
