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
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetStorageKey returns a deterministic hash of the sourceModelUri for folder naming.
// This enables storage deduplication - all models with the same URI share the same folder.
func GetStorageKey(sourceModelUri string) string {
	hash := sha256.Sum256([]byte(sourceModelUri))
	return hex.EncodeToString(hash[:])[:16] // Use first 16 chars of hash
}

// LocalModelStorageSpec defines credential and storage configuration for model download
// +k8s:openapi-gen=true
type LocalModelStorageSpec struct {
	// The Storage Key in the secret for this object.
	// +optional
	StorageKey *string `json:"key,omitempty"`
	// Parameters to override the default storage credentials and config.
	// +optional
	Parameters *map[string]string `json:"parameters,omitempty"`
}

// LocalModelCacheSpec
// +k8s:openapi-gen=true
type LocalModelCacheSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="StorageUri is immutable"
	// Original StorageUri
	SourceModelUri string `json:"sourceModelUri" validate:"required"`
	// Model size to make sure it does not exceed the disk space reserved for local models. The limit is defined on the NodeGroup.
	ModelSize resource.Quantity `json:"modelSize" validate:"required"`
	// group of nodes to cache the model on.
	// Todo: support more than 1 node groups
	// +kubebuilder:validation:MinItems=1
	NodeGroups []string `json:"nodeGroups" validate:"required"`
	// ServiceAccountName specifies the service account to use for credential lookup.
	// The service account should have secrets attached that contain the credentials
	// for accessing the model storage (e.g., HuggingFace token, S3 credentials).
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Storage configuration for credentials and storage parameters.
	// +optional
	Storage *LocalModelStorageSpec `json:"storage,omitempty"`
}

// LocalModelCache
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope="Cluster"
type LocalModelCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocalModelCacheSpec   `json:"spec,omitempty"`
	Status LocalModelCacheStatus `json:"status,omitempty"`
}

// LocalModelCacheList
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LocalModelCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LocalModelCache `json:"items" validate:"required"`
}

func init() {
	SchemeBuilder.Register(&LocalModelCache{}, &LocalModelCacheList{})
}

// If the storageUri from inference service matches the sourceModelUri of the LocalModelCache or is a subdirectory of the sourceModelUri, return true
func (spec *LocalModelCacheSpec) MatchStorageURI(storageUri string) bool {
	cachedUri := strings.TrimSuffix(spec.SourceModelUri, "/")
	isvcStorageUri := strings.TrimSuffix(storageUri, "/")
	if strings.HasPrefix(isvcStorageUri, cachedUri) {
		if len(isvcStorageUri) == len(cachedUri) {
			return true
		}

		// If the storageUri is a subdirectory of the cachedUri, the next character after the cachedUri should be a "/"
		if len(cachedUri) < len(isvcStorageUri) && string(isvcStorageUri[len(cachedUri)]) == "/" {
			return true
		}
	}
	return false
}
