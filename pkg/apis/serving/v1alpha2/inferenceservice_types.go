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
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

// InferenceServiceSpec defines the desired state of InferenceService
type InferenceServiceSpec struct {
	// Default defines default InferenceService endpoints
	// +required
	Default EndpointSpec `json:"default"`
	// Canary defines alternate endpoints to route a percentage of traffic.
	// +optional
	Canary *EndpointSpec `json:"canary,omitempty"`
	// CanaryTrafficPercent defines the percentage of traffic going to canary InferenceService endpoints
	// +optional
	CanaryTrafficPercent *int `json:"canaryTrafficPercent,omitempty"`
}

type EndpointSpec struct {
	// Predictor defines the model serving spec
	// +required
	Predictor PredictorSpec `json:"predictor"`

	// Explainer defines the model explanation service spec,
	// explainer service calls to predictor or transformer if it is specified.
	// +optional
	Explainer *ExplainerSpec `json:"explainer,omitempty"`

	// Transformer defines the pre/post processing before and after the predictor call,
	// transformer service calls to predictor service.
	// +optional
	Transformer *TransformerSpec `json:"transformer,omitempty"`
}

// DeploymentSpec defines the configuration for a given InferenceService service component
type DeploymentSpec struct {
	// ServiceAccountName is the name of the ServiceAccount to use to run the service
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Minimum number of replicas which defaults to 1, when minReplicas = 0 pods scale down to 0 in case of no traffic
	// +optional
	MinReplicas *int `json:"minReplicas,omitempty"`
	// This is the up bound for autoscaler to scale to
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`
	// Parallelism specifies how many requests can be processed concurrently, this sets the hard limit of the container
	// concurrency(https://knative.dev/docs/serving/autoscaling/concurrency).
	Parallelism int `json:"parallelism,omitempty"`
	// Activate request/response logging
	// +optional
	Logger *Logger `json:"logger,omitempty"`
	// Activate request batching
	// +optional
	Batcher *Batcher `json:"batcher,omitempty"`
}

type LoggerMode string

const (
	LogAll      LoggerMode = "all"
	LogRequest  LoggerMode = "request"
	LogResponse LoggerMode = "response"
)

// Logger provides optional payload logging for all endpoints
// +experimental
type Logger struct {
	// URL to send request logging CloudEvents
	// +optional
	Url *string `json:"url,omitempty"`
	// What payloads to log: [all, request, response]
	Mode LoggerMode `json:"mode,omitempty"`
}

// Batcher provides optional payload batcher for all endpoints
// +experimental
type Batcher struct {
	// MaxBatchSize of batcher service
	// +optional
	MaxBatchSize *int `json:"maxBatchSize,omitempty"`
	// MaxLatency of batcher service
	// +optional
	MaxLatency *int `json:"maxLatency,omitempty"`
	// Timeout of batcher service
	// +optional
	Timeout *int `json:"timeout,omitempty"`
}

// PredictorSpec defines the configuration for a predictor,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type PredictorSpec struct {
	// Spec for a custom predictor
	Custom *CustomSpec `json:"custom,omitempty"`
	// Spec for Tensorflow Serving (https://github.com/tensorflow/serving)
	Tensorflow *TensorflowSpec `json:"tensorflow,omitempty"`
	// Spec for Triton Inference Server (https://github.com/triton-inference-server/server)
	Triton *TritonSpec `json:"triton,omitempty"`
	// Spec for XGBoost predictor
	XGBoost *XGBoostSpec `json:"xgboost,omitempty"`
	// Spec for LightGBM predictor
	LightGBM *LightGBMSpec `json:"lightgbm,omitempty"`
	// Spec for SKLearn predictor
	SKLearn *SKLearnSpec `json:"sklearn,omitempty"`
	// Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)
	ONNX *ONNXSpec `json:"onnx,omitempty"`
	// Spec for PyTorch predictor
	PyTorch *PyTorchSpec `json:"pytorch,omitempty"`
	// Spec for PMML predictor
	PMML *PMMLSpec `json:"pmml,omitempty"`

	DeploymentSpec `json:",inline"`
}

// ExplainerSpec defines the arguments for a model explanation server,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type ExplainerSpec struct {
	// Spec for alibi explainer
	Alibi *AlibiExplainerSpec `json:"alibi,omitempty"`
	// Spec for AIX explainer
	AIX *AIXExplainerSpec `json:"aix,omitempty"`
	// Spec for a custom explainer
	Custom *CustomSpec `json:"custom,omitempty"`

	DeploymentSpec `json:",inline"`
}

// TransformerSpec defines transformer service for pre/post processing
type TransformerSpec struct {
	// Spec for a custom transformer
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
	// Alibi docker image version which defaults to latest release
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
}

type AIXExplainerType string

const (
	AIXLimeImageExplainer AIXExplainerType = "LimeImages"
)

// AIXExplainerSpec defines the arguments for configuring an AIX Explanation Server
type AIXExplainerSpec struct {
	// The type of AIX explainer
	Type AIXExplainerType `json:"type"`
	// The location of a trained explanation model
	StorageURI string `json:"storageUri,omitempty"`
	// Defaults to latest AIX Version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
}

// TensorflowSpec defines arguments for configuring Tensorflow model serving.
type TensorflowSpec struct {
	// The URI for the saved model(https://www.tensorflow.org/tutorials/keras/save_and_load)
	StorageURI string `json:"storageUri"`
	// TFServing docker image version(https://hub.docker.com/r/tensorflow/serving), default version can be set in the inferenceservice configmap.
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// TritonSpec defines arguments for configuring Triton Inference Server.
type TritonSpec struct {
	// The URI for the trained model repository(https://docs.nvidia.com/deeplearning/triton-inference-server/master-user-guide/docs/model_repository.html)
	StorageURI string `json:"storageUri"`
	// Triton Inference Server docker image version, default version can be set in the inferenceservice configmap
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// XGBoostSpec defines arguments for configuring XGBoost model serving.
type XGBoostSpec struct {
	// The URI of the trained model which contains model.bst
	StorageURI string `json:"storageUri"`
	// Number of thread to be used by XGBoost
	NThread int `json:"nthread,omitempty"`
	// XGBoost KFServer docker image version which defaults to latest release
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// LightGBMSpec defines arguments for configuring LightGBM model serving.
type LightGBMSpec struct {
	// The URI of the trained model which contains model.bst
	StorageURI string `json:"storageUri"`
	// Number of thread to be used by LightGBM
	NThread int `json:"nthread,omitempty"`
	// LightGBM KFServer docker image version which defaults to latest release
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// SKLearnSpec defines arguments for configuring SKLearn model serving.
type SKLearnSpec struct {
	// The URI of the trained model which contains model.pickle, model.pkl or model.joblib
	StorageURI string `json:"storageUri"`
	// SKLearn KFServer docker image version which defaults to latest release
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// ONNXSpec defines arguments for configuring ONNX model serving.
type ONNXSpec struct {
	// The URI of the exported onnx model(model.onnx)
	StorageURI string `json:"storageUri"`
	// ONNXRuntime docker image versions, default version can be set in the inferenceservice configmap
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// PyTorchSpec defines arguments for configuring PyTorch model serving.
type PyTorchSpec struct {
	// The URI of the trained model which contains model.pt
	StorageURI string `json:"storageUri"`
	// Defaults PyTorch model class name to 'PyTorchModel'
	ModelClassName string `json:"modelClassName,omitempty"`
	// PyTorch KFServer docker image version which defaults to latest release
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// PMMLSpec defines arguments for configuring PMML model serving.
type PMMLSpec struct {
	// The URI of the trained model which contains model.pmml
	StorageURI string `json:"storageUri"`
	// PMML KFServer docker image version which defaults to latest release
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// CustomSpec provides a hook for arbitrary container configuration.
type CustomSpec struct {
	Container v1.Container `json:"container"`
}

// EndpointStatusMap defines the observed state of InferenceService endpoints
type ComponentStatusMap *map[constants.InferenceServiceComponent]StatusConfigurationSpec

// InferenceServiceStatus defines the observed state of InferenceService
type InferenceServiceStatus struct {
	duckv1beta1.Status `json:",inline"`
	// URL of the InferenceService
	URL string `json:"url,omitempty"`
	// Traffic percentage that goes to default services
	Traffic int `json:"traffic,omitempty"`
	// Traffic percentage that goes to canary services
	CanaryTraffic int `json:"canaryTraffic,omitempty"`
	// Statuses for the default endpoints of the InferenceService
	Default ComponentStatusMap `json:"default,omitempty"`
	// Statuses for the canary endpoints of the InferenceService
	Canary ComponentStatusMap `json:"canary,omitempty"`
	// Addressable URL for eventing
	Address *duckv1beta1.Addressable `json:"address,omitempty"`
}

// StatusConfigurationSpec describes the state of the configuration receiving traffic.
type StatusConfigurationSpec struct {
	// Latest revision name that is in ready state
	Name string `json:"name,omitempty"`
	// Host name of the service
	Hostname string `json:"host,omitempty"`
}

// InferenceService is the Schema for the services API
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Default Traffic",type="integer",JSONPath=".status.traffic"
// +kubebuilder:printcolumn:name="Canary Traffic",type="integer",JSONPath=".status.canaryTraffic"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=inferenceservices,shortName=isvc
type InferenceService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InferenceServiceSpec   `json:"spec,omitempty"`
	Status InferenceServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// InferenceServiceList contains a list of Service
type InferenceServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []InferenceService `json:"items"`
}

// +k8s:openapi-gen=false
//  VirtualServiceStatus captures the status of the virtual service
type VirtualServiceStatus struct {
	URL           string
	CanaryWeight  int
	DefaultWeight int
	// Address holds the information needed for a Route to be the target of an event.
	// +optional
	Address *duckv1beta1.Addressable `json:"address,omitempty"`

	duckv1beta1.Status
}

func init() {
	SchemeBuilder.Register(&InferenceService{}, &InferenceServiceList{})
}
