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

package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"

	corev1 "k8s.io/api/core/v1"
)

// KernelCachePodTemplate customizes extraction Job pods
// +k8s:openapi-gen=true
type KernelCachePodTemplate struct {
	// NodeSelector for extraction Jobs
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for extraction Jobs
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// PriorityClassName for extraction Jobs
	// +optional
	PriorityClassName *string `json:"priorityClassName,omitempty"`
}

// GPUTypeInfo describes GPU hardware on a node
// +k8s:openapi-gen=true
type GPUTypeInfo struct {
	// GPU type identifier (from MCV detection)
	// Examples: "Aldebaran/MI200", "nvidia-a100-80gb"
	GPUType string `json:"gpuType"`

	// Driver version for this GPU type
	// +optional
	DriverVersion string `json:"driverVersion,omitempty"` // "6.12.10-100.fc40.x86_64"

	// CUDA or ROCm version
	// +optional
	CUDAVersion string `json:"cudaVersion,omitempty"` // "12.0" (NVIDIA)
	// +optional
	ROCmVersion string `json:"rocmVersion,omitempty"` // "5.7" (AMD)

	// GPU device IDs for this type (0-indexed)
	// Example: [0, 1, 2, 3] means GPUs 0-3 are this type
	// Supports heterogeneous nodes with mixed GPU types
	IDs []int `json:"ids"`
}

// GetKernelCacheStorageKey returns hash of image URI for storage deduplication
// Same pattern as LocalModelCache's GetStorageKey()
func GetKernelCacheStorageKey(imageUri string) string {
	hash := sha256.Sum256([]byte(imageUri))
	return hex.EncodeToString(hash[:])[:16] // First 16 chars of SHA256
}
