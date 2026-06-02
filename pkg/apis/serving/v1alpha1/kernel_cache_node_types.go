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
// Cluster-scoped like LocalModelNode - one per physical node, tracks caches from all namespaces.
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Caches-In-Use",type=integer,JSONPath=".status.counts.cachesInUse"
// +kubebuilder:printcolumn:name="Caches-Not-In-Use",type=integer,JSONPath=".status.counts.cachesNotInUse"
// +kubebuilder:printcolumn:name="Caches-Error",type=integer,JSONPath=".status.counts.cachesError"
// +kubebuilder:printcolumn:name="Pod-Running",type=integer,JSONPath=".status.counts.podRunningCnt"
// +kubebuilder:printcolumn:name="Pod-Deleting",type=integer,JSONPath=".status.counts.podDeletingCnt"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCacheNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec removed - operator-created CRD pattern uses Status only
	Status KernelCacheNodeStatus `json:"status,omitempty"`
}

// KernelCacheNodeStatus defines per-node extraction and GPU compatibility status
// Agent owns all writes to this structure
type KernelCacheNodeStatus struct {
	// NodeName is the Kubernetes node this tracks
	NodeName string `json:"nodeName"`

	// GPU info: list of GPU types detected on this node (from MCV)
	// Can be empty (CPU-only node) or heterogeneous (mixed GPU types)
	// +optional
	GPUInfo []GPUTypeInfo `json:"gpuInfo,omitempty"`

	// CacheStatus maps cache name (unique within namespace) to full cache info and status
	// Agent discovers caches by watching KernelCache CRs and populates this map
	// Key format: "{namespace}/{name}" for uniqueness across namespaces
	// +optional
	CacheStatus map[string]CacheNodeCacheInfo `json:"cacheStatus,omitempty"`

	// Aggregate counts across all caches on this node (for kubectl get display)
	// +optional
	Counts *NodeCacheCounts `json:"counts,omitempty"`
}

// NodeCacheCounts aggregates cache and pod counts across all caches on a node
type NodeCacheCounts struct {
	// CachesInUse - caches in Running state (mounted by pods)
	CachesInUse int `json:"cachesInUse"`
	// CachesNotInUse - caches in Extracted state (available but not mounted)
	CachesNotInUse int `json:"cachesNotInUse"`
	// CachesError - caches in Error state
	CachesError int `json:"cachesError"`
	// PodRunningCnt - total pods using any cache on this node
	PodRunningCnt int `json:"podRunningCnt"`
	// PodDeletingCnt - total pods terminating
	PodDeletingCnt int `json:"podDeletingCnt"`
}

// CacheNodeCacheInfo tracks full cache information and status on one node
// Combines cache identity (name/namespace/image/digest) with extraction state
// Agent populates all fields by watching KernelCache CRs and tracking extraction jobs
type CacheNodeCacheInfo struct {
	// Cache identity (agent reads from KernelCache CR)
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Digest    string `json:"digest,omitempty"`

	// State represents this cache's state on this specific node
	// +optional
	State NodeCacheState `json:"state,omitempty"`

	// GPU compatibility (from MCV validation during extraction)
	// Lists GPU IDs that are compatible/incompatible with this cache
	// +optional
	CompatibleGPUs []int `json:"compatibleGPUs,omitempty"`
	// +optional
	IncompatibleGPUs []int `json:"incompatibleGPUs,omitempty"`

	// Serving PVC usage on this node (per-namespace counts)
	// +optional
	ServingNamespaces map[string]NamespaceServingCounts `json:"servingNamespaces,omitempty"`

	// Message provides details about current state (e.g., error messages)
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdate timestamp
	// +optional
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
}

// CacheNodeExtractionStatus is deprecated - use CacheNodeCacheInfo
// Kept for backward compatibility during migration
type CacheNodeExtractionStatus = CacheNodeCacheInfo

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
