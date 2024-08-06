/*
Copyright 2021 The KServe Authors.

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

type CachedModelStatus struct {
	OverallStatus OverallStatus         `json:"overallStatus,omitempty"`
	NodeStatus    map[string]NodeStatus `json:"nodeStatus,omitempty"`

	// How many nodes have the model available
	// +optional
	ModelCopies       *ModelCopies     `json:"copies,omitempty"`
	InferenceServices []NamespacedName `json:"infereneceServices,omitempty"`
}

type NamespacedName struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// NodeStatus enum
// +kubebuilder:validation:Enum="";Ready;Unknown;Downloading;Deleting;Deleted
type NodeStatus string

// NodeStatus Enum values
const (
	Ready       NodeStatus = "Ready"
	Downloading NodeStatus = "Downloading"
	Deleting    NodeStatus = "Deleting"
	Deleted     NodeStatus = "Deleted"
)

// OverallStatus enum
// +kubebuilder:validation:Enum="";Ready;Unknown;Downloading;Deleting
type OverallStatus string

// OverallStatus Enum values
const (
	ModelReady          OverallStatus = "Ready"
	ModelPartiallyReady OverallStatus = "PartiallyReady"
	ModelDownloading    OverallStatus = "Downloading"
	ModelDeleting       OverallStatus = "Deleting"
)

type ModelCopies struct {
	// +kubebuilder:default=0
	SuccessfulCopies int `json:"successfulCopies"`
	// +optional
	TotalCopies int `json:"totalCopies,omitempty"`
}
