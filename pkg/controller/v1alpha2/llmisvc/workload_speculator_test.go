/*
Copyright 2026 The KServe Authors.

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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	kserveTypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

func TestInjectSpeculativeDecodingArgs_Eagle3(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
	speculator := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
		Config: map[string]string{
			"method":                     "eagle3",
			"num_speculative_tokens":     "3",
			"draft_tensor_parallel_size": "1",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	mainContainer := getContainerByName(podSpec, "main")
	require.NotNil(t, mainContainer)

	vllmArgs := getVLLMAdditionalArgs(mainContainer)
	assert.Contains(t, vllmArgs, "--speculative-config")
	assert.Contains(t, vllmArgs, constants.DefaultSpeculatorLocalMountPath)

	config := extractSpecConfigJSON(t, vllmArgs)
	assert.Equal(t, "eagle3", config["method"])
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, config["model"])
}

func TestInjectSpeculativeDecodingArgs_NgramNoModel(t *testing.T) {
	t.Parallel()
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "ngram",
			"num_speculative_tokens": "4",
			"prompt_lookup_max":      "5",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	config := extractSpecConfigJSON(t, vllmArgs)

	assert.Equal(t, "ngram", config["method"])
	_, hasModel := config["model"]
	assert.False(t, hasModel, "ngram method should not have a 'model' key")
}

func TestInjectSpeculativeDecodingArgs_AppendToExisting(t *testing.T) {
	t.Parallel()
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "ngram",
			"num_speculative_tokens": "4",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "main",
			Env: []corev1.EnvVar{
				{Name: "VLLM_ADDITIONAL_ARGS", Value: "--existing-flag value"},
			},
		}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	assert.True(t, strings.HasPrefix(vllmArgs, "--existing-flag value "), "existing args should be preserved")
	assert.Contains(t, vllmArgs, "--speculative-config")
}

func TestInjectSpeculativeDecodingArgs_MTPMethodRefinement(t *testing.T) {
	t.Parallel()
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{
			"method":                 "qwen3_next_mtp",
			"num_speculative_tokens": "1",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	config := extractSpecConfigJSON(t, vllmArgs)
	assert.Equal(t, "qwen3_next_mtp", config["method"])
}

func TestInjectSpeculativeDecodingArgs_ContainerNotFound(t *testing.T) {
	t.Parallel()
	speculator := &v1alpha2.SpeculatorSpec{
		Config: map[string]string{"method": "ngram"},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "other"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestInjectSpeculativeDecodingArgs_DraftModel(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("hf://meta-llama/Llama-3.2-1B-Instruct")
	speculator := &v1alpha2.SpeculatorSpec{
		Model: &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
		Config: map[string]string{
			"method":                     "draft_model",
			"num_speculative_tokens":     "5",
			"max_model_len":              "8192",
			"draft_tensor_parallel_size": "1",
		},
	}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	config := extractSpecConfigJSON(t, vllmArgs)

	assert.Equal(t, "draft_model", config["method"])
	assert.Equal(t, constants.DefaultSpeculatorLocalMountPath, config["model"])
	assert.Equal(t, float64(5), config["num_speculative_tokens"])
}

func TestInjectSpeculativeDecodingArgs_EmptySpeculator(t *testing.T) {
	t.Parallel()
	speculator := &v1alpha2.SpeculatorSpec{}

	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := injectSpeculativeDecodingArgs(speculator, podSpec, "main")
	require.NoError(t, err)

	mainContainer := getContainerByName(podSpec, "main")
	vllmArgs := getVLLMAdditionalArgs(mainContainer)
	assert.Empty(t, vllmArgs, "VLLM_ADDITIONAL_ARGS should not be set for empty speculator")
}

func TestAppendToVLLMAdditionalArgs_NewEnvVar(t *testing.T) {
	t.Parallel()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	err := appendToVLLMAdditionalArgs(podSpec, "main", "--speculative-config '{}'")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	assert.Equal(t, "--speculative-config '{}'", vllmArgs)
}

func TestAppendToVLLMAdditionalArgs_ExistingEnvVar(t *testing.T) {
	t.Parallel()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "main",
			Env: []corev1.EnvVar{
				{Name: "VLLM_ADDITIONAL_ARGS", Value: "--existing-arg"},
			},
		}},
	}

	err := appendToVLLMAdditionalArgs(podSpec, "main", "--speculative-config '{}'")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	assert.Equal(t, "--existing-arg --speculative-config '{}'", vllmArgs)
}

func TestAppendToVLLMAdditionalArgs_ReplacesExistingSpeculativeConfig(t *testing.T) {
	t.Parallel()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "main",
			Env: []corev1.EnvVar{
				{Name: "VLLM_ADDITIONAL_ARGS", Value: "--speculative-config '{\"method\":\"ngram\"}'"},
			},
		}},
	}

	err := appendToVLLMAdditionalArgs(podSpec, "main", "--speculative-config '{\"method\":\"eagle3\"}'")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	assert.NotContains(t, vllmArgs, "ngram", "old --speculative-config should have been stripped")
	assert.Contains(t, vllmArgs, "eagle3", "CR-defined --speculative-config should replace the old one")
}

func TestAppendToVLLMAdditionalArgs_ReplacesAndPreservesOtherFlags(t *testing.T) {
	t.Parallel()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "main",
			Env: []corev1.EnvVar{
				{Name: "VLLM_ADDITIONAL_ARGS", Value: "--max-num-seqs 128 --speculative-config '{\"method\":\"ngram\"}' --enable-chunked-prefill"},
			},
		}},
	}

	err := appendToVLLMAdditionalArgs(podSpec, "main", "--speculative-config '{\"method\":\"eagle3\"}'")
	require.NoError(t, err)

	vllmArgs := getVLLMAdditionalArgs(getContainerByName(podSpec, "main"))
	assert.NotContains(t, vllmArgs, "ngram")
	assert.Contains(t, vllmArgs, "eagle3")
	assert.Contains(t, vllmArgs, "--max-num-seqs 128")
	assert.Contains(t, vllmArgs, "--enable-chunked-prefill")
}

func TestAppendToVLLMAdditionalArgs_ErrorsOnValueFrom(t *testing.T) {
	t.Parallel()
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "main",
			Env: []corev1.EnvVar{
				{
					Name: "VLLM_ADDITIONAL_ARGS",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"},
							Key:                  "vllm-args",
						},
					},
				},
			},
		}},
	}

	err := appendToVLLMAdditionalArgs(podSpec, "main", "--speculative-config '{}'")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "valueFrom")
}

func TestStripSpeculativeConfigFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no flag", "--max-num-seqs 128", "--max-num-seqs 128"},
		{"only flag", "--speculative-config '{\"method\":\"ngram\"}'", ""},
		{"flag at start", "--speculative-config '{\"method\":\"ngram\"}' --other-flag", "--other-flag"},
		{"flag at end", "--other-flag --speculative-config '{\"method\":\"ngram\"}'", "--other-flag"},
		{"flag in middle", "--before --speculative-config '{\"method\":\"ngram\"}' --after", "--before --after"},
		{"double-quoted only", `--speculative-config "{\"method\":\"ngram\"}"`, ""},
		{"double-quoted at start", `--speculative-config "{\"method\":\"ngram\"}" --other-flag`, "--other-flag"},
		{"double-quoted in middle", `--before --speculative-config "{\"method\":\"ngram\"}" --after`, "--before --after"},
		{"malformed no closing quote", "--speculative-config '{\"method\":\"ngram\"}", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stripSpeculativeConfigFlag(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestStripPriorSpeculatorInitializer(t *testing.T) {
	t.Parallel()
	podSpec := &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{Name: "storage-initializer"},
			{Name: constants.SpeculatorInitializerContainerName},
			{Name: "other-init"},
		},
	}

	stripPriorSpeculatorInitializer(podSpec)

	require.Len(t, podSpec.InitContainers, 2)
	for _, ic := range podSpec.InitContainers {
		assert.NotEqual(t, constants.SpeculatorInitializerContainerName, ic.Name)
	}
}

func TestStripPriorSpeculatorInitializer_NilPodSpec(t *testing.T) {
	t.Parallel()
	stripPriorSpeculatorInitializer(nil)
}

func TestStripPriorSpeculatorInitializer_NoMatch(t *testing.T) {
	t.Parallel()
	podSpec := &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{Name: "storage-initializer"},
			{Name: "other-init"},
		},
	}

	stripPriorSpeculatorInitializer(podSpec)
	require.Len(t, podSpec.InitContainers, 2)
}

func TestAttachSpeculatorModelArtifacts_PvcURI(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("pvc://my-pvc/path/to/speculator")
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
				Config: map[string]string{"method": "eagle3", "num_speculative_tokens": "3"},
			},
		},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, &Config{}, "main")
	require.NoError(t, err)

	mainContainer := getContainerByName(podSpec, "main")
	hasMount := false
	for _, vm := range mainContainer.VolumeMounts {
		if vm.MountPath == constants.DefaultSpeculatorLocalMountPath {
			hasMount = true
			assert.Equal(t, constants.SpeculatorVolumeName, vm.Name,
				"speculator PVC mount should use dedicated volume name, not the main model PVC volume")
		}
	}
	assert.True(t, hasMount, "expected volume mount at %s", constants.DefaultSpeculatorLocalMountPath)

	hasVolume := false
	for _, v := range podSpec.Volumes {
		if v.Name == constants.SpeculatorVolumeName {
			hasVolume = true
			require.NotNil(t, v.PersistentVolumeClaim, "expected PVC volume source")
			assert.Equal(t, "my-pvc", v.PersistentVolumeClaim.ClaimName)
		}
	}
	assert.True(t, hasVolume, "expected volume %s for speculator PVC", constants.SpeculatorVolumeName)

	vllmArgs := getVLLMAdditionalArgs(mainContainer)
	assert.Contains(t, vllmArgs, "--speculative-config")
}

func TestAttachSpeculatorModelArtifacts_InvalidURI(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("no-scheme-here")
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
				Config: map[string]string{"method": "eagle3"},
			},
		},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, &Config{}, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid speculator model URI")
}

func TestAttachSpeculatorModelArtifacts_UnsupportedScheme(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("ftp://example.com/model")
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
				Config: map[string]string{"method": "eagle3"},
			},
		},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, &Config{}, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported scheme")
}

func TestAttachSpeculatorModelArtifacts_OciEnabled(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("oci://registry.example.com/speculator:v1")
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
				Config: map[string]string{"method": "eagle3"},
			},
		},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	config := &Config{
		StorageConfig: &kserveTypes.StorageInitializerConfig{
			EnableOciImageSource: true,
		},
	}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, config, "main")
	require.NoError(t, err)

	_, _, expectedVolumeName := utils.ModelcarNames(1)
	hasVolume := false
	for _, v := range podSpec.Volumes {
		if v.Name == expectedVolumeName {
			hasVolume = true
		}
	}
	assert.True(t, hasVolume, "expected speculator OCI volume %s (ociIndex=1)", expectedVolumeName)

	ociParentDir := utils.GetParentDirectory(constants.DefaultSpeculatorLocalMountPath)
	mainContainer := getContainerByName(podSpec, "main")
	hasMount := false
	for _, vm := range mainContainer.VolumeMounts {
		if vm.MountPath == ociParentDir && vm.Name == expectedVolumeName {
			hasMount = true
		}
	}
	assert.True(t, hasMount, "expected volume mount at %s with volume %s", ociParentDir, expectedVolumeName)
}

func TestAttachSpeculatorModelArtifacts_OciDisabled(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("oci://registry.example.com/model:v1")
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
				Config: map[string]string{"method": "eagle3"},
			},
		},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	config := &Config{
		StorageConfig: &kserveTypes.StorageInitializerConfig{
			EnableOciImageSource: false,
		},
	}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, config, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OCI modelcars is not enabled in the cluster configuration")
}

func TestAttachSpeculatorModelArtifacts_NilSpeculator(t *testing.T) {
	t.Parallel()
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, &Config{}, "main")
	require.NoError(t, err)

	mainContainer := getContainerByName(podSpec, "main")
	assert.Empty(t, mainContainer.Env, "no env vars should be injected for nil speculator")
}

func TestAttachSpeculatorModelArtifacts_StorageInitializerDisabled(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("hf://RedHatAI/Qwen3-32B-speculator.eagle3")
	enabled := false
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			StorageInitializer: &v1alpha2.StorageInitializerSpec{
				Enabled: &enabled,
			},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
				Config: map[string]string{"method": "eagle3"},
			},
		},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, &Config{}, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage initializer")
}

func TestAttachSpeculatorModelArtifacts_S3StorageInitializerDisabled(t *testing.T) {
	t.Parallel()
	uri, _ := apis.ParseURL("s3://bucket/speculator-model")
	enabled := false
	llmSvc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			StorageInitializer: &v1alpha2.StorageInitializerSpec{
				Enabled: &enabled,
			},
			Speculator: &v1alpha2.SpeculatorSpec{
				Model:  &v1alpha2.LLMSpeculatorModelSpec{URI: *uri},
				Config: map[string]string{"method": "draft_model"},
			},
		},
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}

	r := &LLMISVCReconciler{}
	err := r.attachSpeculatorModelArtifacts(t.Context(), nil, llmSvc, corev1.PodSpec{}, podSpec, &Config{}, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage initializer")
}

func TestInferJSONType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{"integer", "5", int64(5)},
		{"negative integer", "-3", int64(-3)},
		{"float", "0.9", float64(0.9)},
		{"bool true", "true", true},
		{"bool false", "false", false},
		{"string", "eagle3", "eagle3"},
		{"path", "/mnt/speculator/model", "/mnt/speculator/model"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, inferJSONType(tt.input))
		})
	}
}

// getVLLMAdditionalArgs returns the value of VLLM_ADDITIONAL_ARGS from a container.
func getVLLMAdditionalArgs(container *corev1.Container) string {
	for _, env := range container.Env {
		if env.Name == "VLLM_ADDITIONAL_ARGS" {
			return env.Value
		}
	}
	return ""
}

// extractSpecConfigJSON extracts and parses the JSON from a --speculative-config '...' argument
// in a VLLM_ADDITIONAL_ARGS value. Fails the test if the delimiters are not found or JSON is invalid.
func extractSpecConfigJSON(t *testing.T, vllmArgs string) map[string]interface{} {
	t.Helper()
	start := strings.Index(vllmArgs, "'")
	require.NotEqual(t, -1, start, "no opening quote found in VLLM_ADDITIONAL_ARGS: %s", vllmArgs)

	end := strings.Index(vllmArgs[start+1:], "'")
	require.NotEqual(t, -1, end, "no closing quote found in VLLM_ADDITIONAL_ARGS: %s", vllmArgs)

	configJSON := vllmArgs[start+1 : start+1+end]

	var config map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(configJSON), &config), "failed to parse speculative config JSON %q", configJSON)
	return config
}
