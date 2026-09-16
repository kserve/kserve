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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// KernelCacheState represents the aggregate node preparation state.
type KernelCacheState string

const (
	KernelCacheStatePending   KernelCacheState = "Pending"
	KernelCacheStatePreparing KernelCacheState = "Preparing"
	KernelCacheStateReady     KernelCacheState = "Ready"
	KernelCacheStateError     KernelCacheState = "Error"
)

// KernelCacheArtifactSecurityState represents a signing or verification outcome.
type KernelCacheArtifactSecurityState string

const (
	KernelCacheArtifactSecurityStatePending   KernelCacheArtifactSecurityState = "Pending"
	KernelCacheArtifactSecurityStateSucceeded KernelCacheArtifactSecurityState = "Succeeded"
	KernelCacheArtifactSecurityStateFailed    KernelCacheArtifactSecurityState = "Failed"
	KernelCacheArtifactSecurityStateSkipped   KernelCacheArtifactSecurityState = "Skipped"
)

// KernelCacheSigningStatus contains artifact signing results.
// +k8s:openapi-gen=true
type KernelCacheSigningStatus struct {
	// Mode is the configured signing mode.
	// +kubebuilder:validation:Enum=none;cert
	Mode string `json:"mode"`

	// State is the latest signing outcome.
	// +kubebuilder:validation:Enum=Pending;Succeeded;Failed;Skipped
	State KernelCacheArtifactSecurityState `json:"state"`

	// Signed indicates whether a signature was created.
	Signed bool `json:"signed"`

	// Reason describes the signing result.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message provides additional signing details.
	// +optional
	Message string `json:"message,omitempty"`

	// SignedAt is the time of successful signing.
	// +optional
	SignedAt *metav1.Time `json:"signedAt,omitempty"`
}

// KernelCacheVerificationStatus contains artifact verification results.
// +k8s:openapi-gen=true
type KernelCacheVerificationStatus struct {
	// Mode is the configured verification mode.
	// +kubebuilder:validation:Enum=none;cert
	Mode string `json:"mode"`

	// State is the latest verification outcome.
	// +kubebuilder:validation:Enum=Pending;Succeeded;Failed;Skipped
	State KernelCacheArtifactSecurityState `json:"state"`

	// Verified indicates whether the immutable artifact passed verification.
	Verified bool `json:"verified"`

	// Reason describes the verification result.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message provides additional verification details.
	// +optional
	Message string `json:"message,omitempty"`

	// VerifiedAt is the time of the latest verification.
	// +optional
	VerifiedAt *metav1.Time `json:"verifiedAt,omitempty"`
}

// KernelCacheUsage contains the Pods currently using a KernelCache.
// +k8s:openapi-gen=true
type KernelCacheUsage struct {
	// Pods lists active consumers. Entries are keyed by PodUID.
	// +optional
	// +listType=map
	// +listMapKey=podUID
	Pods []KernelCachePodUsage `json:"pods,omitempty"`

	// TotalPodsUsing is derived from Pods.
	// +optional
	TotalPodsUsing int `json:"totalPodsUsing,omitempty"`
}

// KernelCachePodUsage identifies one Pod consuming a cache.
// +k8s:openapi-gen=true
type KernelCachePodUsage struct {
	// PodUID is the stable identity used as the list key.
	PodUID types.UID `json:"podUID"`

	// PodRef contains the current Pod namespace and name.
	PodRef NamespacedName `json:"podRef"`

	// NodeName is the node hosting the Pod.
	NodeName string `json:"nodeName"`

	// ObservedAt is the last time this usage was observed.
	// +optional
	ObservedAt metav1.Time `json:"observedAt,omitempty"`
}

// KernelCacheCounts contains aggregate node preparation counts.
// +k8s:openapi-gen=true
type KernelCacheCounts struct {
	// NodeCount is the number of nodes tracked for this cache.
	// +kubebuilder:validation:Minimum=0
	NodeCount int `json:"nodeCount"`

	// NodesReady is the number of nodes where the cache is ready.
	// +kubebuilder:validation:Minimum=0
	NodesReady int `json:"nodesReady"`

	// NodesPreparing is the number of nodes preparing the cache.
	// +kubebuilder:validation:Minimum=0
	NodesPreparing int `json:"nodesPreparing"`

	// NodesError is the number of nodes with preparation errors.
	// +kubebuilder:validation:Minimum=0
	NodesError int `json:"nodesError"`
}

// KernelCacheStatus defines the observed state of a KernelCache.
// +k8s:openapi-gen=true
type KernelCacheStatus struct {
	// State is the aggregate node preparation state.
	// +kubebuilder:validation:Enum=Pending;Preparing;Ready;Error
	// +optional
	State KernelCacheState `json:"state,omitempty"`

	// MountType is the observed cache delivery mode.
	// +kubebuilder:validation:Enum=oci
	// +optional
	MountType KernelCacheMountType `json:"mountType,omitempty"`

	// Verification contains the artifact verification result.
	// +optional
	Verification *KernelCacheVerificationStatus `json:"verification,omitempty"`

	// Usage contains active Pod consumers.
	// +optional
	Usage *KernelCacheUsage `json:"usage,omitempty"`

	// Counts contains aggregate node preparation counts.
	// +optional
	Counts *KernelCacheCounts `json:"counts,omitempty"`

	// Conditions represent the latest availability observations.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}
