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
	"reflect"
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

var _ Component = &ExplainerSpec{}

func (e *ExplainerSpec) Validate() error {
	explainer := e.GetExplainer()[0]
	for _, err := range []error{
		explainer.Validate(),
		validateStorageURI(explainer.GetStorageUri()),
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

func (e *ExplainerSpec) GetExplainer() []Component {
	explainers := []Component{}
	for _, explainer := range []Component{
		e.Alibi,
		e.CustomExplainer,
	} {
		if !reflect.ValueOf(explainer).IsNil() {
			explainers = append(explainers, explainer)
		}
	}
	return explainers
}
