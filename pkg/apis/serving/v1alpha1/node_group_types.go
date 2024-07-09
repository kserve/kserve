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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelCacheNodeGroupSpec defines the container spec for the storage initializer init container, and the protocols it supports.
// +k8s:openapi-gen=true
type ModelCacheNodeGroupSpec struct {
	StorageLimit resource.Quantity `json:"storageLimit" validate:"required"`
	NodeSelector map[string]string `json:"nodeSelector" validate:"required"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope="Cluster"
type ModelCacheNodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelCacheNodeGroupSpec `json:"spec,omitempty"`

	// +optional
	Disabled *bool `json:"disabled,omitempty"`
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ModelCacheNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelCacheNodeGroup `json:"items" validate:"required"`
}

func init() {
	SchemeBuilder.Register(&ModelCacheNodeGroup{}, &ModelCacheNodeGroupList{})
}
