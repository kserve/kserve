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
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/accelerator"
	"github.com/kserve/kserve/mcv/pkg/accelerator/devices"
	"github.com/kserve/kserve/mcv/pkg/cache"
	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/constants"
)

// normalizeArchForComparison normalizes architecture strings for comparison
// Strips sm_ prefix from CUDA architectures to handle both "75" and "sm_75" formats
func normalizeArchForComparison(backend, arch string) string {
	if backend == "cuda" {
		return strings.TrimPrefix(arch, "sm_")
	}
	return arch
}

func CompareCacheSummaryLabelToGPU(img v1.Image, labels map[string]string, devInfo []devices.TritonGPUInfo) (matched, unmatched []devices.TritonGPUInfo, err error) {
	logging.Debug("Starting cache summary label preflight check...")
	if labels == nil {
		configFile, ret := img.ConfigFile()
		if ret != nil {
			return nil, nil, fmt.Errorf("failed to get image config: %w", ret)
		}

		labels = configFile.Config.Labels
		if labels == nil {
			return nil, nil, errors.New("image has no labels")
		}
	}

	summaryStr, ok := labels["cache.triton.image/summary"]
	if !ok {
		if summaryStr, ok = labels["cache.vllm.image/summary"]; !ok {
			return nil, nil, errors.New("image missing cache summary label")
		}
	}

	var summary cache.Summary
	if err = json.Unmarshal([]byte(summaryStr), &summary); err != nil {
		return nil, nil, fmt.Errorf("failed to parse summary label: %w", err)
	}

	logging.Debugf("Preflight check: devInfo has %d GPUs, summary has %d targets", len(devInfo), len(summary.Targets))
	for i, gpu := range devInfo {
		logging.Debugf("GPU[%d]: backend=%s, arch=%s, warp=%d", i, gpu.Backend, gpu.Arch, gpu.WarpSize)
	}
	for i, target := range summary.Targets {
		logging.Debugf("Target[%d]: backend=%s, arch=%s, warp=%d", i, target.Backend, target.Arch, target.WarpSize)
	}

	for _, gpu := range devInfo {
		isMatch := false
		for _, target := range summary.Targets {
			backendMatches := target.Backend == gpu.Backend
			// Normalize architectures for comparison (handles "75" vs "sm_75" for CUDA)
			normalizedTargetArch := normalizeArchForComparison(target.Backend, target.Arch)
			normalizedGPUArch := normalizeArchForComparison(gpu.Backend, gpu.Arch)
			archMatches := normalizedTargetArch == normalizedGPUArch
			warpMatches := target.WarpSize == gpu.WarpSize

			logging.Debugf("Comparing cache target vs GPU: backend=%s vs %s, arch=%s(%s) vs %s(%s), warp=%d vs %d",
				target.Backend, gpu.Backend,
				target.Arch, normalizedTargetArch,
				gpu.Arch, normalizedGPUArch,
				target.WarpSize, gpu.WarpSize)

			if backendMatches && archMatches && warpMatches {
				isMatch = true
				break
			}
		}

		if isMatch {
			matched = append(matched, gpu)
		} else {
			unmatched = append(unmatched, gpu)
		}
	}

	if len(matched) == 0 {
		err = fmt.Errorf("no compatible GPU found from summary preflight check")
	}

	return matched, unmatched, err
}

// DetectCacheTypeFromLabels inspects image labels to determine cache type ("triton" or "vllm")
func DetectCacheTypeFromLabels(labels map[string]string) (string, error) {
	if labels == nil {
		return "", fmt.Errorf("no labels provided")
	}
	if _, ok := labels["cache.triton.image/summary"]; ok {
		return constants.Triton, nil
	}
	if _, ok := labels["cache.vllm.image/summary"]; ok {
		return constants.VLLM, nil
	}
	return "", fmt.Errorf("unknown cache type from labels")
}

// CompareCacheManifestToGPU dispatches manifest comparison based on cache type
func CompareCacheManifestToGPU(manifestPath, cacheType string, devInfo []devices.TritonGPUInfo) error {
	if cacheType == "" {
		return fmt.Errorf("cache type is empty")
	}
	switch cacheType {
	case constants.Triton:
		return CompareTritonCacheManifestToGPU(manifestPath, devInfo)
	case constants.VLLM:
		return CompareVLLMCacheManifestToGPU(manifestPath, devInfo)
	default:
		return fmt.Errorf("unsupported cache type: %s", cacheType)
	}
}

func GetAllGPUInfo(acc accelerator.Accelerator) ([]devices.TritonGPUInfo, error) {
	if acc == nil {
		return nil, fmt.Errorf("accelerator is nil")
	}
	if !config.IsGPUEnabled() {
		return nil, nil
	}
	gpu := accelerator.GetActiveAcceleratorByType(config.GPU)
	if gpu == nil {
		return nil, fmt.Errorf("no active GPU accelerator found")
	}
	device := gpu.Device()
	return device.GetAllGPUInfo()
}
