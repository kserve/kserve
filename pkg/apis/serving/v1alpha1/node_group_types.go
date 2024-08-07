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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelCacheNodeGroupSpec defines a group of nodes for to download the model to.
// +k8s:openapi-gen=true
type ModelCacheNodeGroupSpec struct {
	StorageLimit resource.Quantity `json:"storageLimit"`
	NodeSelector map[string]string `json:"nodeSelector"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="persistentVolume is immutable"
	// PV spec template
	PersistentVolume corev1.PersistentVolume `json:"persistentVolume"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="persistentVolumeClaim is immutable"
	// PVC spec template
	PersistentVolumeClaim corev1.PersistentVolumeClaim `json:"persistentVolumeClaim"`
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
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ModelCacheNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelCacheNodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelCacheNodeGroup{}, &ModelCacheNodeGroupList{})
}
