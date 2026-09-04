/*
Copyright 2024 The KServe Authors.

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalModelNamespaceCacheSpec defines the spec for namespace-scoped local model cache.
//
// Exactly one storage mode must be selected: either node-local caching via nodeGroups,
// or shared-PVC import via pvcRef. The two are mutually exclusive.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="!(has(self.nodeGroups) && has(self.pvcRef))",message="nodeGroups and pvcRef are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="has(self.nodeGroups) || has(self.pvcRef)",message="one of nodeGroups or pvcRef must be set"
type LocalModelNamespaceCacheSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="StorageUri is immutable"
	// Original StorageUri
	SourceModelUri string `json:"sourceModelUri" validate:"required"`
	// Model size to make sure it does not exceed the disk space reserved for local models. The limit is defined on the NodeGroup.
	ModelSize resource.Quantity `json:"modelSize" validate:"required"`
	// group of nodes to cache the model on. Selects the legacy node-local caching mode.
	// Mutually exclusive with pvcRef.
	// +kubebuilder:validation:MinItems=1
	// +optional
	NodeGroups []string `json:"nodeGroups,omitempty"`
	// PVCRef is the name of a pre-created PersistentVolumeClaim in the cache CR's namespace.
	// Selects shared-PVC import mode: the model is imported once onto the referenced claim and
	// shared read-only by serving replicas. The claim must be ReadWriteMany with filesystem
	// volume mode. It is immutable; changing the destination requires a new cache CR.
	// Mutually exclusive with nodeGroups.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="pvcRef is immutable"
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +optional
	PVCRef *string `json:"pvcRef,omitempty"`
	// ServiceAccountName specifies the service account to use for credential lookup.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// +optional
	Storage *LocalModelStorageSpec `json:"storage,omitempty"`
}

// SharedPVCMode reports whether the cache uses shared-PVC import mode.
func (spec *LocalModelNamespaceCacheSpec) SharedPVCMode() bool {
	return spec.PVCRef != nil && *spec.PVCRef != ""
}

// LocalModelNamespaceCache is a namespace-scoped version of LocalModelCache.
// It allows InferenceServices only in the same namespace to use the cached model.
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope="Namespaced"
type LocalModelNamespaceCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocalModelNamespaceCacheSpec `json:"spec,omitempty"`
	Status LocalModelCacheStatus        `json:"status,omitempty"`
}

// LocalModelNamespaceCacheList contains a list of LocalModelNamespaceCache
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LocalModelNamespaceCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LocalModelNamespaceCache `json:"items" validate:"required"`
}

func init() {
	SchemeBuilder.Register(&LocalModelNamespaceCache{}, &LocalModelNamespaceCacheList{})
}

// MatchStorageURI checks if storageUri matches the sourceModelUri or is a subdirectory of it
func (spec *LocalModelNamespaceCacheSpec) MatchStorageURI(storageUri string) bool {
	return MatchStorageURI(spec.SourceModelUri, storageUri)
}
