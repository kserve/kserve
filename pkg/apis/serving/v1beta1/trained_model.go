package v1beta1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

type TrainedModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TrainedModelSpec         `json:"spec,omitempty"`
	Status            TrainedModelDeployStatus `json:"status,omitempty"`
}
type TrainedModelSpec struct {
	// Required field for parent inference service
	InferenceService string `json:"inferenceService"`
	// Predictor model spec
	PredictorModel ModelSpec `json:"predictorModel"`
	// Explainer model spec
	ExplainerModel ModelSpec `json:"explainerModel,omitempty"`
}
type ModelSpec struct {
	// Storage URI for the model repository
	StorageURI string `json:"storageUri"`
	// Default to latest
	ModelVersionPolicy ModelVersionPolicy `json:"modelVersionPolicy,omitempty"`
	// ML framework name
	// The values could be: "tensorflow","pytorch","sklearn","onnx","xgboost", "custom", "myawesomeinternalframework" etc.
	Framework string `json:"framework"`
	// Framework version for the trained model
	FrameworkVersion string `json:"frameworkVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}
type ModelVersionPolicy struct {
	// +kubebuilder:webhooks:Enum={"all","latest","specific"}
	ModelVersionPolicy string `json:"modelVersionPolicy"`
	// A list of specific model versions to serve, it is required when model version policy is "specific"
	ModelVersions []string `json:"modelVersions,omitempty"`
}
type TrainedModelDeployStatus struct {
	// Condition for "Deployed"
	duckv1beta1.Status `json:",inline"`
	// Addressable endpoint for the deployed trained model
	// http://inferenceservice.metadata.name/v1/models/trainedmodel.metadata.name
	Address *duckv1beta1.Addressable `json:"address,omitempty"`
}
