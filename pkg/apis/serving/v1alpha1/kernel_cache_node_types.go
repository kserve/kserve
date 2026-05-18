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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KernelCacheNode tracks per-node kernel cache extraction status.
// Operator-created CRD pattern uses Status only (no Spec).
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
type KernelCacheNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec removed - operator-created CRD pattern uses Status only
	Status KernelCacheNodeStatus `json:"status,omitempty"`
}

// KernelCacheInfo identifies a kernel cache to extract
type KernelCacheInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Digest    string `json:"digest,omitempty"`
}

// KernelCacheNodeStatus defines per-node extraction and GPU compatibility status
type KernelCacheNodeStatus struct {
	// NodeName is the Kubernetes node this tracks (moved from Spec)
	NodeName string `json:"nodeName"`

	// Caches lists kernel caches to extract on this node (moved from Spec)
	// +optional
	Caches []KernelCacheInfo `json:"caches,omitempty"`

	// GPU info: list of GPU types detected on this node (from MCV)
	// Can be empty (CPU-only node) or heterogeneous (mixed GPU types)
	// +optional
	GPUInfo []GPUTypeInfo `json:"gpuInfo,omitempty"`

	// CacheStatus maps cache name to extraction and compatibility status
	// +optional
	CacheStatus map[string]CacheNodeExtractionStatus `json:"cacheStatus,omitempty"`
}

// CacheNodeExtractionStatus tracks extraction and compatibility for one cache on one node
type CacheNodeExtractionStatus struct {
	// Download phase tracking
	DownloadStatus NodeExtractionStatus `json:"downloadStatus"`

	// GPU compatibility (from MCV validation during extraction)
	// Lists GPU IDs that are compatible/incompatible with this cache
	// +optional
	CompatibleGPUs []int `json:"compatibleGPUs,omitempty"`
	// +optional
	IncompatibleGPUs []int `json:"incompatibleGPUs,omitempty"`

	// Phase 2: Serving PVC usage on this node (per-namespace counts)
	// +optional
	ServingNamespaces map[string]NamespaceServingCounts `json:"servingNamespaces,omitempty"`

	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
}

// NamespaceServingCounts tracks pod usage per namespace
type NamespaceServingCounts struct {
	PodsUsing       int `json:"podsUsing"`       // Total pods mounting cache in this namespace
	PodsReady       int `json:"podsReady"`       // Pods in Ready state
	PodsTerminating int `json:"podsTerminating"` // Pods being deleted
}

// KernelCacheNodeList contains a list of KernelCacheNode
// +kubebuilder:object:root=true
type KernelCacheNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCacheNode `json:"items"`
}
