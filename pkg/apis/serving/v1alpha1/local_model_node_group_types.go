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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalModelNodeGroupSpec defines a group of nodes for to download the model to.
// +k8s:openapi-gen=true
type LocalModelNodeGroupSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="persistentVolume is immutable"
	PersistentVolumeSpec corev1.PersistentVolumeSpec `json:"persistentVolumeSpec"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="persistentVolumeClaim is immutable"
	PersistentVolumeClaimSpec corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaimSpec"`
}

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
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
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LocalModelNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LocalModelNodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LocalModelNodeGroup{}, &LocalModelNodeGroupList{})
}
