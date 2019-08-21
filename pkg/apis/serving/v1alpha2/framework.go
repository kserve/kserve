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

type FrameworkHandler interface {
	GetModelSourceUri() string
	CreateModelServingContainer(modelName string, config *FrameworksConfig) *v1.Container
	ApplyDefaults()
	Validate() error
}

const (
	// ExactlyOneModelSpecViolatedError is a known error message
	ExactlyOneModelSpecViolatedError = "Exactly one of [Custom, ONNX, Tensorflow, TensorRT, SKLearn, XGBoost] must be specified in ModelSpec"
	// AtLeastOneModelSpecViolatedError is a known error message
	AtLeastOneModelSpecViolatedError = "At least one of [Custom, ONNX, Tensorflow, TensorRT, SKLearn, XGBoost] must be specified in ModelSpec"
)

var (
	DefaultMemoryRequests = resource.MustParse("2Gi")
	DefaultCPURequests    = resource.MustParse("1")
)

// Returns a URI to the model. This URI is passed to the model-initializer via the ModelInitializerSourceUriInternalAnnotationKey
func (m *ModelSpec) GetModelSourceUri() string {
	return getHandler(m).GetModelSourceUri()
}

func (m *ModelSpec) CreateModelServingContainer(modelName string, config *FrameworksConfig) *v1.Container {
	return getHandler(m).CreateModelServingContainer(modelName, config)
}

func (m *ModelSpec) ApplyDefaults() {
	getHandler(m).ApplyDefaults()
}

func (m *ModelSpec) Validate() error {
	handler, err := makeHandler(m)
	if err != nil {
		return err
	}
	return handler.Validate()
}

type FrameworkConfig struct {
	ContainerImage string `json:"image"`

	//TODO add readiness/liveness probe config
}
type FrameworksConfig struct {
	Tensorflow FrameworkConfig `json:"tensorflow,omitempty"`
	TensorRT   FrameworkConfig `json:"tensorrt,omitempty"`
	Xgboost    FrameworkConfig `json:"xgboost,omitempty"`
	SKlearn    FrameworkConfig `json:"sklearn,omitempty"`
	PyTorch    FrameworkConfig `json:"pytorch,omitempty"`
	ONNX       FrameworkConfig `json:"onnx,omitempty"`
}

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}

	if _, ok := requirements.Requests[v1.ResourceCPU]; !ok {
		requirements.Requests[v1.ResourceCPU] = DefaultCPURequests
	}
	if _, ok := requirements.Requests[v1.ResourceMemory]; !ok {
		requirements.Requests[v1.ResourceMemory] = DefaultMemoryRequests
	}
}

func isGPUEnabled(requirements v1.ResourceRequirements) bool {
	_, ok := requirements.Limits[constants.NvidiaGPUResourceType]
	return ok
}

func getHandler(modelSpec *ModelSpec) FrameworkHandler {
	handler, err := makeHandler(modelSpec)
	if err != nil {
		klog.Fatal(err)
	}

	return handler
}

func makeHandler(modelSpec *ModelSpec) (FrameworkHandler, error) {
	handlers := []FrameworkHandler{}
	if modelSpec.Custom != nil {
		handlers = append(handlers, modelSpec.Custom)
	}
	if modelSpec.XGBoost != nil {
		handlers = append(handlers, modelSpec.XGBoost)
	}
	if modelSpec.SKLearn != nil {
		handlers = append(handlers, modelSpec.SKLearn)
	}
	if modelSpec.Tensorflow != nil {
		handlers = append(handlers, modelSpec.Tensorflow)
	}
	if modelSpec.ONNX != nil {
		handlers = append(handlers, modelSpec.ONNX)
	}
	if modelSpec.PyTorch != nil {
		handlers = append(handlers, modelSpec.PyTorch)
	}
	if modelSpec.TensorRT != nil {
		handlers = append(handlers, modelSpec.TensorRT)
	}
	if len(handlers) == 0 {
		return nil, fmt.Errorf(AtLeastOneModelSpecViolatedError)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOneModelSpecViolatedError)
	}
	return handlers[0], nil
}
