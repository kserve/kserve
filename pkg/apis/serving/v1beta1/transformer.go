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

// TransformerSpec defines transformer service for pre/post processing
type TransformerSpec struct {
	// This spec is dual purpose.
	// 1) Users may choose to provide a full PodSpec for their transformer.
	// The field PodSpec.Containers is mutually exclusive with other Transformer (i.e. Feast).
	// 2) Users may choose to provide a Transformer (i.e. Feast) and specify PodSpec
	// overrides in the CustomTransformer PodSpec. They must not provide PodSpec.Containers in this case.
	PodSpec `json:",inline"`
	// Extensions available in all components
	ComponentExtensionSpec `json:",inline"`
}

// GetImplementations returns the implementations for the component
func (s *TransformerSpec) GetImplementations() []ComponentImplementation {
	implementations := []ComponentImplementation{}
	// This struct is not a pointer, so it will never be nil; include if containers are specified
	if len(s.PodSpec.Containers) != 0 {
		implementations = append(implementations, NewCustomTransformer(&s.PodSpec))
	}
	return implementations
}

// GetImplementation returns the implementation for the component
func (s *TransformerSpec) GetImplementation() ComponentImplementation {
	return s.GetImplementations()[0]
}

// GetExtensions returns the extensions for the component
func (s *TransformerSpec) GetExtensions() *ComponentExtensionSpec {
	return &s.ComponentExtensionSpec
}
