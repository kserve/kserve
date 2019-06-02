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

package v1alpha1

import (
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

type FrameworkHandler interface {
	CreateModelServingContainer(modelName string, config *FrameworksConfig) *v1.Container
	ApplyDefaults()
	Validate() error
}

const (
	// ExactlyOneModelSpecViolatedError is a known error message
	ExactlyOneModelSpecViolatedError = "Exactly one of [Custom, Tensorflow, SKLearn, XGBoost] must be specified in ModelSpec"
	// AtLeastOneModelSpecViolatedError is a known error message
	AtLeastOneModelSpecViolatedError = "At least one of [Custom, Tensorflow, SKLearn, XGBoost] must be specified in ModelSpec"
)

var (
	DefaultMemoryRequests = resource.MustParse("2Gi")
	DefaultCPURequests    = resource.MustParse("1")
)

func (m *ModelSpec) CreateModelServingContainer(modelName string, config *FrameworksConfig) *v1.Container {
	return getHandler(m).CreateModelServingContainer(modelName, config)
}

func (m *ModelSpec) ApplyDefaults() {
	getHandler(m).ApplyDefaults()
}

func (m *ModelSpec) Validate() error {
	_, err := makeHandler(m)
	return err
}

type FrameworkConfig struct {
	ContainerImage string `json:"image"`

	//TODO add readiness/liveness probe config
}
type FrameworksConfig struct {
	Tensorflow FrameworkConfig `json:"tensorflow,omitempty"`
	Xgboost    FrameworkConfig `json:"xgboost,omitempty"`
	SKlearn    FrameworkConfig `json:"sklearn,omitempty"`
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

func getHandler(modelSpec *ModelSpec) FrameworkHandler {
	handler, err := makeHandler(modelSpec)
	if err != nil {
		log.Fatal(err)
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
	if len(handlers) == 0 {
		return nil, fmt.Errorf(AtLeastOneModelSpecViolatedError)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOneModelSpecViolatedError)
	}
	return handlers[0], nil
}
