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

// +k8s:openapi-gen=true
type ClusterCachedModelSpec struct {

	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="storageUri is immutable"
	// Original StorageUri
	StorageUri string `json:"storageUri"`
	// Model size to make sure it does not exceed the disk space reserved for local models. The limit is defined on the NodeGroup.
	ModelSize resource.Quantity `json:"modelSize"`
	// A group of nodes to cache the model on.
	NodeGroup string `json:"nodeGroup"`
	// only local is supported for now
	StorageType StorageType `json:"storageType"`
	// Whether model cache controller creates a job to delete models on local disks.
	CleanupPolicy CleanupPolicy `json:"cleanupPolicy"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="persistentVolume is immutable"
	// PV spec template
	PersistentVolume corev1.PersistentVolumeClaim `json:"persistentVolume"`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="persistentVolumeClaim is immutable"
	// PVC spec template
	PersistentVolumeClaim corev1.PersistentVolumeClaim `json:"persistentVolumeClaim"`
}

// StorageType enum
// +kubebuilder:validation:Enum="";LocalPV
type StorageType string

// StorageType Enum values
const (
	LocalPV StorageType = "LocalPV"
)

// CleanupPolicy enum
// +kubebuilder:validation:Enum="";DeleteModel;Ignore
type CleanupPolicy string

// CleanupPolicy Enum values
const (
	DeleteModel CleanupPolicy = "DeleteModel"
	Ignore      CleanupPolicy = "Ignore"
)

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope="Cluster"
type ClusterCachedModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterCachedModelSpec `json:"spec,omitempty"`
}

// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ClusterCachedModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterCachedModel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterCachedModel{}, &ClusterCachedModelList{})
}
