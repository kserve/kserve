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

type LocalModelNodeStatus struct {
	// Status of each local model
	ModelStatus map[string]ModelStatus `json:"modelStatus,omitempty"`
}

// ModelStatus enum
// +kubebuilder:validation:Enum="";ModelDownloadPending;ModelDownloading;ModelDownloaded;ModelDownloadError
type ModelStatus string

// ModelStatus Enum values
const (
	ModelDownloadPending ModelStatus = "ModelDownloadPending"
	ModelDownloading     ModelStatus = "ModelDownloading"
	ModelDownloaded      ModelStatus = "ModelDownloaded"
	ModelDownloadError   ModelStatus = "ModelDownloadError"
)
