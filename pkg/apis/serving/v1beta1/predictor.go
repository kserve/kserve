/*
Copyright 2021 The KServe Authors.

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
	"reflect"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

// PredictorImplementation defines common functions for all predictors e.g Tensorflow, Triton, etc
// +kubebuilder:object:generate=false
type PredictorImplementation interface {
}

// PredictorSpec defines the configuration for a predictor,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type PredictorSpec struct {
	// Spec for SKLearn model server
	SKLearn *SKLearnSpec `json:"sklearn,omitempty"`
	// Spec for XGBoost model server
	XGBoost *XGBoostSpec `json:"xgboost,omitempty"`
	// Spec for TFServing (https://github.com/tensorflow/serving)
	Tensorflow *TFServingSpec `json:"tensorflow,omitempty"`
	// Spec for TorchServe (https://pytorch.org/serve)
	PyTorch *TorchServeSpec `json:"pytorch,omitempty"`
	// Spec for Triton Inference Server (https://github.com/triton-inference-server/server)
	Triton *TritonSpec `json:"triton,omitempty"`
	// Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)
	ONNX *ONNXRuntimeSpec `json:"onnx,omitempty"`
	// Spec for PMML (http://dmg.org/pmml/v4-1/GeneralStructure.html)
	PMML *PMMLSpec `json:"pmml,omitempty"`
	// Spec for LightGBM model server
	LightGBM *LightGBMSpec `json:"lightgbm,omitempty"`
	// Spec for Paddle model server (https://github.com/PaddlePaddle/Serving)
	Paddle *PaddleServerSpec `json:"paddle,omitempty"`

	// Model spec for any arbitrary framework.
	Model *ModelSpec `json:"model,omitempty"`

	// This spec is dual purpose. <br />
	// 1) Provide a full PodSpec for custom predictor.
	// The field PodSpec.Containers is mutually exclusive with other predictors (i.e. TFServing). <br />
	// 2) Provide a predictor (i.e. TFServing) and specify PodSpec
	// overrides, you must not provide PodSpec.Containers in this case. <br />
	PodSpec `json:",inline"`
	// Component extension defines the deployment configurations for a predictor
	ComponentExtensionSpec `json:",inline"`
}

var _ Component = &PredictorSpec{}

// PredictorExtensionSpec defines configuration shared across all predictor frameworks
type PredictorExtensionSpec struct {
	// This field points to the location of the trained model which is mounted onto the pod.
	// +optional
	StorageURI *string `json:"storageUri,omitempty"`
	// Runtime version of the predictor docker image
	// +optional
	RuntimeVersion *string `json:"runtimeVersion,omitempty"`
	// Protocol version to use by the predictor (i.e. v1 or v2 or grpc-v1 or grpc-v2)
	// +optional
	ProtocolVersion *constants.InferenceServiceProtocol `json:"protocolVersion,omitempty"`
	// Container enables overrides for the predictor.
	// Each framework will have different defaults that are populated in the underlying container spec.
	// +optional
	v1.Container `json:",inline"`
	// Storage Spec for model location
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`
}

type StorageSpec struct {
	// The path to the model object in the storage. It cannot co-exist
	// with the storageURI.
	// +optional
	Path *string `json:"path,omitempty"`
	// The path to the model schema file in the storage.
	// +optional
	SchemaPath *string `json:"schemaPath,omitempty"`
	// Parameters to override the default storage credentials and config.
	// +optional
	Parameters *map[string]string `json:"parameters,omitempty"`
	// The Storage Key in the secret for this model.
	// +optional
	StorageKey *string `json:"key,omitempty"`
}

// GetImplementations returns the implementations for the component
func (s *PredictorSpec) GetImplementations() []ComponentImplementation {
	implementations := NonNilComponents([]ComponentImplementation{
		s.XGBoost,
		s.PyTorch,
		s.Triton,
		s.SKLearn,
		s.Tensorflow,
		s.ONNX,
		s.PMML,
		s.LightGBM,
		s.Paddle,
		s.Model,
	})
	// This struct is not a pointer, so it will never be nil; include if containers are specified
	if len(s.PodSpec.Containers) != 0 {
		implementations = append(implementations, NewCustomPredictor(&s.PodSpec))
	}

	return implementations
}

// GetImplementation returns the implementation for the component
func (s *PredictorSpec) GetImplementation() ComponentImplementation {
	return s.GetImplementations()[0]
}

// GetExtensions returns the extensions for the component
func (s *PredictorSpec) GetExtensions() *ComponentExtensionSpec {
	return &s.ComponentExtensionSpec
}

// Validate returns an error if invalid
func (p *PredictorExtensionSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(p.GetStorageUri()),
		// TODO: Re-enable storage spec validation once azure/gcs are supported.
		// Enabling this currently prevents those storage types from working with ModelMesh.
		// validateStorageSpec(p.GetStorageSpec(), p.GetStorageUri()),
	})
}

// GetStorageUri returns the predictor storage Uri
func (p *PredictorExtensionSpec) GetStorageUri() *string {
	return p.StorageURI
}

// GetStorageSpec returns the predictor storage spec object
func (p *PredictorExtensionSpec) GetStorageSpec() *StorageSpec {
	return p.Storage
}

// GetPredictorImplementations GetPredictor returns the implementation for the predictor
func (s *PredictorSpec) GetPredictorImplementations() []ComponentImplementation {
	implementations := NonNilPredictors([]ComponentImplementation{
		s.XGBoost,
		s.PyTorch,
		s.Triton,
		s.SKLearn,
		s.Tensorflow,
		s.ONNX,
		s.PMML,
		s.LightGBM,
		s.Paddle,
		s.Model,
	})
	// This struct is not a pointer, so it will never be nil; include if containers are specified
	if len(s.PodSpec.Containers) != 0 {
		implementations = append(implementations, NewCustomPredictor(&s.PodSpec))
	}
	return implementations
}

func (s *PredictorSpec) GetPredictorImplementation() *ComponentImplementation {
	predictors := s.GetPredictorImplementations()
	if len(predictors) == 0 {
		return nil
	}
	return &s.GetPredictorImplementations()[0]
}

func NonNilPredictors(objects []ComponentImplementation) (results []ComponentImplementation) {
	for _, object := range objects {
		if !reflect.ValueOf(object).IsNil() {
			results = append(results, object)
		}
	}
	return results
}
