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

// KernelCacheStatus defines the observed state of KernelCache
type KernelCacheStatus struct {
	// ResolvedDigest is the image digest (from webhook validation, Phase 2)
	// +optional
	ResolvedDigest string `json:"resolvedDigest,omitempty"`

	// Download status (aggregate from all KernelCacheNodes)
	// NodeStatus removed - unreadable with 500+ nodes, use CacheCopies aggregate instead
	// +optional
	CacheCopies *CacheCopies `json:"cacheCopies,omitempty"` // Aggregate counts

	// GPU compatibility summary (aggregate from all nodes)
	// +optional
	GPUCompatibility *GPUCompatibilitySummary `json:"gpuCompatibility,omitempty"`

	// Phase 2: Serving PVC usage (aggregate across all nodes/namespaces)
	// +optional
	ServingStatus *ServingStatus `json:"servingStatus,omitempty"`

	// Phase 2: ISVCs referencing this cache
	// +optional
	InferenceServices []NamespacedName `json:"inferenceServices,omitempty"`
}

// NodeExtractionStatus represents extraction status on a node
type NodeExtractionStatus string

const (
	NodeExtractionPending    NodeExtractionStatus = "Pending"
	NodeExtractionInProgress NodeExtractionStatus = "InProgress"
	NodeExtractionCompleted  NodeExtractionStatus = "Completed"
	NodeExtractionFailed     NodeExtractionStatus = "Failed"
)

// CacheCopies tracks aggregate extraction counts across all nodes
type CacheCopies struct {
	Total      int `json:"total"`      // Total nodes targeted for extraction
	Available  int `json:"available"`  // Nodes with completed extraction
	Failed     int `json:"failed"`     // Nodes with failed extraction
	InProgress int `json:"inProgress"` // Nodes currently extracting
}

// GPUCompatibilitySummary aggregates GPU compatibility across all nodes
type GPUCompatibilitySummary struct {
	// GPU types this cache is compatible with (from all nodes)
	// +optional
	CompatibleTypes []string `json:"compatibleTypes,omitempty"` // ["Aldebaran/MI200", "nvidia-a100-80gb"]

	// GPU types this cache is incompatible with
	// +optional
	IncompatibleTypes []string `json:"incompatibleTypes,omitempty"` // ["Aldebaran/MI210"]

	// Total compatible GPU count across all nodes
	TotalCompatibleGPUs int `json:"totalCompatibleGPUs"`

	// Total incompatible GPU count
	TotalIncompatibleGPUs int `json:"totalIncompatibleGPUs"`
}

// ServingStatus tracks serving PVC usage (Phase 2)
type ServingStatus struct {
	// Aggregate counts across all nodes/namespaces (Phase 2)
	TotalNamespaces      int `json:"totalNamespaces"`      // Namespaces with serving PVCs
	TotalPods            int `json:"totalPods"`            // Total pods using cache
	TotalPodsReady       int `json:"totalPodsReady"`       // Total ready pods
	TotalPodsTerminating int `json:"totalPodsTerminating"` // Pods being deleted

	// Per-namespace breakdown (for debugging)
	// +optional
	NamespaceCounts map[string]NamespaceServingCounts `json:"namespaceCounts,omitempty"`
}

// NamespacedName is already defined in local_model_cache_status.go
// Reusing that type to avoid duplication
