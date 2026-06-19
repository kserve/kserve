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
	"github.com/kserve/kserve/mcv/pkg/config"
)

func CompareTritonCacheManifestToGPU(manifestPath string, devInfo []devices.TritonGPUInfo) error {
	data, err := os.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		return fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest cache.TritonManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	return CompareTritonEntriesToGPU(manifest.Triton, devInfo)
}

func CompareTritonEntriesToGPU(entries []cache.TritonCacheMetadata, devInfo []devices.TritonGPUInfo) error {
	if len(entries) == 0 {
		return errors.New("no cache metadata entries provided")
	}
	if devInfo == nil {
		return errors.New("devInfo is nil")
	}

	var hasMatch bool
	var backendMismatch bool

	for _, entry := range entries {
		dummyKeyMatches := true

		if config.IsBaremetalEnabled() {
			cacheData := &cache.TritonCacheData{
				Hash: entry.Hash,
				Target: cache.Target{
					Backend:  entry.Backend,
					Arch:     entry.Arch,
					WarpSize: entry.WarpSize,
				},
				PtxVersion: &entry.PtxVersion,
				NumStages:  entry.NumStages,
				NumWarps:   entry.NumWarps,
				Debug:      entry.Debug,
			}

			expectedDummyKey, err := cache.ComputeDummyTritonKey(cacheData)
			if err != nil {
				return fmt.Errorf("failed to compute dummy key for entry: %w", err)
			}
			dummyKeyMatches = entry.DummyKey == expectedDummyKey
		}

		for _, gpuInfo := range devInfo {
			backendMatches := entry.Backend == gpuInfo.Backend
			// Normalize architectures for comparison (handles "75" vs "sm_75" for CUDA)
			entryArchStr := cache.ConvertArchToString(entry.Arch)
			normalizedEntryArch := normalizeArchForComparison(entry.Backend, entryArchStr)
			normalizedGPUArch := normalizeArchForComparison(gpuInfo.Backend, gpuInfo.Arch)
			archMatches := normalizedEntryArch == normalizedGPUArch
			warpMatches := entry.WarpSize == gpuInfo.WarpSize

			ptxMatches := true
			if entry.Backend == "cuda" {
				ptxMatches = entry.PtxVersion == gpuInfo.PTXVersion
			}

			if backendMatches && archMatches && warpMatches && ptxMatches && dummyKeyMatches {
				logging.Infof("Compatible cache found: %s", entry.Hash)
				hasMatch = true
				break
			}

			if !backendMatches {
				backendMismatch = true
			}
		}
	}

	if hasMatch {
		return nil
	}
	if backendMismatch {
		return errors.New("incompatibility detected: backend mismatch")
	}
	return errors.New("no compatible GPU found")
}
