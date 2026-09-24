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

package identity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestParseRuntimeConfigFactors(t *testing.T) {
	factors := ParseRuntimeConfigFactors(
		[]string{"python", "-m", "vllm.entrypoints.openai.api_server", "--dtype=bfloat16", "-tp", "2"},
		[]string{
			"--max-model-len", "4096",
			"--quantization", "awq",
			"--compilation-config", "{\"level\":3}",
			"--enable-chunked-prefill",
			"--no-enable-chunked-prefill",
		},
		map[string]string{
			"VLLM_USE_AOT_COMPILE":              "1",
			"VLLM_ENABLE_INDUCTOR_MAX_AUTOTUNE": "true",
			"VLLM_DISABLED_KERNELS":             "Marlin, ExLlama",
			"VLLM_CACHE_ROOT":                   "/tmp/vllm",
		},
	)

	require.Equal(t, map[string]string{
		"option.dtype":                          "bfloat16",
		"option.tensorParallelSize":             "2",
		"option.maxModelLen":                    "4096",
		"option.quantization":                   "awq",
		"option.compilationConfigHash":          HashStrings([]string{"{\"level\":3}"}),
		"option.enableChunkedPrefill":           "false",
		"env.VLLM_USE_AOT_COMPILE":              "true",
		"env.VLLM_ENABLE_INDUCTOR_MAX_AUTOTUNE": "true",
		"env.VLLM_DISABLED_KERNELS":             "ExLlama,Marlin",
	}, factors)
}

func TestParseRuntimeConfigFactorsCanonicalizesCommaSeparatedValues(t *testing.T) {
	first := ParseRuntimeConfigFactors(nil, nil, map[string]string{
		"VLLM_DISABLED_KERNELS": "Marlin, ExLlama",
	})
	second := ParseRuntimeConfigFactors(nil, nil, map[string]string{
		"VLLM_DISABLED_KERNELS": "ExLlama,Marlin",
	})

	require.Equal(t, "ExLlama,Marlin", first["env.VLLM_DISABLED_KERNELS"])
	require.Equal(t, first, second)
}

func TestParseRuntimeConfigFactorsUsesArgsOverCommand(t *testing.T) {
	factors := ParseRuntimeConfigFactors(
		[]string{"--max-num-seqs=128", "--tensor_parallel_size=2"},
		[]string{"--max-num-seqs", "256", "--tp=4"},
		nil,
	)

	require.Equal(t, "256", factors["option.maxNumSeqs"])
	require.Equal(t, "4", factors[TensorParallelSizeFactor])
}

func TestParseRuntimeConfigFactorsPreservesJSONValue(t *testing.T) {
	value := `{"level": 3, "mode": "max-autotune"}`
	factors := ParseRuntimeConfigFactors(nil, []string{"--compilation-config", value}, nil)

	require.Equal(t, HashStrings([]string{value}), factors[CompilationConfigFactor])
}

func TestExtractRuntimeConfigFactorsFromShellContainer(t *testing.T) {
	container := &corev1.Container{
		Command: []string{"/bin/bash", "-c", `eval "exec vllm serve ${VLLM_ADDITIONAL_ARGS} $@"`},
		Env:     []corev1.EnvVar{{Name: "VLLM_ADDITIONAL_ARGS", Value: "--dtype float16 --tensor-parallel-size 4"}},
	}

	factors := ExtractRuntimeConfigFactors(container)

	require.Equal(t, "float16", factors[DTypeFactor])
	require.Equal(t, "4", factors[TensorParallelSizeFactor])
}

func TestParseRuntimeConfigFactorsIgnoresUnsupportedAndInvalidValues(t *testing.T) {
	factors := ParseRuntimeConfigFactors(
		[]string{"--unknown-option=value", "--tensor-parallel-size=0"},
		nil,
		map[string]string{
			"VLLM_USE_AOT_COMPILE": "not-a-bool",
			"VLLM_CACHE_ROOT":      "/tmp/vllm",
		},
	)

	require.Empty(t, factors)
}

func TestIsStableRuntimeImageRequiresDigest(t *testing.T) {
	digest := "registry.example/vllm@sha256:" + strings.Repeat("a", 64)

	for _, test := range []struct {
		name   string
		image  string
		stable bool
	}{
		{name: "digest", image: digest, stable: true},
		{name: "version tag", image: "registry.example/vllm:v0.10", stable: false},
		{name: "latest tag", image: "registry.example/vllm:latest", stable: false},
		{name: "implicit latest", image: "registry.example/vllm", stable: false},
		{name: "invalid digest", image: "registry.example/vllm@sha256:bad", stable: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.stable, IsStableRuntimeImage(test.image))
		})
	}
}
