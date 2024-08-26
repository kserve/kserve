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

// +k8s:openapi-gen=true
type ClusterLocalModelSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="StorageUri is immutable"
	// Original StorageUri
	SourceModelUri string `json:"sourceModelUri" validate:"required"`
	// Model size to make sure it does not exceed the disk space reserved for local models. The limit is defined on the NodeGroup.
	ModelSize resource.Quantity `json:"modelSize" validate:"required"`
	// group of nodes to cache the model on.
	NodeGroup string `json:"nodeGroup" validate:"required"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope="Cluster"
type ClusterLocalModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterLocalModelSpec   `json:"spec,omitempty"`
	Status ClusterLocalModelStatus `json:"status,omitempty"`
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterLocalModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterLocalModel `json:"items" validate:"required"`
}

func init() {
	SchemeBuilder.Register(&ClusterLocalModel{}, &ClusterLocalModelList{})
}
