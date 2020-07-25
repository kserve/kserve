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

package v1beta1

import v1 "k8s.io/api/core/v1"

// AlibiExplainerType is the explanation method
type AlibiExplainerType string

// AlibiExplainerType Enum
const (
	AlibiAnchorsTabularExplainer  AlibiExplainerType = "AnchorTabular"
	AlibiAnchorsImageExplainer    AlibiExplainerType = "AnchorImages"
	AlibiAnchorsTextExplainer     AlibiExplainerType = "AnchorText"
	AlibiCounterfactualsExplainer AlibiExplainerType = "Counterfactuals"
	AlibiContrastiveExplainer     AlibiExplainerType = "Contrastive"
)

// AlibiExplainerSpec defines the arguments for configuring an Alibi Explanation Server
type AlibiExplainerSpec struct {
	// The type of Alibi explainer
	Type AlibiExplainerType `json:"type"`
	// The location of a trained explanation model
	StorageURI string `json:"storageUri,omitempty"`
	// Defaults to latest Alibi Version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
}
