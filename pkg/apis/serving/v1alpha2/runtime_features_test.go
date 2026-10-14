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

package v1alpha2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

func TestIsKnownRuntime(t *testing.T) {
	tests := []struct {
		name    string
		runtime *string
		want    bool
	}{
		{name: "nil runtime is known (default vLLM)", runtime: nil, want: true},
		{name: "empty string is known (default vLLM)", runtime: ptr.To(""), want: true},
		{name: "SGLang runtime is known", runtime: ptr.To(SGLangRuntimeName), want: true},
		{name: "unknown runtime is not known", runtime: ptr.To("unknown-runtime"), want: false},
		{name: "typo in sglang name is not known", runtime: ptr.To("kserve-llm-SGLang"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsKnownRuntime(tt.runtime))
		})
	}
}

func TestKnownRuntimeNames(t *testing.T) {
	names := KnownRuntimeNames()
	assert.NotEmpty(t, names, "should have at least one known runtime name")
	assert.Contains(t, names, SGLangRuntimeName)

	// Verify sorted order
	for i := 1; i < len(names); i++ {
		assert.Less(t, names[i-1], names[i], "KnownRuntimeNames should be sorted")
	}

	// The empty string (default vLLM) should NOT be in the list — it's implicit
	assert.NotContains(t, names, "")
}

func TestRuntimeSupportsFeature(t *testing.T) {
	allFeatures := []string{
		FeatureNameLoRA,
		FeatureNameTracing,
		FeatureNameKVCacheOffloading,
		FeatureNameTLS,
		FeatureNameMultiNode,
		FeatureNamePrefillDecode,
	}

	tests := []struct {
		name     string
		runtime  *string
		expected map[string]bool
	}{
		{
			name:    "nil runtime (vLLM default) supports all features",
			runtime: nil,
			expected: map[string]bool{
				FeatureNameLoRA:              true,
				FeatureNameTracing:           true,
				FeatureNameKVCacheOffloading: true,
				FeatureNameTLS:               true,
				FeatureNameMultiNode:         true,
				FeatureNamePrefillDecode:     true,
			},
		},
		{
			name:    "empty string runtime (vLLM default) supports all features",
			runtime: ptr.To(""),
			expected: map[string]bool{
				FeatureNameLoRA:              true,
				FeatureNameTracing:           true,
				FeatureNameKVCacheOffloading: true,
				FeatureNameTLS:               true,
				FeatureNameMultiNode:         true,
				FeatureNamePrefillDecode:     true,
			},
		},
		{
			name:    "SGLang supports LoRA and tracing only",
			runtime: ptr.To(SGLangRuntimeName),
			expected: map[string]bool{
				FeatureNameLoRA:              true,
				FeatureNameTracing:           true,
				FeatureNameKVCacheOffloading: false,
				FeatureNameTLS:               false,
				FeatureNameMultiNode:         false,
				FeatureNamePrefillDecode:     false,
			},
		},
		{
			name:    "unknown runtime supports nothing",
			runtime: ptr.To("unknown-runtime"),
			expected: map[string]bool{
				FeatureNameLoRA:              false,
				FeatureNameTracing:           false,
				FeatureNameKVCacheOffloading: false,
				FeatureNameTLS:               false,
				FeatureNameMultiNode:         false,
				FeatureNamePrefillDecode:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, feat := range allFeatures {
				got := RuntimeSupportsFeature(tt.runtime, feat)
				assert.Equal(t, tt.expected[feat], got,
					"RuntimeSupportsFeature(%v, %q)", tt.runtime, feat)
			}
		})
	}
}
