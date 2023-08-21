/*
Copyright 2023 The KServe Authors.

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
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageContainerSpec defines the container spec for the stoarge initializer init container, and the protocols it supports.
// +k8s:openapi-gen=true
type StorageContainerSpec struct {
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	StorageContainer corev1.Container `json:"storageContainer" validate:"required"`

	SupportedPrefixes []string `json:"supportedPrefixes,omitempty"`
	SupportedRegexes  []string `json:"supportedRegexes,omitempty"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope="Cluster"
type ClusterStorageContainer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec StorageContainerSpec `json:"spec,omitempty"`
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type ClusterStorageContainerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterStorageContainer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterStorageContainer{}, &ClusterStorageContainerList{})
}

func (scSpec *StorageContainerSpec) IsDisabled() bool {
	return scSpec.Disabled != nil && *scSpec.Disabled
}

func (scSpec *StorageContainerSpec) IsStorageUriSupported(storageUri string) bool {
	for _, prefix := range scSpec.SupportedPrefixes {
		if strings.HasPrefix(storageUri, prefix) {
			return true
		}
	}
	for _, pattern := range scSpec.SupportedRegexes {
		match, _ := regexp.MatchString(pattern, storageUri)
		if match {
			return true
		}
	}
	return false
}
