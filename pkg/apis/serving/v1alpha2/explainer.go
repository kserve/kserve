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

type Explainer interface {
	GetStorageUri() string
	CreateExplainerContainer(modelName string, predictorHost string, config *ExplainersConfig) *v1.Container
	ApplyExplainerDefaults(config *ExplainersConfig)
	ValidateExplainer(config *ExplainersConfig) error
}

const (
	// ExactlyOneExplainerViolatedError is a known error message
	ExactlyOneExplainerViolatedError = "Exactly one of [Custom, Alibi] must be specified in ExplainerSpec"
)

// Returns a URI to the explainer. This URI is passed to the model-initializer via the ModelInitializerSourceUriInternalAnnotationKey
func (e *ExplainerSpec) GetStorageUri() string {
	explainer, err := getExplainer(e)
	if err != nil {
		return ""
	}
	return explainer.GetStorageUri()
}

func (e *ExplainerSpec) CreateExplainerContainer(modelName string, predictorHost string, config *ExplainersConfig) *v1.Container {
	explainer, err := getExplainer(e)
	if err != nil {
		return nil
	}
	return explainer.CreateExplainerContainer(modelName, predictorHost, config)
}

func (e *ExplainerSpec) ApplyDefaults(config *ExplainersConfig) {
	explainer, err := getExplainer(e)
	if err == nil {
		explainer.ApplyExplainerDefaults(config)
	}
}

func (e *ExplainerSpec) Validate(config *ExplainersConfig) error {
	explainer, err := getExplainer(e)
	if err != nil {
		return err
	}
	if err := explainer.ValidateExplainer(config); err != nil {
		return err
	}
	if err := validateStorageURI(e.GetStorageUri()); err != nil {
		return err
	}
	return nil
}

// +k8s:openapi-gen=false
type ExplainerConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion string `json:"defaultImageVersion"`

	AllowedImageVersions []string `json:"allowedImageVersions"`
}

// +k8s:openapi-gen=false
type ExplainersConfig struct {
	AlibiExplainer ExplainerConfig `json:"alibi,omitempty"`
}

func getExplainer(explainerSpec *ExplainerSpec) (Explainer, error) {
	handlers := []Explainer{}
	if explainerSpec.Custom != nil {
		handlers = append(handlers, explainerSpec.Custom)
	}
	if explainerSpec.Alibi != nil {
		handlers = append(handlers, explainerSpec.Alibi)
	}
	if len(handlers) != 1 {
		err := fmt.Errorf(ExactlyOneExplainerViolatedError)
		klog.Error(err)
		return nil, err
	}
	return handlers[0], nil
}
