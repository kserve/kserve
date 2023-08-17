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

// // StorageContainerSpec defines the container spec for the storage initializer init container, and the protocols it supports.
// // +k8s:openapi-gen=true
// type StorageContainerSpec struct {
// 	StorageContainer corev1.Container `json:"storageContainer" validate:"required"`

// 	SupportedPrefixes []string `json:"supportedPrefixes,omitempty"`
// 	SupportedRegexes  []string `json:"supportedRegexes,omitempty"`
// }

type StorageUriFormatType string

const (
	Prefix StorageUriFormatType = "Prefix"
	Regex  StorageUriFormatType = "Regex"
)

// +k8s:openapi-gen=true
type SupportedUriFormat struct {
	// +required
	Name string `json:"name" validate:"required"`
	// +required
	Type StorageUriFormatType `json:"version,omitempty" validate:"required"`
	// +required
	Str string `json:"str,omitempty" validate:"required"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope="Cluster"
type ClusterStorageContainer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Container           corev1.Container     `json:"container" validate:"required"`
	SupportedUriFormats []SupportedUriFormat `json:"supportedUriFormats" validate:"required"`

	// +optional
	Disabled *bool `json:"disabled,omitempty"`
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type ClusterStorageContainerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterStorageContainer `json:"items" validate:"required"`
}

func init() {
	SchemeBuilder.Register(&ClusterStorageContainer{}, &ClusterStorageContainerList{})
}

func (sc *ClusterStorageContainer) IsDisabled() bool {
	return sc.Disabled != nil && *sc.Disabled
}

func (sc *ClusterStorageContainer) IsStorageUriSupported(storageUri string) bool {
	for _, supportedUriFormat := range sc.SupportedUriFormats {
		switch supportedUriFormat.Type {
		case Prefix:
			if strings.HasPrefix(storageUri, supportedUriFormat.Str) {
				return true
			}
		case Regex:
			// Todo: handle error
			match, _ := regexp.MatchString(supportedUriFormat.Str, storageUri)
			if match {
				return true
			}
		}
	}
	return false
}
