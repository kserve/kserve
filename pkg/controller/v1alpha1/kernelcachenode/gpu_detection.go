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

package kernelcachenode

import (
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	mcvClient "github.com/redhat-et/GKM/mcv/pkg/client"
	mcvDevices "github.com/redhat-et/GKM/mcv/pkg/accelerator/devices"
)

// populateGPUInfo calls MCV GetGpuList() to detect GPU hardware
// Pattern from GKM's agent implementation
// Supports heterogeneous nodes with mixed GPU types
// When noGPU flag is set (for KIND testing), returns stub GPU data
func (c *KernelCacheNodeReconciler) populateGPUInfo(
	kcNode *v1alpha1.KernelCacheNode,
	noGPU bool,
) error {
	if len(kcNode.Status.GPUInfo) > 0 {
		return nil // Already populated
	}

	// Call MCV GetSystemGPUInfo (GKM pattern)
	// When noGPU=true, MCV returns stub data for KIND testing
	// When noGPU=false, MCV reads from cache file (fast) or detects hardware (slow, first call)
	disableTimeout := 0 // 0 = use default timeout
	gpus, err := mcvClient.GetSystemGPUInfo(mcvClient.HwOptions{
		EnableStub: &noGPU,
		Timeout:    disableTimeout,
	})
	if err != nil {
		// Log error but don't fail reconciliation - GPU detection may not work in all environments
		c.Log.Error(err, "failed to get GPU info from MCV", "node", kcNode.Status.NodeName)
		return nil // Graceful degradation - continue without GPU info
	}

	if noGPU {
		c.Log.Info("noGPU flag set - using MCV stub GPU data for testing", "node", kcNode.Status.NodeName)
	} else {
		gpuCount := 0
		if gpus != nil {
			gpuCount = len(gpus.GPUs)
		}
		c.Log.Info("Detected GPU devices via MCV", "node", kcNode.Status.NodeName, "gpuCount", gpuCount)
	}

	// Convert MCV response to GPUTypeInfo list (supports heterogeneous nodes)
	kcNode.Status.GPUInfo = convertMCVToGPUTypeInfo(gpus)
	return nil
}

// convertMCVToGPUTypeInfo converts MCV GPUFleetSummary to GPUTypeInfo list
// MCV already groups GPUs by type (handles heterogeneous nodes)
func convertMCVToGPUTypeInfo(gpuSummary *mcvDevices.GPUFleetSummary) []v1alpha1.GPUTypeInfo {
	if gpuSummary == nil || len(gpuSummary.GPUs) == 0 {
		return []v1alpha1.GPUTypeInfo{} // CPU-only node
	}

	// MCV already groups GPUs by type - just convert to our format
	result := make([]v1alpha1.GPUTypeInfo, 0, len(gpuSummary.GPUs))

	for _, gpuGroup := range gpuSummary.GPUs {
		gpuInfo := v1alpha1.GPUTypeInfo{
			GPUType:       gpuGroup.GPUType,
			DriverVersion: gpuGroup.DriverVersion,
			IDs:           gpuGroup.IDs,
		}

		// Parse vendor-specific version info from GPUType
		// MCV returns normalized type like "nvidia-a100-80gb" or "Aldebaran/MI200"
		if strings.Contains(gpuGroup.GPUType, "nvidia") {
			// NVIDIA GPU - would need to query CUDA version separately if needed
			// For now, leave CUDAVersion empty (can add later via NVML query)
			gpuInfo.CUDAVersion = ""
			gpuInfo.ROCmVersion = ""
		} else if strings.Contains(gpuGroup.GPUType, "/") {
			// AMD GPU - format is "Architecture/Model"
			gpuInfo.CUDAVersion = ""
			gpuInfo.ROCmVersion = "" // Would need to query ROCm version separately
		}

		result = append(result, gpuInfo)
	}

	return result
}

// updateCacheCompatibility validates which GPUs are compatible with this cache
// Phase 1: Stub implementation - marks all GPUs as compatible
// Phase 2+: Will use MCV cache validation when available
func (c *KernelCacheNodeReconciler) updateCacheCompatibility(
	kcNode *v1alpha1.KernelCacheNode,
	cacheName string,
	cacheImage string,
	noGPU bool,
) error {
	cacheStatus := kcNode.Status.CacheStatus[cacheName]

	// Phase 1: Mark all GPUs as compatible (no validation yet)
	// MCV cache validation integration planned for Phase 2
	c.Log.V(1).Info("marking all GPUs as compatible (validation not yet implemented)",
		"cache", cacheName, "node", kcNode.Status.NodeName)

	// Mark all detected GPUs as compatible
	for _, gpuInfo := range kcNode.Status.GPUInfo {
		cacheStatus.CompatibleGPUs = append(cacheStatus.CompatibleGPUs, gpuInfo.IDs...)
	}
	cacheStatus.IncompatibleGPUs = []int{} // No incompatible GPUs in Phase 1

	kcNode.Status.CacheStatus[cacheName] = cacheStatus
	return nil
}
