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

package devices

// TritonGPUInfo holds key GPU fields relevant to Triton cache validation
// It now supports both NVIDIA (CUDA) and AMD (ROCm) GPUs.
type TritonGPUInfo struct {
	// Name represents the model name of the GPU (e.g., "Tesla V100", "RTX 3090", "Radeon RX 6900 XT").
	// This field is universal for all GPUs.
	Name string `json:"name"`

	// UUID represents the unique identifier of the GPU, useful for distinguishing multiple GPUs.
	// This field is also universal.
	UUID string `json:"uuid"`

	// ComputeCapability reflects the GPU's compute capability. For CUDA GPUs, it's a string
	// (e.g., "6.1" for Volta), while for ROCm GPUs, it might be represented by a version number
	// or a similar capability specifier. It could be used to identify supported instruction sets
	// and hardware features.
	ComputeCapability string `json:"compute_capability"`

	// Arch is a numerical representation of the GPU's architecture. For CUDA GPUs, it's a specific
	// number (e.g., 60 for Maxwell, 70 for Volta), while ROCm might have a different notation
	// (e.g., "gfx906" for Vega GPUs).
	Arch string `json:"arch"`

	// WarpSize reflects the number of threads in a single warp on the GPU.
	// This field would be applicable to CUDA GPUs (usually 32 threads per warp), but might be
	// handled differently in ROCm, as AMD uses wavefronts, which might not have a direct one-to-one mapping.
	WarpSize int `json:"warp_size"`

	// MemoryTotalMB represents the total amount of memory available on the GPU in megabytes.
	// This is common for both NVIDIA and AMD GPUs.
	MemoryTotalMB uint64 `json:"memory_total_mb"`

	// PTXVersion indicates the PTX version used for NVIDIA CUDA GPUs.
	// For ROCm, this would be replaced with a similar field for the intermediate language (e.g., HIP).
	PTXVersion int `json:"ptx_version"`

	Backend string `json:"backend"`

	ID int
}

type GPUDevice struct {
	ID         int
	TritonInfo TritonGPUInfo
	Summary    DeviceSummary
}
