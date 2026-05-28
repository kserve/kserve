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

package llmisvc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestInjectSpeculativeDecodingArgs_Eagle3WithSpeculator(t *testing.T) {
	speculatorURI, err := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
	require.NoError(t, err)

	speculator := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMModelSpec{
			URI: *speculatorURI,
		},
		Config: map[string]string{
			"method":                     "eagle3",
			"num_speculative_tokens":     "6",
			"draft_tensor_parallel_size": "1",
			"max_model_len":              "4096",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err = injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
			break
		}
	}
	require.NotEmpty(t, vllmArgs, "VLLM_ADDITIONAL_ARGS should be set")

	assert.Contains(t, vllmArgs, "--speculative-config")
	jsonStr := extractSpecConfigJSON(t, vllmArgs)

	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	assert.Equal(t, "eagle3", specConfig["method"])
	assert.Equal(t, "6", specConfig["num_speculative_tokens"])
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, specConfig["model"])
	assert.Equal(t, "1", specConfig["draft_tensor_parallel_size"])
	assert.Equal(t, "4096", specConfig["max_model_len"])
}

func TestInjectSpeculativeDecodingArgs_NgramNoSpeculator(t *testing.T) {
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "ngram",
			"num_speculative_tokens": "4",
			"prompt_lookup_max":      "5",
			"prompt_lookup_min":      "2",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
			break
		}
	}
	require.NotEmpty(t, vllmArgs)

	jsonStr := extractSpecConfigJSON(t, vllmArgs)

	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	assert.Equal(t, "ngram", specConfig["method"])
	assert.Equal(t, "4", specConfig["num_speculative_tokens"])
	assert.Equal(t, "5", specConfig["prompt_lookup_max"])
	assert.Equal(t, "2", specConfig["prompt_lookup_min"])
	_, hasModel := specConfig["model"]
	assert.False(t, hasModel, "ngram should not have a model field")
}

func TestInjectSpeculativeDecodingArgs_ConfigIsAuthoritative(t *testing.T) {
	speculatorURI, err := apis.ParseURL("hf://meta-llama/Llama-3.2-1B-Instruct")
	require.NoError(t, err)

	speculator := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMModelSpec{
			URI: *speculatorURI,
		},
		Config: map[string]string{
			"method":                     "draft_model",
			"num_speculative_tokens":     "10",
			"draft_tensor_parallel_size": "2",
			"enforce_eager":              "true",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err = injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	jsonStr := extractSpecConfigJSON(t, podSpec.Containers[0].Env[0].Value)

	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	assert.Equal(t, "draft_model", specConfig["method"])
	assert.Equal(t, "10", specConfig["num_speculative_tokens"])
	assert.Equal(t, "2", specConfig["draft_tensor_parallel_size"])
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, specConfig["model"])
	assert.Equal(t, "true", specConfig["enforce_eager"])
}

func TestInjectSpeculativeDecodingArgs_AppendsToExistingVLLMArgs(t *testing.T) {
	speculatorURI, err := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
	require.NoError(t, err)

	speculator := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMModelSpec{
			URI: *speculatorURI,
		},
		Config: map[string]string{
			"method":                 "eagle3",
			"num_speculative_tokens": "3",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name: "main",
				Env: []corev1.EnvVar{
					{Name: "VLLM_ADDITIONAL_ARGS", Value: "--existing-flag"},
				},
			},
		},
	}

	err = injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	vllmArgs := podSpec.Containers[0].Env[0].Value
	assert.Contains(t, vllmArgs, "--existing-flag")
	assert.Contains(t, vllmArgs, "--speculative-config")
}

func TestInjectSpeculativeDecodingArgs_MTP(t *testing.T) {
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "mtp",
			"num_speculative_tokens": "3",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	jsonStr := extractSpecConfigJSON(t, podSpec.Containers[0].Env[0].Value)

	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	assert.Equal(t, "mtp", specConfig["method"])
	assert.Equal(t, "3", specConfig["num_speculative_tokens"])
	_, hasModel := specConfig["model"]
	assert.False(t, hasModel, "mtp should not have a model field")
}

func TestInjectSpeculativeDecodingArgs_Medusa(t *testing.T) {
	speculatorURI, err := apis.ParseURL("hf://FasterDecoding/vllm-medusa-vicuna-7b-v1.3")
	require.NoError(t, err)

	speculator := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMModelSpec{
			URI: *speculatorURI,
		},
		Config: map[string]string{
			"method":                     "medusa",
			"num_speculative_tokens":     "5",
			"draft_tensor_parallel_size": "1",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err = injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	jsonStr := extractSpecConfigJSON(t, podSpec.Containers[0].Env[0].Value)

	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	assert.Equal(t, "medusa", specConfig["method"])
	assert.Equal(t, "5", specConfig["num_speculative_tokens"])
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, specConfig["model"])
	assert.Equal(t, "1", specConfig["draft_tensor_parallel_size"])
}

func TestInjectSpeculativeDecodingArgs_MTPRuntimeVariant(t *testing.T) {
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "qwen3_next_mtp",
			"num_speculative_tokens": "2",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	jsonStr := extractSpecConfigJSON(t, podSpec.Containers[0].Env[0].Value)

	var specConfig map[string]any
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	assert.Equal(t, "qwen3_next_mtp", specConfig["method"])
	assert.Equal(t, "2", specConfig["num_speculative_tokens"])
}

// extractSpecConfigJSON extracts the JSON string from a --speculative-config '...' argument.
func extractSpecConfigJSON(t *testing.T, args string) string {
	t.Helper()
	prefix := "--speculative-config '"
	start := 0
	for i := 0; i <= len(args)-len(prefix); i++ {
		if args[i:i+len(prefix)] == prefix {
			start = i + len(prefix)
			break
		}
	}
	require.NotZero(t, start, "could not find --speculative-config in args: %s", args)

	end := start
	for end < len(args) && args[end] != '\'' {
		end++
	}
	require.Less(t, end, len(args), "could not find closing quote in args: %s", args)

	return args[start:end]
}
