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

// StorageContainerSpec defines the container spec for the storage initializer init container, and the protocols it supports.
// +k8s:openapi-gen=true
type StorageContainerSpec struct {
	// Container spec for the storage initializer init container
	Container corev1.Container `json:"container" validate:"required"`

	// List of URI formats that this container supports
	SupportedUriFormats []SupportedUriFormat `json:"supportedUriFormats" validate:"required"`
	// +kubebuilder:default="initContainer"
	WorkloadType WorkloadType `json:"workloadType,omitempty"`
}

// SupportedUriFormat can be either prefix or regex. Todo: Add validation that only one of them is set.
// +k8s:openapi-gen=true
type SupportedUriFormat struct {
	Prefix string `json:"prefix,omitempty"`
	Regex  string `json:"regex,omitempty"`
}

// +k8s:openapi-gen=true
type WorkloadType string

const (
	InitContainer         WorkloadType = "initContainer"
	LocalModelDownloadJob WorkloadType = "localModelDownloadJob"
)

// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope="Cluster"
type ClusterStorageContainer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec StorageContainerSpec `json:"spec,omitempty"`

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

func (spec *StorageContainerSpec) IsStorageUriSupported(storageUri string) (bool, error) {
	for _, supportedUriFormat := range spec.SupportedUriFormats {
		if supportedUriFormat.Prefix != "" {
			if strings.HasPrefix(storageUri, supportedUriFormat.Prefix) {
				return true, nil
			}
		} else if supportedUriFormat.Regex != "" {
			match, err := regexp.MatchString(supportedUriFormat.Regex, storageUri)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		}
	}
	return false, nil
}
