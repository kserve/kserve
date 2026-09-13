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

// CacheState represents overall cache state across all nodes
// +kubebuilder:validation:Enum=Pending;Downloading;Extracted;Running;Error
type CacheState string

const (
	// CacheStatePending - initial state, no extraction started
	CacheStatePending CacheState = "Pending"
	// CacheStateDownloading - extraction Job running on at least one node
	CacheStateDownloading CacheState = "Downloading"
	// CacheStateExtracted - cache extracted on at least one node, not in use
	CacheStateExtracted CacheState = "Extracted"
	// CacheStateRunning - cache mounted by pods on at least one node
	CacheStateRunning CacheState = "Running"
	// CacheStateError - extraction failed on at least one node
	CacheStateError CacheState = "Error"
)

// KernelCacheStatus defines the observed state of KernelCache
// +k8s:openapi-gen=true
type KernelCacheStatus struct {
	// State represents overall cache state across all nodes
	// Hierarchy: Error > Running > Extracted > Downloading > Pending
	// +optional
	State CacheState `json:"state,omitempty"`

	// MountType tracks the active mount type (pvc or imageVolume)
	// Reflects the spec.mountType value for observability
	// +optional
	MountType KernelCacheMountType `json:"mountType,omitempty"`

	// ResolvedDigest is the image digest (sha256:...) resolved by mutating webhook
	// This field is immutable once set - copied from annotation on first reconcile
	// Controller ALWAYS uses this field (not annotation) to prevent tampering
	// +optional
	ResolvedDigest string `json:"resolvedDigest,omitempty"`

	// Counts tracks aggregate node and pod counts for state calculation
	// +optional
	Counts *CacheCounts `json:"counts,omitempty"`

	// GPU compatibility summary (aggregate from all nodes)
	// +optional
	GPUCompatibility *GPUCompatibilitySummary `json:"gpuCompatibility,omitempty"`

	// Serving PVC usage (aggregate across all nodes/namespaces)
	// +optional
	ServingStatus *ServingStatus `json:"servingStatus,omitempty"`
}

// NodeCacheState represents per-node cache state
// +kubebuilder:validation:Enum=Pending;Downloading;Extracted;Running;Error
type NodeCacheState string

const (
	// NodeCacheStatePending - initial state, no extraction started
	NodeCacheStatePending NodeCacheState = "Pending"
	// NodeCacheStateDownloading - extraction Job running on this node
	NodeCacheStateDownloading NodeCacheState = "Downloading"
	// NodeCacheStateExtracted - cache extracted, not in use
	NodeCacheStateExtracted NodeCacheState = "Extracted"
	// NodeCacheStateRunning - cache mounted by pods on this node
	NodeCacheStateRunning NodeCacheState = "Running"
	// NodeCacheStateError - extraction failed on this node
	NodeCacheStateError NodeCacheState = "Error"
)

// CacheCounts tracks aggregate node counts for state calculation
// +k8s:openapi-gen=true
type CacheCounts struct {
	// NodeCnt - total nodes with this cache tracked
	NodeCnt int `json:"nodeCnt"`
	// NodeErrorCnt - nodes with extraction errors
	NodeErrorCnt int `json:"nodeErrorCnt"`
	// NodeInUseCnt - nodes with cache mounted by pods
	NodeInUseCnt int `json:"nodeInUseCnt"`
	// NodeNotInUseCnt - nodes with cache extracted but not mounted
	NodeNotInUseCnt int `json:"nodeNotInUseCnt"`
}

// GPUCompatibilitySummary aggregates GPU compatibility across all nodes
// +k8s:openapi-gen=true
type GPUCompatibilitySummary struct {
	// GPU types this cache is compatible with (from all nodes)
	// +optional
	// +listType=atomic
	CompatibleTypes []string `json:"compatibleTypes,omitempty"` // ["Aldebaran/MI200", "nvidia-a100-80gb"]

	// GPU types this cache is incompatible with
	// +optional
	// +listType=atomic
	IncompatibleTypes []string `json:"incompatibleTypes,omitempty"` // ["Aldebaran/MI210"]

	// Total compatible GPU count across all nodes
	TotalCompatibleGPUs int `json:"totalCompatibleGPUs"`

	// Total incompatible GPU count
	TotalIncompatibleGPUs int `json:"totalIncompatibleGPUs"`
}

// ServingStatus tracks pod usage of this cache across all nodes
// +k8s:openapi-gen=true
type ServingStatus struct {
	// TotalPodsUsing is the total number of non-terminating pods across all nodes
	// that currently have this cache mounted
	TotalPodsUsing int `json:"totalPodsUsing"`
}
