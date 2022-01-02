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
	v1 "k8s.io/api/core/v1"
)

// PredictorImplementation defines common functions for all predictors e.g Tensorflow, Triton, etc
// +kubebuilder:object:generate=false
type PredictorImplementation interface {
	IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool
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
	// Protocol version to use by the predictor (i.e. v1 or v2)
	// +optional
	ProtocolVersion *constants.InferenceServiceProtocol `json:"protocolVersion,omitempty"`
	// Container enables overrides for the predictor.
	// Each framework will have different defaults that are populated in the underlying container spec.
	// +optional
	v1.Container `json:",inline"`
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

// GetPredictor returns the implementation for the predictor
func (s *PredictorSpec) GetPredictorImplementations() []PredictorImplementation {
	implementations := NonNilPredictors([]PredictorImplementation{
		s.XGBoost,
		s.PyTorch,
		s.Triton,
		s.SKLearn,
		s.Tensorflow,
		s.ONNX,
		s.PMML,
		s.LightGBM,
		s.Paddle,
	})
	// This struct is not a pointer, so it will never be nil; include if containers are specified
	if len(s.PodSpec.Containers) != 0 {
		implementations = append(implementations, NewCustomPredictor(&s.PodSpec))
	}
	return implementations
}

func (s *PredictorSpec) GetPredictorImplementation() *PredictorImplementation {
	predictors := s.GetPredictorImplementations()
	if len(predictors) == 0 {
		return nil
	}
	return &s.GetPredictorImplementations()[0]
}

func NonNilPredictors(objects []PredictorImplementation) (results []PredictorImplementation) {
	for _, object := range objects {
		if !reflect.ValueOf(object).IsNil() {
			results = append(results, object)
		}
	}
	return results
}

func isFrameworkIncluded(supportedFrameworks []string, framework string) bool {
	for _, supportedFramework := range supportedFrameworks {
		if supportedFramework == framework {
			return true
		}
	}
	return false
}
