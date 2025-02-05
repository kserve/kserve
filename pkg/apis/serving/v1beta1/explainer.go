/*
Copyright 2021 The KServe Authors.

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
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/utils"
)

// ExplainerSpec defines the container spec for a model explanation server,
// The following fields follow a "1-of" semantic. Users must specify exactly one spec.
type ExplainerSpec struct {
	// Spec for ART explainer
	ART *ARTExplainerSpec `json:"art,omitempty"`
	// This spec is dual purpose.
	// 1) Users may choose to provide a full PodSpec for their custom explainer.
	// The field PodSpec.Containers is mutually exclusive with other explainers.
	// 2) Users may choose to provide a Explainer and specify PodSpec
	// overrides in the PodSpec. They must not provide PodSpec.Containers in this case.
	PodSpec `json:",inline"`
	// Component extension defines the deployment configurations for explainer
	ComponentExtensionSpec `json:",inline"`
}

// ExplainerExtensionSpec defines configuration shared across all explainer frameworks
type ExplainerExtensionSpec struct {
	// The location of a trained explanation model
	StorageURI string `json:"storageUri,omitempty"`
	// Defaults to latest Explainer Version
	RuntimeVersion *string `json:"runtimeVersion,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
	// Container enables overrides for the predictor.
	// Each framework will have different defaults that are populated in the underlying container spec.
	// +optional
	corev1.Container `json:",inline"`
	// Storage Spec for model location
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`
}

var _ Component = &ExplainerSpec{}

// Validate returns an error if invalid
func (e *ExplainerExtensionSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageSpec(e.GetStorageSpec(), e.GetStorageUri()),
	})
}

// GetStorageUri returns the predictor storage Uri
func (e *ExplainerExtensionSpec) GetStorageUri() *string {
	if e.StorageURI != "" {
		return &e.StorageURI
	}
	return nil
}

// GetStorageSpec returns the predictor storage spec object
func (e *ExplainerExtensionSpec) GetStorageSpec() *StorageSpec {
	return e.Storage
}

// GetImplementations returns the implementations for the component
func (s *ExplainerSpec) GetImplementations() []ComponentImplementation {
	implementations := NonNilComponents([]ComponentImplementation{
		s.ART,
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
