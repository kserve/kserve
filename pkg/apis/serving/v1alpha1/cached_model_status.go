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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CachedModelStatus struct {
	// Addressable endpoint for the deployed trained model
	// http://<inferenceservice.metadata.name>/v1/models/<trainedmodel>.metadata.name
	OverallStatus OverallStatus         `json:"address,omitempty"`
	NodeStatus    map[string]NodeStatus `json:"nodeStatus,omitempty"`
	// Details of last failure, when load of target model is failed or blocked.
	// +optional
	LastFailureInfo *ModelCacheFailureInfo `json:"lastFailureInfo,omitempty"`

	// Model copy information of the predictor's model.
	// +optional
	ModelCopies *ModelCopies `json:"copies,omitempty"`
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
// +kubebuilder:validation:Enum="";Ready;Unknown;Downloading;Deleting;Deleted
type OverallStatus string

// OverallStatus Enum values
const (
	ModelReady          OverallStatus = "Ready"
	ModelPartiallyReady OverallStatus = "PartiallyReady"
	ModelDownloading    OverallStatus = "Downloading"
	ModelDeleting       OverallStatus = "Deleting"
	ModelDeleted        OverallStatus = "Deleted"
)

type ModelCacheFailureInfo struct {
	// Name of component to which the failure relates (usually Pod name)
	//+optional
	Node string `json:"node,omitempty"`
	// Detailed error message
	//+optional
	Message string `json:"message,omitempty"`
	// Time failure occurred or was discovered
	//+optional
	Time *metav1.Time `json:"time,omitempty"`
	// Exit status from the last termination of the container
	//+optional
	ExitCode int32 `json:"exitCode,omitempty"`
}

type ModelCopies struct {
	// How many copies of this predictor's models failed to load recently
	// +kubebuilder:default=0
	FailedCopies int `json:"failedCopies"`
	// Total number copies of this predictor's models that are currently loaded
	// +optional
	TotalCopies int `json:"totalCopies,omitempty"`
}
