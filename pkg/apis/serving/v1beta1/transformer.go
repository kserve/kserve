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

// Constants
const (
	ExactlyOneTransformerViolatedError = "Exactly one of [Custom] must be specified in TransformerSpec"
)

// TransformerSpec defines transformer service for pre/post processing
type TransformerSpec struct {
	// Pass through Pod fields or specify a custom container spec
	*CustomTransformer `json:",inline"`
	// Extensions available in all components
	ComponentExtensionSpec `json:",inline"`
}

func (t *TransformerSpec) GetTransformer() []Component {
	transformers := []Component{}
	if t.CustomTransformer != nil {
		transformers = append(transformers, t.CustomTransformer)
	}
	return transformers
}

// Validate the TransformerSpec
func (t *TransformerSpec) Validate() error {
	transformer := t.GetTransformer()[0]
	for _, err := range []error{
		transformer.Validate(),
		validateStorageURI(transformer.GetStorageUri()),
		validateContainerConcurrency(t.ContainerConcurrency),
		validateReplicas(t.MinReplicas, t.MaxReplicas),
		validateLogger(t.LoggerSpec),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}
