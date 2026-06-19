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

package preflightcheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/accelerator/devices"
	"github.com/kserve/kserve/mcv/pkg/cache"
)

// CompareVLLMCacheManifestToGPU compares VLLM manifest entries to GPU info
// Handles both triton cache (legacy) and binary cache (new) formats
func CompareVLLMCacheManifestToGPU(manifestPath string, devInfo []devices.TritonGPUInfo) error {
	data, err := os.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		return fmt.Errorf("failed to read VLLM manifest file: %w", err)
	}

	var manifest cache.VLLMManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse VLLM manifest JSON: %w", err)
	}

	for _, entry := range manifest.VLLM {
		// Check cache format and validate accordingly
		switch entry.CacheFormat {
		case cache.BinaryCacheFormat:
			if len(entry.BinaryCacheEntries) > 0 {
				if err := compareBinaryCacheEntriesToGPU(entry.BinaryCacheEntries, devInfo); err != nil {
					return err
				}
			}
		case cache.AOTCompileCacheFormat:
			if len(entry.AOTCompileEntries) > 0 {
				if err := compareAOTCompileCacheEntriesToGPU(entry.AOTCompileEntries, devInfo); err != nil {
					return err
				}
			}
		case cache.TritonCacheFormat:
			if len(entry.TritonCacheEntries) > 0 {
				// Handle triton cache format (legacy)
				// TritonCacheEntries contains JSON-unmarshalled map[string]interface{} values,
				// so we need to re-marshal and unmarshal to get proper cache.TritonCacheMetadata structs
				convertedEntries := make([]cache.TritonCacheMetadata, len(entry.TritonCacheEntries))
				for i, e := range entry.TritonCacheEntries {
					// Re-marshal the entry to JSON
					jsonData, err := json.Marshal(e)
					if err != nil {
						return fmt.Errorf("failed to marshal triton cache entry: %w", err)
					}
					// Unmarshal into proper struct
					if err := json.Unmarshal(jsonData, &convertedEntries[i]); err != nil {
						return fmt.Errorf("failed to unmarshal triton cache entry: %w", err)
					}
				}
				if err := CompareTritonEntriesToGPU(convertedEntries, devInfo); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown cache format: %s", entry.CacheFormat)
		}
	}

	return nil
}

// compareAOTCompileCacheEntriesToGPU validates AOT compile cache entries against GPU hardware
// AOT compile caches have limited metadata, so this primarily relies on the summary-based check
func compareAOTCompileCacheEntriesToGPU(entries []cache.AOTCompileCacheMetadata, _ []devices.TritonGPUInfo) error {
	// AOT compile cache entries don't contain cache_key_factors.json with env vars,
	// so we can't extract detailed hardware requirements from the manifest.
	// The summary label (created during image build) contains the actual GPU info
	// and is checked by CompareCacheSummaryLabelToGPU.
	//
	// Here we just verify the entries exist and log for debugging.
	if len(entries) == 0 {
		return errors.New("no AOT compiled cache entries found")
	}

	// Log the AOT cache entries for debugging
	for _, entry := range entries {
		logging.Debugf("AOT compiled cache: hash=%s, rank=%s, size=%d bytes",
			entry.Hash, entry.Rank, entry.FileSize)
	}

	// Actual hardware compatibility is validated via the summary label
	return nil
}

// compareBinaryCacheEntriesToGPU validates binary cache entries against GPU hardware
// Note: Binary cache metadata doesn't directly contain compute capability.
// The Summary label (built during image creation using actual GPU detection) is the
// primary source of truth for hardware compatibility. This function provides a basic
// backend-level check.
func compareBinaryCacheEntriesToGPU(entries []cache.BinaryCacheMetadata, devInfo []devices.TritonGPUInfo) error {
	for i := range entries {
		entry := &entries[i]
		// Extract backend from the binary cache metadata
		backend := entry.TargetDevice
		if backend == "" {
			backend = cache.CUDABackend // Default if not specified
		}

		// Basic warp size validation based on backend
		expectedWarpSize := 32 // Default for CUDA
		switch backend {
		case cache.ROCmBackend, cache.HIPBackend:
			expectedWarpSize = 64 // AMD GPUs use 64-wide wavefronts
		case cache.CUDABackend:
			expectedWarpSize = 32 // NVIDIA GPUs use 32-wide warps
		case "tpu":
			expectedWarpSize = 128 // TPU uses different parallelism model
		case "cpu":
			expectedWarpSize = 1 // CPU doesn't have warp concept
		}

		// Check if any GPU matches the backend and warp size
		matched := false
		for _, gpu := range devInfo {
			backendMatches := backend == gpu.Backend
			warpMatches := expectedWarpSize == gpu.WarpSize

			if backendMatches && warpMatches {
				matched = true
				// For detailed arch compatibility, rely on Summary label check
				logging.Debugf("Binary cache entry matches GPU: backend=%s, warpSize=%d",
					backend, expectedWarpSize)
				break
			}
		}

		if !matched {
			return fmt.Errorf("binary cache entry (backend=%s, warpSize=%d) does not match any available GPU. Use Summary label for precise arch validation", backend, expectedWarpSize)
		}
	}

	return nil
}
