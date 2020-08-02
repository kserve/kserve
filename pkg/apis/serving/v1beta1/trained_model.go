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

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrainedModel is the Schema for the TrainedModel API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".status.url"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=trainedmodel,shortName=tm
type TrainedModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TrainedModelSpec   `json:"spec,omitempty"`
	Status            TrainedModelStatus `json:"status,omitempty"`
}

// TrainedModelSpec defines the trained model spec
type TrainedModelSpec struct {
	// parent inference service to deploy to
	// +required
	InferenceService string `json:"inferenceService"`
	// Predictor model spec
	// +required
	PredictorModel ModelSpec `json:"predictorModel"`
}

// ModelSpec describes a trained model
type ModelSpec struct {
	// Storage URI for the model repository
	StorageURI string `json:"storageUri"`
	// Machine Learning framework which is used for trained model
	// Valid values are:
	// - "tensorflow";
	// - "xgboost";
	// - "sklearn";
	// - "pytorch";
	// - "onnx";
	// - "customframework";
	Framework string `json:"framework"`
	// Framework version for trained model
	// +optional
	FrameworkVersion string `json:"frameworkVersion,omitempty"`
	// Maximum memory this model will consume, this field is used to decide if a model server has enough memory to load this model.
	// +optional
	Memory resource.Quantity `json:"memory,omitempty"`
}

// TrainedModelList contains a list of TrainedModels
// +kubebuilder:object:root=true
type TrainedModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// +listType=set
	Items []TrainedModel `json:"items"`
}
