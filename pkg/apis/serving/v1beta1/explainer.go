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
	// This spec is dual purpose <br />
	// 1) Provide a full PodSpec for custom explainer.
	// The field PodSpec.Containers is mutually exclusive with other explainers (i.e. Alibi). <br />
	// 2) Provide a explainer (i.e. Alibi) and specify PodSpec
	// overrides, you must not provide PodSpec.Containers in this case. <br />
	PodSpec `json:",inline"`
	// Component extension defines the deployment configurations for explainer
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
