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
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

// InferenceRouterStatus defines the observed state of resource
type InferenceRouterStatus struct {
	duckv1.Status `json:",inline"`
	// Addressable endpoint for the router
	Address *duckv1.Addressable `json:"address,omitempty"`
}
