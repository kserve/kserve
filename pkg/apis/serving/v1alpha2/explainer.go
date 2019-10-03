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

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
)

type ExplainerHandler interface {
	GetStorageUri() string
	CreateExplainerServingContainer(modelName string, predictorHost string, config *ExplainersConfig) *v1.Container
	ApplyDefaults()
	Validate() error
}

const (
	// ExactlyOnePredictorViolatedError is a known error message
	ExactlyOneExplainerSpecViolatedError = "Exactly one of [Custom, Alibi] must be specified in ExplainerSpec"
)

// Returns a URI to the explainer. This URI is passed to the model-initializer via the ModelInitializerSourceUriInternalAnnotationKey
func (m *ExplainerSpec) GetStorageUri() string {
	return getExplainerHandler(m).GetStorageUri()
}

func (m *ExplainerSpec) CreateExplainerServingContainer(modelName string, predictorHost string, config *ExplainersConfig) *v1.Container {
	return getExplainerHandler(m).CreateExplainerServingContainer(modelName, predictorHost, config)
}

func (m *ExplainerSpec) ApplyDefaults() {
	getExplainerHandler(m).ApplyDefaults()
}

func (m *ExplainerSpec) Validate() error {
	handler, err := makeExplainerHandler(m)
	if err != nil {
		return err
	}
	return handler.Validate()
}

type ExplainerConfig struct {
	ContainerImage string `json:"image"`

	//TODO add readiness/liveness probe config
}
type ExplainersConfig struct {
	AlibiExplainer ExplainerConfig `json:"alibi,omitempty"`
}

func getExplainerHandler(modelSpec *ExplainerSpec) ExplainerHandler {
	handler, err := makeExplainerHandler(modelSpec)
	if err != nil {
		klog.Fatal(err)
	}

	return handler
}

func makeExplainerHandler(explainerSpec *ExplainerSpec) (ExplainerHandler, error) {
	handlers := []ExplainerHandler{}
	if explainerSpec.Custom != nil {
		handlers = append(handlers, explainerSpec.Custom)
	}
	if explainerSpec.Alibi != nil {
		handlers = append(handlers, explainerSpec.Alibi)
	}
	if len(handlers) != 1 {
		return nil, fmt.Errorf(ExactlyOneExplainerSpecViolatedError)
	}
	return handlers[0], nil
}
