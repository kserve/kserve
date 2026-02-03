/*
Copyright 2026 The KServe Authors.

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

// LocalModelCacheDeploymentSpec defines the desired state of LocalModelCacheDeployment
type LocalModelCacheDeploymentSpec struct {
	// Source URI of the model (e.g., s3://bucket/model)
	// +kubebuilder:validation:Required
	SourceModelUri string `json:"sourceModelUri"`
	// Size of the model for storage allocation
	// +kubebuilder:validation:Required
	ModelSize resource.Quantity `json:"modelSize"`
	// Node groups to cache the model on
	// +kubebuilder:validation:MinItems=1
	NodeGroups []string `json:"nodeGroups"`
}

// LocalModelCacheDeploymentRevision represents a revision of the LocalModelCacheDeployment
type LocalModelCacheDeploymentRevision struct {
	// Name of the LocalModelCache for this revision
	Name string `json:"name"`
	// Revision number
	Revision int32 `json:"revision"`
}

// LocalModelCacheDeploymentStatus defines the observed state of LocalModelCacheDeployment
type LocalModelCacheDeploymentStatus struct {
	// Current active revision name
	// +optional
	CurrentRevision string `json:"currentRevision,omitempty"`
	// ObservedGeneration is the last observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// List of all revisions
	// +optional
	Revisions []LocalModelCacheDeploymentRevision `json:"revisions,omitempty"`
}

// LocalModelCacheDeployment is the Schema for the localmodels API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope="Cluster"
// +kubebuilder:printcolumn:name="SourceURI",type="string",JSONPath=".spec.sourceModelUri"
// +kubebuilder:printcolumn:name="CurrentRevision",type="string",JSONPath=".status.currentRevision"
// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
type LocalModelCacheDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocalModelCacheDeploymentSpec   `json:"spec,omitempty"`
	Status LocalModelCacheDeploymentStatus `json:"status,omitempty"`
}

// LocalModelCacheDeploymentList contains a list of LocalModelCacheDeployment
// +kubebuilder:object:root=true
type LocalModelCacheDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LocalModelCacheDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LocalModelCacheDeployment{}, &LocalModelCacheDeploymentList{})
}
