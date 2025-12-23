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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

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
	assert.NotNil(t, dst.ObjectMeta.Annotations)
	assert.Equal(t, string(criticality), dst.ObjectMeta.Annotations[ModelCriticalityAnnotationKey])

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify the criticality is restored
	assert.NotNil(t, restored.Spec.Model.Criticality)
	assert.Equal(t, criticality, *restored.Spec.Model.Criticality)

	// Verify the annotation is cleaned up
	_, hasAnnotation := restored.ObjectMeta.Annotations[ModelCriticalityAnnotationKey]
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
	assert.NotNil(t, dst.ObjectMeta.Annotations)
	assert.Equal(t, string(modelCriticality), dst.ObjectMeta.Annotations[ModelCriticalityAnnotationKey])
	assert.Contains(t, dst.ObjectMeta.Annotations, LoRACriticalitiesAnnotationKey)

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
	_, hasModelAnnotation := restored.ObjectMeta.Annotations[ModelCriticalityAnnotationKey]
	assert.False(t, hasModelAnnotation, "Model criticality annotation should be cleaned up")
	_, hasLoRAAnnotation := restored.ObjectMeta.Annotations[LoRACriticalitiesAnnotationKey]
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
	if dst.ObjectMeta.Annotations != nil {
		_, hasAnnotation := dst.ObjectMeta.Annotations[ModelCriticalityAnnotationKey]
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
	assert.NotNil(t, dst.ObjectMeta.Annotations)
	assert.Equal(t, string(criticality), dst.ObjectMeta.Annotations[ModelCriticalityAnnotationKey])

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
	assert.Equal(t, existingAnnotation, dst.ObjectMeta.Annotations["existing-annotation"])
	assert.Equal(t, string(criticality), dst.ObjectMeta.Annotations[ModelCriticalityAnnotationKey])

	// Convert back to v1alpha1
	restored := &LLMInferenceService{}
	err = restored.ConvertFrom(dst)
	require.NoError(t, err)

	// Verify the criticality is restored
	assert.NotNil(t, restored.Spec.Model.Criticality)
	assert.Equal(t, criticality, *restored.Spec.Model.Criticality)

	// Verify existing annotation is preserved
	assert.Equal(t, existingAnnotation, restored.ObjectMeta.Annotations["existing-annotation"])

	// Verify criticality annotation is cleaned up
	_, hasAnnotation := restored.ObjectMeta.Annotations[ModelCriticalityAnnotationKey]
	assert.False(t, hasAnnotation, "Criticality annotation should be cleaned up")
}
