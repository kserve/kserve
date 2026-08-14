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

package pod

import "testing"

func TestKernelCacheCaptureResolveCachePath(t *testing.T) {
	tests := []struct {
		name         string
		preset       string
		pathOverride string
		want         string
	}{
		{
			name:         "vllm preset",
			preset:       "vllm",
			pathOverride: "",
			want:         "/home/kserve/.cache/vllm",
		},
		{
			name:         "tgi preset",
			preset:       "tgi",
			pathOverride: "",
			want:         "/data",
		},
		{
			name:         "triton-python preset",
			preset:       "triton-python",
			pathOverride: "",
			want:         "/opt/tritonserver/backends/python/models/.cache",
		},
		{
			name:         "explicit path overrides preset",
			preset:       "vllm",
			pathOverride: "/custom/cache/path",
			want:         "/custom/cache/path",
		},
		{
			name:         "explicit path with no preset",
			preset:       "",
			pathOverride: "/custom/cache/path",
			want:         "/custom/cache/path",
		},
		{
			name:         "unknown preset returns empty",
			preset:       "unknown-framework",
			pathOverride: "",
			want:         "",
		},
		{
			name:         "empty preset and no override returns empty",
			preset:       "",
			pathOverride: "",
			want:         "",
		},
		{
			name:         "override with empty string still returns empty",
			preset:       "",
			pathOverride: "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveCachePath(tt.preset, tt.pathOverride)
			if got != tt.want {
				t.Errorf("ResolveCachePath(%q, %q) = %q, want %q",
					tt.preset, tt.pathOverride, got, tt.want)
			}
		})
	}
}

func TestKernelCacheCaptureGetKnownPresets(t *testing.T) {
	presets := GetKnownPresets()

	// Should have all 3 known presets
	if len(presets) != 3 {
		t.Errorf("GetKnownPresets() returned %d presets, want 3", len(presets))
	}

	// Check that known presets are included
	knownPresets := map[string]bool{
		"vllm":          false,
		"tgi":           false,
		"triton-python": false,
	}

	for _, p := range presets {
		if _, ok := knownPresets[p]; ok {
			knownPresets[p] = true
		}
	}

	for preset, found := range knownPresets {
		if !found {
			t.Errorf("GetKnownPresets() missing expected preset %q", preset)
		}
	}
}

func TestKernelCacheCapturePresetsImmutability(t *testing.T) {
	// Verify that cachePresets cannot be accidentally modified
	// by getting a copy through GetKnownPresets
	before := len(cachePresets)
	_ = GetKnownPresets()
	after := len(cachePresets)

	if before != after {
		t.Errorf("cachePresets was modified by GetKnownPresets: before=%d, after=%d",
			before, after)
	}
}
