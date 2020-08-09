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
	"fmt"
	v1 "k8s.io/api/core/v1"
)

const (
	// ExactlyOnePredictorViolatedError is a known error message
	ExactlyOnePredictorViolatedError = "Exactly one of [Custom, ONNX, Tensorflow, Triton, SKLearn, XGBoost] must be specified in PredictorSpec"
)

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
	// Spec for Triton Inference Server (https://github.com/NVIDIA/triton-inference-server)
	Triton *TritonSpec `json:"triton,omitempty"`
	// Spec for ONNX runtime (https://github.com/microsoft/onnxruntime)
	ONNX *ONNXRuntimeSpec `json:"onnx,omitempty"`
	// Passthrough Pod fields or specify a custom container spec
	*CustomPredictor `json:",inline"`
	// Extensions available in all components
	ComponentExtensionSpec `json:",inline"`
}

var _ Component = &PredictorSpec{}

// PredictorExtensionSpec defines configuration shared across all predictor frameworks
type PredictorExtensionSpec struct {
	// This field points to the location of the trained model which is mounted onto the pod.
	StorageURI *string `json:"storageUri"`
	// Runtime version of the predictor docker image
	// +optional
	RuntimeVersion *string `json:"runtimeVersion,omitempty"`
	// Container enables overrides for the predictor.
	// Each framework will have different defaults that are populated in the underlying container spec.
	// +optional
	v1.Container `json:",inline"`
}

// GetPredictor returns the framework for the Predictor
func (p *PredictorSpec) GetPredictor() (Component, error) {
	predictors := []Component{}
	if p.CustomPredictor != nil {
		predictors = append(predictors, p.CustomPredictor)
	}
	if p.XGBoost != nil {
		predictors = append(predictors, p.XGBoost)
	}
	if p.SKLearn != nil {
		predictors = append(predictors, p.SKLearn)
	}
	if p.Tensorflow != nil {
		predictors = append(predictors, p.Tensorflow)
	}
	if p.ONNX != nil {
		predictors = append(predictors, p.ONNX)
	}
	if p.PyTorch != nil {
		predictors = append(predictors, p.PyTorch)
	}
	if p.Triton != nil {
		predictors = append(predictors, p.Triton)
	}
	if len(predictors) != 1 {
		err := fmt.Errorf(ExactlyOnePredictorViolatedError)
		return nil, err
	}
	return predictors[0], nil
}

// GetPredictorPodSpec returns the PodSpec for the Predictor
func (p *PredictorSpec) GetPredictorPodSpec() v1.PodSpec {
	return p.CustomPredictor.Spec
}

func (p *PredictorSpec) Validate() error {
	predictor, err := p.GetPredictor()
	if err != nil {
		return err
	}
	for _, err := range []error{
		predictor.Validate(),
		validateStorageURI(predictor.GetStorageUri()),
		validateContainerConcurrency(p.ContainerConcurrency),
		validateReplicas(p.MinReplicas, p.MaxReplicas),
		validateLogger(p.LoggerSpec),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}
