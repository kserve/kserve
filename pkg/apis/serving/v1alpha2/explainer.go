/*
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

// +kubebuilder:object:generate=false
type Explainer interface {
	GetResourceRequirements() *v1.ResourceRequirements
	GetStorageUri() string
	CreateExplainerContainer(modelName string, parallelism int, predictorHost string, config *InferenceServicesConfig) *v1.Container
	ApplyDefaults(config *InferenceServicesConfig)
	Validate(config *InferenceServicesConfig) error
}

const (
	// ExactlyOneExplainerViolatedError is a known error message
	ExactlyOneExplainerViolatedError = "Exactly one of [Custom, Alibi, AIX] must be specified in ExplainerSpec"
)

// Returns a URI to the explainer. This URI is passed to the model-initializer via the ModelInitializerSourceUriInternalAnnotationKey
func (e *ExplainerSpec) GetStorageUri() string {
	explainer, err := getExplainer(e)
	if err != nil {
		return ""
	}
	return explainer.GetStorageUri()
}

func (e *ExplainerSpec) CreateExplainerContainer(modelName string, parallelism int, predictorHost string, config *InferenceServicesConfig) *v1.Container {
	explainer, err := getExplainer(e)
	if err != nil {
		return nil
	}
	return explainer.CreateExplainerContainer(modelName, parallelism, predictorHost, config)
}

func (e *ExplainerSpec) ApplyDefaults(config *InferenceServicesConfig) {
	explainer, err := getExplainer(e)
	if err == nil {
		explainer.ApplyDefaults(config)
	}
}

func (e *ExplainerSpec) Validate(config *InferenceServicesConfig) error {
	explainer, err := getExplainer(e)
	if err != nil {
		return err
	}
	for _, err := range []error{
		explainer.Validate(config),
		validateStorageURI(e.GetStorageUri()),
		validateParallelism(e.Parallelism),
		validateReplicas(e.MinReplicas, e.MaxReplicas),
		validateLogger(e.Logger),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

func getExplainer(explainerSpec *ExplainerSpec) (Explainer, error) {
	handlers := []Explainer{}
	if explainerSpec.Custom != nil {
		handlers = append(handlers, explainerSpec.Custom)
	}
	if explainerSpec.Alibi != nil {
		handlers = append(handlers, explainerSpec.Alibi)
	}
	if explainerSpec.AIX != nil {
		handlers = append(handlers, explainerSpec.AIX)
	}
	if len(handlers) != 1 {
		err := fmt.Errorf(ExactlyOneExplainerViolatedError)
		klog.Error(err)
		return nil, err
	}
	return handlers[0], nil
}
