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
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestInjectSpeculativeDecodingArgs_Eagle3WithSpeculator(t *testing.T) {
	specDecoding := &v1alpha2.SpeculativeDecodingSpec{
		Method:               "eagle3",
		NumSpeculativeTokens: 3,
		Speculator: &v1alpha2.SpeculatorSpec{
			TensorParallelSize: ptr.To(int32(1)),
			MaxModelLen:        ptr.To(int32(4096)),
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err := injectSpeculativeDecodingArgs(specDecoding, podSpec, "main")
	require.NoError(t, err)

	// Find VLLM_ADDITIONAL_ARGS
	var vllmArgs string
	for _, env := range podSpec.Containers[0].Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			vllmArgs = env.Value
			break
		}
	}
	require.NotEmpty(t, vllmArgs, "VLLM_ADDITIONAL_ARGS should be set")

	// Parse out the JSON from --speculative-config '...'
	assert.Contains(t, vllmArgs, "--speculative-config")
	jsonStr := extractSpecConfigJSON(t, vllmArgs)

	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	assert.Equal(t, "eagle3", specConfig["method"])
	assert.Equal(t, float64(3), specConfig["num_speculative_tokens"])
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, specConfig["model"])
	assert.Equal(t, float64(1), specConfig["draft_tensor_parallel_size"])
	assert.Equal(t, float64(4096), specConfig["max_model_len"])
}

func TestInjectSpeculativeDecodingArgs_NgramNoSpeculator(t *testing.T) {
	specDecoding := &v1alpha2.SpeculativeDecodingSpec{
		Method:               "ngram",
		NumSpeculativeTokens: 5,
		AdditionalConfig: map[string]string{
			"prompt_lookup_max": "4",
			"prompt_lookup_min": "1",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err := injectSpeculativeDecodingArgs(specDecoding, podSpec, "main")
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
	assert.Equal(t, float64(5), specConfig["num_speculative_tokens"])
	assert.Equal(t, "4", specConfig["prompt_lookup_max"])
	assert.Equal(t, "1", specConfig["prompt_lookup_min"])
	// No model or draft_tensor_parallel_size for ngram
	_, hasModel := specConfig["model"]
	assert.False(t, hasModel, "ngram should not have a model field")
}

func TestInjectSpeculativeDecodingArgs_AdditionalConfigOverriddenByFirstClass(t *testing.T) {
	specDecoding := &v1alpha2.SpeculativeDecodingSpec{
		Method:               "eagle3",
		NumSpeculativeTokens: 3,
		Speculator: &v1alpha2.SpeculatorSpec{
			TensorParallelSize: ptr.To(int32(2)),
		},
		AdditionalConfig: map[string]string{
			// These should be overridden by first-class fields
			"method":               "draft_model",
			"num_speculative_tokens": "10",
			// This should survive since it's not a first-class field
			"enforce_eager": "true",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
		},
	}

	err := injectSpeculativeDecodingArgs(specDecoding, podSpec, "main")
	require.NoError(t, err)

	jsonStr := extractSpecConfigJSON(t, podSpec.Containers[0].Env[0].Value)

	var specConfig map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &specConfig)
	require.NoError(t, err)

	// First-class fields take precedence
	assert.Equal(t, "eagle3", specConfig["method"])
	assert.Equal(t, float64(3), specConfig["num_speculative_tokens"])
	assert.Equal(t, float64(2), specConfig["draft_tensor_parallel_size"])
	// additionalConfig passthrough
	assert.Equal(t, "true", specConfig["enforce_eager"])
}

func TestInjectSpeculativeDecodingArgs_AppendsToExistingVLLMArgs(t *testing.T) {
	specDecoding := &v1alpha2.SpeculativeDecodingSpec{
		Method:               "eagle3",
		NumSpeculativeTokens: 3,
		Speculator:           &v1alpha2.SpeculatorSpec{},
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

	err := injectSpeculativeDecodingArgs(specDecoding, podSpec, "main")
	require.NoError(t, err)

	vllmArgs := podSpec.Containers[0].Env[0].Value
	assert.Contains(t, vllmArgs, "--existing-flag")
	assert.Contains(t, vllmArgs, "--speculative-config")
}

// extractSpecConfigJSON extracts the JSON string from a --speculative-config '...' argument.
func extractSpecConfigJSON(t *testing.T, args string) string {
	t.Helper()
	// Find the start of the JSON after --speculative-config '
	prefix := "--speculative-config '"
	start := 0
	for i := 0; i <= len(args)-len(prefix); i++ {
		if args[i:i+len(prefix)] == prefix {
			start = i + len(prefix)
			break
		}
	}
	require.NotZero(t, start, "could not find --speculative-config in args: %s", args)

	// Find the closing quote
	end := start
	for end < len(args) && args[end] != '\'' {
		end++
	}
	require.Less(t, end, len(args), "could not find closing quote in args: %s", args)

	return args[start:end]
}
