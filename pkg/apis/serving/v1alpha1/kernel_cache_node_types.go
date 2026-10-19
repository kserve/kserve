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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// KernelCacheNode represents node-local preparation and usage projections.
// It is operator-created and cluster-scoped.
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=kcn
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=".status.counts.cachesReady"
// +kubebuilder:printcolumn:name="Preparing",type=integer,JSONPath=".status.counts.cachesPreparing"
// +kubebuilder:printcolumn:name="Error",type=integer,JSONPath=".status.counts.cachesError"
// +kubebuilder:printcolumn:name="Pods-Using",type=integer,JSONPath=".status.counts.totalPodsUsing"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
type KernelCacheNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            KernelCacheNodeStatus `json:"status,omitempty"`
}

// KernelCacheNodePreparationState represents node-local preparation state.
// +kubebuilder:validation:Enum=Pending;Pulling;Extracting;Ready;Error
type KernelCacheNodePreparationState string

const (
	KernelCacheNodePreparationStatePending    KernelCacheNodePreparationState = "Pending"
	KernelCacheNodePreparationStatePulling    KernelCacheNodePreparationState = "Pulling"
	KernelCacheNodePreparationStateExtracting KernelCacheNodePreparationState = "Extracting"
	KernelCacheNodePreparationStateReady      KernelCacheNodePreparationState = "Ready"
	KernelCacheNodePreparationStateError      KernelCacheNodePreparationState = "Error"
)

// KernelCacheNodeStatus defines node-local cache preparation and usage status.
// +k8s:openapi-gen=true
type KernelCacheNodeStatus struct {
	// CacheStatus maps "namespace/name" to node-local cache status.
	// +optional
	CacheStatus map[string]KernelCacheNodeCacheInfo `json:"cacheStatus,omitempty"`

	// Counts contains aggregate counts for this node.
	// +optional
	Counts *KernelCacheNodeCounts `json:"counts,omitempty"`
}

// KernelCacheNodeCounts contains aggregate node-local counts.
// +k8s:openapi-gen=true
type KernelCacheNodeCounts struct {
	// CachesReady is the number of ready caches on this node.
	CachesReady int `json:"cachesReady"`

	// CachesPreparing is the number of caches being prepared.
	CachesPreparing int `json:"cachesPreparing"`

	// CachesError is the number of caches with preparation errors.
	CachesError int `json:"cachesError"`

	// TotalPodsUsing is the total number of Pods using caches on this node.
	TotalPodsUsing int `json:"totalPodsUsing"`
}

// KernelCacheNodeCacheInfo contains one cache's state on a node.
// +k8s:openapi-gen=true
type KernelCacheNodeCacheInfo struct {
	// KernelCacheRef identifies the namespaced KernelCache object.
	KernelCacheRef NamespacedName `json:"kernelCacheRef"`

	// ImageReference identifies the artifact being prepared on this node.
	// +optional
	ImageReference string `json:"imageReference,omitempty"`

	// State represents the preparation state of this cache on this node.
	// +optional
	State KernelCacheNodePreparationState `json:"state,omitempty"`

	// Message provides details about the current preparation state.
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdate is the time when this cache status was last changed on this node.
	// +optional
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`

	// Footprints identifies the cache artifact available on this node.
	Footprints KernelCacheFootprints `json:"footprints"`
}

// KernelCacheNodeList contains a list of KernelCacheNode objects.
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type KernelCacheNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KernelCacheNode `json:"items"`
}
