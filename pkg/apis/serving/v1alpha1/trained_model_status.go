/*
Copyright 2020 kubeflow.org.

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
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// TrainedModelStatus defines the observed state of TrainedModel
type TrainedModelStatus struct {
	// Conditions for trained model
	duckv1.Status `json:",inline"`
	// URL holds the url that will distribute traffic over the provided traffic targets.
	// For v1: http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}/v1/models/<trainedmodel>:predict
	// For v2: http[s]://{route-name}.{route-namespace}.{cluster-level-suffix}/v2/models/<trainedmodel>/infer
	URL *apis.URL `json:"url,omitempty"`
	// Addressable endpoint for the deployed trained model
	// http://<inferenceservice.metadata.name>/v1/models/<trainedmodel>.metadata.name
	Address *duckv1.Addressable `json:"address,omitempty"`
}
