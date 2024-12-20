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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type LocalModelInfo struct {
	// Original StorageUri
	SourceModelUri string `json:"sourceModelUri" validate:"required"`
	// Model name. Used as the subdirectory name to store this model on local file system
	ModelName string `json:"modelName" validate:"required"`
}

// +k8s:openapi-gen=true
type LocalModelNodeSpec struct {
	// List of model source URI and their names
	LocalModels []LocalModelInfo `json:"localModels" validate:"required"`
}

// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope="Cluster"
type LocalModelNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocalModelNodeSpec   `json:"spec,omitempty"`
	Status LocalModelNodeStatus `json:"status,omitempty"`
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LocalModelNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LocalModelNode `json:"items" validate:"required"`
}

func init() {
	SchemeBuilder.Register(&LocalModelNode{}, &LocalModelNodeList{})
}
