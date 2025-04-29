/*
Copyright 2025 The KServe Authors.

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
	knativeapis "knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// DistributedInferenceServiceStatus defines the observed state of DistributedInferenceService.
type DistributedInferenceServiceStatus struct {
	// Conditions for DistributedInferenceService
	duckv1.Status `json:",inline"`
	// Addressable endpoint in the cluster for the DistributedInferenceService
	// +optional
	Address *duckv1.Addressable `json:"address,omitempty"`
	// URL holds the url that will distribute traffic over the provided traffic targets.
	// It generally has the form http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}
	// +optional
	URL *knativeapis.URL `json:"url,omitempty"`
	// Replicas indicates the current number of ready InferenceService
	Replicas int32 `json:"replicas,omitempty"`
	// Store current version for distributed inferenceService
	CurrentVersion int `json:"currentVersion,omitempty"`
	// Store prev version for distributed inferenceService
	// +optional
	PrevVersion int `json:"prevVersion,omitempty"`
	// DesiredReplicas specifies the target number of InferenceService replicas during scaling.
	// When no scaling is in progress, it should match the actual number of replicas.
	DesiredReplicas int32 `json:"desiredReplicas,omitempty"`
}
