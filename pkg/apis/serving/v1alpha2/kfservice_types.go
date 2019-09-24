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
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

// KFServiceSpec defines the desired state of KFService
type KFServiceSpec struct {
	// Default defines default KFService endpoints
	// +required
	Default EndpointSpec `json:"default"`
	// Canary defines an alternate endpoints to route a percentage of traffic.
	// +optional
	Canary *EndpointSpec `json:"canary,omitempty"`
	// CanaryTrafficPercent defines the percentage of traffic going to canary KFService endpoints
	// +optional
	CanaryTrafficPercent int `json:"canaryTrafficPercent,omitempty"`
}

type EndpointSpec struct {
	// Predictor defines the model serving spec
	// +required
	Predictor PredictorSpec `json:"predictor"`

	// Explainer defines the model explanation service spec
	// explainer service calls to transformer or predictor service
	// +optional
	Explainer *ExplainerSpec `json:"explainer,omitempty"`

	// Transformer defines the transformer service spec for pre/post processing
	// transformer service calls to predictor service
	// +optional
	Transformer *TransformerSpec `json:"transformer,omitempty"`
}

// DeploymentSpec defines the configuration for a given KFService service component
type DeploymentSpec struct {
	// ServiceAccountName is the name of the ServiceAccount to use to run the service
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Minimum number of replicas, pods won't scale down to 0 in case of no traffic
	// +optional
	MinReplicas int `json:"minReplicas,omitempty"`
	// This is the up bound for autoscaler to scale to
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`
}

// PredictorSpec defines the configuration to route traffic to a predictor.
type PredictorSpec struct {
	// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
	Custom     *CustomSpec     `json:"custom,omitempty"`
	Tensorflow *TensorflowSpec `json:"tensorflow,omitempty"`
	TensorRT   *TensorRTSpec   `json:"tensorrt,omitempty"`
	XGBoost    *XGBoostSpec    `json:"xgboost,omitempty"`
	SKLearn    *SKLearnSpec    `json:"sklearn,omitempty"`
	ONNX       *ONNXSpec       `json:"onnx,omitempty"`
	PyTorch    *PyTorchSpec    `json:"pytorch,omitempty"`

	DeploymentSpec `json:",inline"`
}

// ExplainerSpec defines the arguments for a model explanation server
type ExplainerSpec struct {
	// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
	Alibi  *AlibiExplainerSpec `json:"alibi,omitempty"`
	Custom *CustomSpec         `json:"custom,omitempty"`

	DeploymentSpec `json:",inline"`
}

// TransformerSpec defines transformer service for pre/post processing
type TransformerSpec struct {
	Custom *CustomSpec `json:"custom,omitempty"`

	DeploymentSpec `json:",inline"`
}

type AlibiExplainerType string

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
	// Defaults to latest Alibi Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
}

// TensorflowSpec defines arguments for configuring Tensorflow model serving.
type TensorflowSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Defaults to latest TF Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// TensorRTSpec defines arguments for configuring TensorRT model serving.
type TensorRTSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Defaults to latest TensorRT Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// XGBoostSpec defines arguments for configuring XGBoost model serving.
type XGBoostSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Defaults to latest XGBoost Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// SKLearnSpec defines arguments for configuring SKLearn model serving.
type SKLearnSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Defaults to latest SKLearn Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// ONNXSpec defines arguments for configuring ONNX model serving.
type ONNXSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Defaults to latest ONNX Version.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// PyTorchSpec defines arguments for configuring PyTorch model serving.
type PyTorchSpec struct {
	// The location of the trained model
	StorageURI string `json:"storageUri"`
	// Defaults PyTorch model class name to 'PyTorchModel'
	ModelClassName string `json:"modelClassName,omitempty"`
	// Defaults to latest PyTorch Version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// CustomSpec provides a hook for arbitrary container configuration.
type CustomSpec struct {
	Container v1.Container `json:"container"`
}

// StatusConfigurationSpec describes the state of the configuration receiving traffic.
type StatusConfigurationSpec struct {
	Name     string `json:"name,omitempty"`
	Hostname string `json:"host,omitempty"`
	Replicas int    `json:"replicas,omitempty"`
	Traffic  int    `json:"traffic,omitempty"`
}

// KFServiceStatus defines the observed state of KFService
// TODO (rakelkar) okay to depend on constants from types?
type EndpointStatusMap map[constants.KFServiceEndpoint]*StatusConfigurationSpec

type KFServiceStatus struct {
	duckv1beta1.Status `json:",inline"`
	URL                string            `json:"Url,omitempty"`
	Default            EndpointStatusMap `json:"default,omitempty"`
	Canary             EndpointStatusMap `json:"canary,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KFService is the Schema for the services API
// +k8s:openapi-gen=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
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

//  VirtualServiceStatus captures the status of the virtual service
type VirtualServiceStatus struct {
	URL           string
	CanaryWeight  int
	DefaultWeight int

	duckv1beta1.Status
}

func init() {
	SchemeBuilder.Register(&KFService{}, &KFServiceList{})
}
