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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	igwapiv1 "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"

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
