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
	"fmt"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

type Predictor interface {
	GetStorageUri() string
	GetContainer(modelName string, config *PredictorsConfig) *v1.Container
	ApplyDefaults()
	Validate() error
}

const (
	// ExactlyOnePredictorViolatedError is a known error message
	ExactlyOnePredictorViolatedError = "Exactly one of [Custom, ONNX, Tensorflow, TensorRT, SKLearn, XGBoost] must be specified in ModelSpec"
)

var (
	DefaultMemory = resource.MustParse("2Gi")
	DefaultCPU    = resource.MustParse("1")
)

// Returns a URI to the model. This URI is passed to the storage-initializer via the StorageInitializerSourceUriInternalAnnotationKey
func (m *PredictorSpec) GetStorageUri() string {
	return getPredictor(m).GetStorageUri()
}

func (m *PredictorSpec) GetContainer(modelName string, config *PredictorsConfig) *v1.Container {
	return getPredictor(m).GetContainer(modelName, config)
}

func (m *PredictorSpec) ApplyDefaults() {
	getPredictor(m).ApplyDefaults()
}

func (m *PredictorSpec) Validate() error {
	handler, err := makeHandler(m)
	if err != nil {
		return err
	}
	return handler.Validate()
}

type PredictorConfig struct {
	ContainerImage string `json:"image"`

	//TODO add readiness/liveness probe config
}
type PredictorsConfig struct {
	Tensorflow PredictorConfig `json:"tensorflow,omitempty"`
	TensorRT   PredictorConfig `json:"tensorrt,omitempty"`
	Xgboost    PredictorConfig `json:"xgboost,omitempty"`
	SKlearn    PredictorConfig `json:"sklearn,omitempty"`
	PyTorch    PredictorConfig `json:"pytorch,omitempty"`
	ONNX       PredictorConfig `json:"onnx,omitempty"`
}

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}

	if _, ok := requirements.Requests[v1.ResourceCPU]; !ok {
		requirements.Requests[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Requests[v1.ResourceMemory]; !ok {
		requirements.Requests[v1.ResourceMemory] = DefaultMemory
	}

	if requirements.Limits == nil {
		requirements.Limits = v1.ResourceList{}
	}

	if _, ok := requirements.Limits[v1.ResourceCPU]; !ok {
		requirements.Limits[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Limits[v1.ResourceMemory]; !ok {
		requirements.Limits[v1.ResourceMemory] = DefaultMemory
	}
}

func isGPUEnabled(requirements v1.ResourceRequirements) bool {
	_, ok := requirements.Limits[constants.NvidiaGPUResourceType]
	return ok
}

func getPredictor(modelSpec *PredictorSpec) Predictor {
	handler, err := makeHandler(modelSpec)
	if err != nil {
		klog.Fatal(err)
	}

	return handler
}

func makeHandler(predictorSpec *PredictorSpec) (Predictor, error) {
	handlers := []Predictor{}
	if predictorSpec.Custom != nil {
		handlers = append(handlers, predictorSpec.Custom)
	}
	if predictorSpec.XGBoost != nil {
		handlers = append(handlers, predictorSpec.XGBoost)
	}
	if predictorSpec.SKLearn != nil {
		handlers = append(handlers, predictorSpec.SKLearn)
	}
	if predictorSpec.Tensorflow != nil {
		handlers = append(handlers, predictorSpec.Tensorflow)
	}
	if predictorSpec.ONNX != nil {
		handlers = append(handlers, predictorSpec.ONNX)
	}
	if predictorSpec.PyTorch != nil {
		handlers = append(handlers, predictorSpec.PyTorch)
	}
	if predictorSpec.TensorRT != nil {
		handlers = append(handlers, predictorSpec.TensorRT)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOnePredictorViolatedError)
	}
	return handlers[0], nil
}
