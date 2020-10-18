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

// ExplainerSpec defines the container spec for a model explanation server,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type ExplainerSpec struct {
	// Spec for alibi explainer
	Alibi *AlibiExplainerSpec `json:"alibi,omitempty"`
	// Spec for AIX explainer
	AIX *AIXExplainerSpec `json:"aix,omitempty"`
	// This spec is dual purpose.
	// 1) Users may choose to provide a full PodSpec for their custom explainer.
	// The field PodSpec.Containers is mutually exclusive with other explainers (i.e. Alibi).
	// 2) Users may choose to provide a Explainer (i.e. Alibi) and specify PodSpec
	// overrides in the PodSpec. They must not provide PodSpec.Containers in this case.
	PodSpec `json:",inline"`
	// Extensions available in all components
	ComponentExtensionSpec `json:",inline"`
}

var _ Component = &ExplainerSpec{}

// GetImplementations returns the implementations for the component
func (s *ExplainerSpec) GetImplementations() []ComponentImplementation {
	implementations := NonNilComponents([]ComponentImplementation{
		s.Alibi,
		s.AIX,
	})
	// This struct is not a pointer, so it will never be nil; include if containers are specified
	if len(s.PodSpec.Containers) != 0 {
		implementations = append(implementations, NewCustomExplainer(&s.PodSpec))
	}
	return implementations
}

// GetImplementation returns the implementation for the component
func (s *ExplainerSpec) GetImplementation() ComponentImplementation {
	return s.GetImplementations()[0]
}

// GetExtensions returns the extensions for the component
func (s *ExplainerSpec) GetExtensions() *ComponentExtensionSpec {
	return &s.ComponentExtensionSpec
}
