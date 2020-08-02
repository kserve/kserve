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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Constants
const (
	ExactlyOneTransformerViolatedError = "Exactly one of [Custom] must be specified in TransformerSpec"
)

// TransformerSpec defines transformer service for pre/post processing
type TransformerSpec struct {
	// Pass through Pod fields or specify a custom container spec
	*CustomTransformer `json:",inline"`
	// Extensions available in all components
	*ComponentExtensionSpec `json:",inline"`
}

// Transformer interface is implemented by all Transformers
// +kubebuilder:object:generate=false
type Transformer interface {
	GetContainer(metadata metav1.ObjectMeta) *v1.Container
	GetStorageUri() *string
	Default()
	Validate() error
}

func (t *TransformerSpec) GetTransformer() (Transformer, error) {
	transformers := []Transformer{}
	if t.CustomTransformer != nil {
		transformers = append(transformers, t.CustomTransformer)
	}
	// Fail if not exactly one
	if len(transformers) != 1 {
		err := fmt.Errorf(ExactlyOneTransformerViolatedError)
		return nil, err
	}
	return transformers[0], nil
}

// Validate the TransformerSpec
func (t *TransformerSpec) Validate(config *InferenceServicesConfig) error {
	transformer, err := t.GetTransformer()
	if err != nil {
		return err
	}
	for _, err := range []error{
		validateContainerConcurrency(t.ContainerConcurrency),
		validateReplicas(t.MinReplicas, t.MaxReplicas),
		transformer.Validate(),
		validateLogger(t.LoggerSpec),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

// Apply Defaults to the TransformerSpec
func (t *TransformerSpec) Default(config *InferenceServicesConfig) {
	transformer, err := t.GetTransformer()
	if err == nil {
		transformer.Default()
	}
}
