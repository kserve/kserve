/*
Copyright 2019 kubeflow.org.
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

package v1alpha2

import (
	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KFServiceSpec defines the desired state of KFService
type KFServiceSpec struct {
	// Default defines the configuration of KFService
	Default ServicesSpec `json:"default"`
	// Canary defines an alternate KFService configuration to route a percentage of traffic.
	Canary *ServicesSpec `json:"canary,omitempty"`
	// Traffic split percentage between default and canary KFService
	CanaryTrafficPercent int `json:"canaryTrafficPercent,omitempty"`
}

type ServicesSpec struct {
	// Optional prepost processing specification
	Prepostprocess *PrepostprocessSpec `json:"prepostprocess,omitempty"`
	// Predictor for a given model
	Predict ModelSpec `json:"predict"`
	// Optional Explain specification to add a model explainer next to the chosen predictor.
	Explain *ExplainSpec `json:"explain,omitempty"`
	// Default deployment specification
	Deployment *DeploymentSpec `json:"deployment,omitempty"`
	// Service account name
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

type DeploymentSpec struct {
	// Minimum number of replicas, pods won't scale down to 0 in case of no traffic
	MinReplicas int `json:"minReplicas,omitempty"`
	// This is the up bound for autoscaler to scale to
	MaxReplicas int `json:"maxReplicas,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// ModelSpec defines the configuration to route traffic to a predictor.
type ModelSpec struct {
	// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
	Custom     *CustomSpec     `json:"custom,omitempty"`
	Tensorflow *TensorflowSpec `json:"tensorflow,omitempty"`
	TensorRT   *TensorRTSpec   `json:"tensorrt,omitempty"`
	XGBoost    *XGBoostSpec    `json:"xgboost,omitempty"`
	SKLearn    *SKLearnSpec    `json:"sklearn,omitempty"`
	PyTorch    *PyTorchSpec    `json:"pytorch,omitempty"`
}

type PrepostprocessSpec struct {
	// Custom container
	Custom *CustomSpec `json:"custom,omitempty"`
}

// ExplainSpec defines the arguments for a model explanation server
type ExplainSpec struct {
	// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
	Alibi  *AlibiExplainSpec `json:"alibi,omitempty"`
	Custom *CustomSpec       `json:"custom,omitempty"`
}

type AlibiExplainerType string

const (
	AlibiAnchorsTabularExplainer  AlibiExplainerType = "AnchorsTabular"
	AlibiAnchorsImageExplainer    AlibiExplainerType = "AnchorsImage"
	AlibiAnchorsTextExplainer     AlibiExplainerType = "AnchorsText"
	AlibiCounterfactualsExplainer AlibiExplainerType = "Counterfactuals"
	AlibiContrastiveExplainer     AlibiExplainerType = "Contrastive"
)

// AlibiExplainSpec defines the arguments for configuring an Alibi Explanation Server
type AlibiExplainSpec struct {
	// The type of Alibi explainer
	Type AlibiExplainerType `json:"type"`
	// The location of a trained explanation model
	StorageURI string `json:"storageUri,omitempty"`
	// Defaults to latest Alibi Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
}

// TensorflowSpec defines arguments for configuring Tensorflow model serving.
type TensorflowSpec struct {
	// Model storage URI
	ModelURI string `json:"modelUri"`
	// Defaults to latest TF Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Deployment if specified it overwrites at upper level
	DeploymentSpec `json:",inline"`
}

// TensorRTSpec defines arguments for configuring TensorRT model serving.
type TensorRTSpec struct {
	// Model storage URI
	ModelURI string `json:"modelUri"`
	// Defaults to latest TensorRT Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Deployment if specified it overwrites at upper level
	DeploymentSpec `json:",inline"`
}

// XGBoostSpec defines arguments for configuring XGBoost model serving.
type XGBoostSpec struct {
	// Model storage URI
	ModelURI string `json:"modelUri"`
	// Defaults to latest XGBoost Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Deployment if specified it overwrites at upper level
	DeploymentSpec `json:",inline"`
}

// SKLearnSpec defines arguments for configuring SKLearn model serving.
type SKLearnSpec struct {
	// Model storage URI
	ModelURI string `json:"modelUri"`
	// Defaults to latest SKLearn Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Deployment if specified it overwrites at upper level
	DeploymentSpec `json:",inline"`
}

// PyTorchSpec defines arguments for configuring PyTorch model serving.
type PyTorchSpec struct {
	// Model storage URI
	ModelURI string `json:"modelUri"`
	// Defaults PyTorch model class name to 'PyTorchModel'
	ModelClassName string `json:"modelClassName,omitempty"`
	// Defaults to latest PyTorch Version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Deployment if specified it overwrites at upper level
	DeploymentSpec `json:",inline"`
}

// CustomSpec provides a hook for arbitrary container configuration.
type CustomSpec struct {
	// Custom container
	Container v1.Container `json:"container"`
	// Deployment if specified it overwrites at upper level
	DeploymentSpec `json:",inline"`
}

// KFServiceStatus defines the observed state of KFService
type KFServiceStatus struct {
	duckv1beta1.Status `json:",inline"`
	URL                *apis.URL               `json:"url,omitempty"`
	Default            StatusConfigurationSpec `json:"default,omitempty"`
	Canary             StatusConfigurationSpec `json:"canary,omitempty"`
}

// StatusConfigurationSpec describes the state of the configuration receiving traffic.
type StatusConfigurationSpec struct {
	Name     string `json:"name,omitempty"`
	Replicas int    `json:"replicas,omitempty"`
	Traffic  int    `json:"traffic,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KFService is the Schema for the services API
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Default Traffic",type="integer",JSONPath=".status.default.traffic"
// +kubebuilder:printcolumn:name="Canary Traffic",type="integer",JSONPath=".status.canary.traffic"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=kfservices,shortName=kfservice
type KFService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KFServiceSpec   `json:"spec,omitempty"`
	Status KFServiceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KFServiceList contains a list of Service
type KFServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KFService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KFService{}, &KFServiceList{})
}
