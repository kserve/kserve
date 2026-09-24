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
)

func TestBuildFootprintsByImageReference(t *testing.T) {
	modelHash := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name              string
		image             string
		wantWorkload      bool
		wantCompatibility bool
	}{
		{name: "digest", image: "registry.example/vllm@sha256:" + strings.Repeat("b", 64), wantWorkload: true, wantCompatibility: true},
		{name: "version tag", image: "registry.example/vllm:v0.10", wantCompatibility: true},
		{name: "latest", image: "registry.example/vllm:latest"},
		{name: "implicit latest", image: "registry.example/vllm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Build(KernelCacheIdentityFactors{
				Namespace: "team", WorkloadKind: "InferenceService", WorkloadName: "model", RuntimeImage: tc.image,
				ModelURIHash: modelHash, RuntimeConfigFactors: map[string]string{TensorParallelSizeFactor: "2"},
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantWorkload, result.Footprints.WorkloadFootprint != "")
			require.Equal(t, tc.wantCompatibility, result.Footprints.CompatibilityFootprint != "")
			require.Equal(t, "team", result.Factors[NamespaceFactor])
			require.Equal(t, "model", result.Factors[WorkloadNameFactor])
			require.Equal(t, tc.image, result.Factors[RuntimeImageFactor])
			require.Equal(t, modelHash, result.Factors[ModelURIHashFactor])
		})
	}
}

func TestBuildRequiresModelURIHash(t *testing.T) {
	_, err := Build(KernelCacheIdentityFactors{
		Namespace: "team", WorkloadKind: "InferenceService", WorkloadName: "model",
		RuntimeImage: "registry.example/vllm:v0.10",
	})
	require.Error(t, err)
}

func TestBuildTrimsRuntimeImage(t *testing.T) {
	image := "registry.example/vllm@sha256:" + strings.Repeat("b", 64)
	result, err := Build(KernelCacheIdentityFactors{
		Namespace: "team", WorkloadKind: "InferenceService", WorkloadName: "model",
		RuntimeImage: "  " + image + "  ", ModelURIHash: "sha256:" + strings.Repeat("a", 64),
	})

	require.NoError(t, err)
	require.Equal(t, image, result.Factors[RuntimeImageFactor])
	require.NotEmpty(t, result.Footprints.WorkloadFootprint)
}

func TestBuildSeparatesWorkloadAndCompatibilityFactors(t *testing.T) {
	base := KernelCacheIdentityFactors{
		Namespace:            "team-a",
		WorkloadKind:         "InferenceService",
		WorkloadName:         "model-a",
		RuntimeImage:         "registry.example/vllm@sha256:" + strings.Repeat("b", 64),
		ModelURIHash:         "sha256:" + strings.Repeat("a", 64),
		RuntimeConfigFactors: map[string]string{TensorParallelSizeFactor: "2"},
		CommandHash:          "sha256:" + strings.Repeat("c", 64),
		ArgsHash:             "sha256:" + strings.Repeat("d", 64),
	}
	first, err := Build(base)
	require.NoError(t, err)

	otherWorkload := base
	otherWorkload.Namespace = "team-b"
	otherWorkload.WorkloadName = "model-b"
	otherWorkload.CommandHash = "sha256:" + strings.Repeat("e", 64)
	otherWorkload.ArgsHash = "sha256:" + strings.Repeat("f", 64)
	second, err := Build(otherWorkload)
	require.NoError(t, err)

	require.NotEqual(t, first.Footprints.WorkloadFootprint, second.Footprints.WorkloadFootprint)
	require.Equal(t, first.Footprints.CompatibilityFootprint, second.Footprints.CompatibilityFootprint)
}

func TestBuildIncludesWorkloadMetadataOnlyInWorkloadFootprint(t *testing.T) {
	base := KernelCacheIdentityFactors{
		Namespace:            "team-a",
		WorkloadKind:         "InferenceService",
		WorkloadName:         "model-a",
		RuntimeImage:         "registry.example/vllm@sha256:" + strings.Repeat("b", 64),
		ModelURIHash:         "sha256:" + strings.Repeat("a", 64),
		RuntimeConfigFactors: map[string]string{TensorParallelSizeFactor: "2"},
	}
	first, err := Build(base)
	require.NoError(t, err)

	otherWorkload := base
	otherWorkload.Namespace = "team-b"
	otherWorkload.WorkloadName = "model-b"
	second, err := Build(otherWorkload)
	require.NoError(t, err)

	require.NotEqual(t, first.Footprints.WorkloadFootprint, second.Footprints.WorkloadFootprint)
	require.Equal(t, first.Footprints.CompatibilityFootprint, second.Footprints.CompatibilityFootprint)
	require.NotEqual(t, first.Factors[NamespaceFactor], second.Factors[NamespaceFactor])
	require.NotEqual(t, first.Factors[WorkloadNameFactor], second.Factors[WorkloadNameFactor])
}

func TestBuildSeparatesWorkloadKinds(t *testing.T) {
	base := KernelCacheIdentityFactors{
		Namespace:            "team",
		WorkloadKind:         "InferenceService",
		WorkloadName:         "model",
		RuntimeImage:         "registry.example/vllm@sha256:" + strings.Repeat("b", 64),
		ModelURIHash:         "sha256:" + strings.Repeat("a", 64),
		RuntimeConfigFactors: map[string]string{TensorParallelSizeFactor: "2"},
	}
	first, err := Build(base)
	require.NoError(t, err)

	secondInput := base
	secondInput.WorkloadKind = "LLMInferenceService"
	second, err := Build(secondInput)
	require.NoError(t, err)

	require.NotEqual(t, first.Footprints.WorkloadFootprint, second.Footprints.WorkloadFootprint)
	require.Equal(t, first.Footprints.CompatibilityFootprint, second.Footprints.CompatibilityFootprint)
	require.Equal(t, "InferenceService", first.Factors[WorkloadKindFactor])
	require.Equal(t, "LLMInferenceService", second.Factors[WorkloadKindFactor])
}

func TestBuildIncludesRuntimeConfigFactorsOnlyInCompatibilityFootprint(t *testing.T) {
	base := KernelCacheIdentityFactors{
		Namespace:    "team-a",
		WorkloadKind: "InferenceService",
		WorkloadName: "model-a",
		RuntimeImage: "registry.example/vllm@sha256:" + strings.Repeat("b", 64),
		ModelURIHash: "sha256:" + strings.Repeat("a", 64),
		CommandHash:  "sha256:" + strings.Repeat("c", 64),
		ArgsHash:     "sha256:" + strings.Repeat("d", 64),
		RuntimeConfigFactors: map[string]string{
			TensorParallelSizeFactor: "2",
			DTypeFactor:              "bfloat16",
		},
	}
	first, err := Build(base)
	require.NoError(t, err)

	changed := base
	changed.RuntimeConfigFactors = map[string]string{
		TensorParallelSizeFactor: "2",
		DTypeFactor:              "float16",
	}
	second, err := Build(changed)
	require.NoError(t, err)

	require.Equal(t, first.Footprints.WorkloadFootprint, second.Footprints.WorkloadFootprint)
	require.NotEqual(t, first.Footprints.CompatibilityFootprint, second.Footprints.CompatibilityFootprint)
	require.Equal(t, "bfloat16", first.Factors[DTypeFactor])
}
