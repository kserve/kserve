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

// cachePresets maps known framework names to their default cache directory paths
// These paths are where each framework typically writes JIT-compiled kernel caches
var cachePresets = map[string]string{
	// vLLM stores compiled kernels in ~/.cache/vllm by default
	// See: https://docs.vllm.ai/en/latest/
	"vllm": "/home/kserve/.cache/vllm",

	// Text Generation Inference (TGI) uses /data as the model cache root
	// Kernels are compiled relative to this directory
	// See: https://huggingface.co/docs/text-generation-inference
	"tgi": "/data",

	// Triton Python backend models cache to this location
	// See: https://github.com/triton-inference-server/python_backend
	"triton-python": "/opt/tritonserver/backends/python/models/.cache",
}

// ResolveCachePath resolves a cache path from preset or explicit path
// Returns the resolved path, or empty string if neither preset nor path is provided
//
// Resolution logic:
// 1. If pathOverride is non-empty, use it (explicit always wins)
// 2. Else if preset is non-empty and known, use preset's path
// 3. Else return empty string
//
// Example usage:
//
//	path := ResolveCachePath("vllm", "")           // Returns "/home/kserve/.cache/vllm"
//	path := ResolveCachePath("vllm", "/custom")    // Returns "/custom" (override wins)
//	path := ResolveCachePath("", "/custom")        // Returns "/custom"
//	path := ResolveCachePath("unknown", "")        // Returns "" (unknown preset)
func ResolveCachePath(preset, pathOverride string) string {
	// Explicit path always wins
	if pathOverride != "" {
		return pathOverride
	}

	// Look up preset
	if preset != "" {
		if path, ok := cachePresets[preset]; ok {
			return path
		}
	}

	// No valid preset or override
	return ""
}

// GetKnownPresets returns a list of all known cache preset names
// Useful for validation and error messages
func GetKnownPresets() []string {
	presets := make([]string, 0, len(cachePresets))
	for k := range cachePresets {
		presets = append(presets, k)
	}
	return presets
}
