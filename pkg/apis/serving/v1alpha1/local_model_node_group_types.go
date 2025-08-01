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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalModelNodeGroupSpec defines a group of nodes for to download the model to.
// +k8s:openapi-gen=true
type LocalModelNodeGroupSpec struct {
	// Max storage size per node in this node group
	StorageLimit resource.Quantity `json:"storageLimit"`
	// Used to create PersistentVolumes for downloading models and in inference service namespaces
	PersistentVolumeSpec corev1.PersistentVolumeSpec `json:"persistentVolumeSpec"`
	// Used to create PersistentVolumeClaims for download and in inference service namespaces
	PersistentVolumeClaimSpec corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec"`
}

// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope="Cluster"
type LocalModelNodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocalModelNodeGroupSpec   `json:"spec,omitempty"`
	Status LocalModelNodeGroupStatus `json:"status,omitempty"`
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
type LocalModelNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LocalModelNodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LocalModelNodeGroup{}, &LocalModelNodeGroupList{})
}
