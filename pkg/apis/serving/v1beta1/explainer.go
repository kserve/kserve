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
)

const (
	// ExactlyOneExplainerViolatedError is a known error message
	ExactlyOneExplainerViolatedError = "Exactly one of [Custom, Alibi] must be specified in ExplainerSpec"
)

// ExplainerSpec defines the container spec for a model explanation server,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type ExplainerSpec struct {
	// Spec for alibi explainer
	Alibi *AlibiExplainerSpec `json:"alibi,omitempty"`
	// Pass through Pod fields or specify a custom container spec
	*CustomExplainer `json:",inline"`
	// Extensions available in all components
	ComponentExtensionSpec `json:",inline"`
}

// Returns a URI to the explainer. This URI is passed to the model-initializer via the ModelInitializerSourceUriInternalAnnotationKey
func (e *ExplainerSpec) GetStorageUri() *string {
	explainer, err := e.GetExplainer()
	if err != nil {
		return nil
	}
	return explainer.GetStorageUri()
}

func (e *ExplainerSpec) Validate() error {
	explainer, err := e.GetExplainer()
	if err != nil {
		return err
	}
	for _, err := range []error{
		explainer.Validate(),
		validateStorageURI(e.GetStorageUri()),
		validateContainerConcurrency(e.ContainerConcurrency),
		validateReplicas(e.MinReplicas, e.MaxReplicas),
		validateLogger(e.LoggerSpec),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *ExplainerSpec) GetExplainer() (Component, error) {
	handlers := []Component{}
	if e.CustomExplainer != nil {
		handlers = append(handlers, e.CustomExplainer)
	}
	if e.Alibi != nil {
		handlers = append(handlers, e.Alibi)
	}
	if len(handlers) != 1 {
		err := fmt.Errorf(ExactlyOneExplainerViolatedError)
		return nil, err
	}
	return handlers[0], nil
}
