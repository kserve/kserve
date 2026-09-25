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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type LocalModelCacheStatus struct {
	// Status of the model on a node, like NodeDownloaded or NodeNotReady
	NodeStatus map[string]NodeStatus `json:"nodeStatus,omitempty"`

	// How many nodes have the model available locally
	// +optional
	ModelCopies *ModelCopies `json:"copies,omitempty"`
	// Inference services using this local model
	InferenceServices []NamespacedName `json:"inferenceServices,omitempty"`
	// LLM inference services using this local model
	LLMInferenceServices []NamespacedName `json:"llmInferenceServices,omitempty"`

	// Conditions describes the observed state of the cache. For shared-PVC mode
	// (LocalModelNamespaceCache with pvcRef) a positive-polarity Ready condition is the
	// stable readiness contract for serving admission and downstream consumers.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// SharedPVCImport records the last successful shared-PVC import. It is set only for a
	// LocalModelNamespaceCache in shared-PVC mode and is the durable evidence that the
	// destination was published as Ready, so the controller does not start a new import
	// writer against data that active consumers still read once the retained Job is gone.
	// +optional
	SharedPVCImport *SharedPVCImportStatus `json:"sharedPVCImport,omitempty"`
}

// SharedPVCImportStatus describes a completed shared-PVC import.
type SharedPVCImportStatus struct {
	// PVCUID is the UID of the referenced PersistentVolumeClaim the model was imported onto.
	// A claim recreated under the same name has a different UID and is imported again.
	PVCUID types.UID `json:"pvcUID"`
	// CompletionTime is when the import Job completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +k8s:openapi-gen=true
type NamespacedName struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// NodeStatus enum
// +kubebuilder:validation:Enum="";NodeNotReady;NodeDownloadPending;NodeDownloading;NodeDownloaded;NodeDownloadError;
type NodeStatus string

// NodeStatus Enum values
const (
	NodeNotReady        NodeStatus = "NodeNotReady"
	NodeDownloadPending NodeStatus = "NodeDownloadPending"
	NodeDownloading     NodeStatus = "NodeDownloading"
	NodeDownloaded      NodeStatus = "NodeDownloaded"
	NodeDownloadError   NodeStatus = "NodeDownloadError"
)

type ModelCopies struct {
	Available int `json:"available,omitempty"`
	// Total number of nodes that we expect the model to be downloaded. Including nodes that are not ready
	Total int `json:"total,omitempty"`
	// Download Failed
	Failed int `json:"failed,omitempty"`
}
