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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// HuggingFaceRuntimeSpec defines arguments for configuring HuggingFace model serving.
type HuggingFaceRuntimeSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &HuggingFaceRuntimeSpec{}

// Validate returns an error if invalid
func (o *HuggingFaceRuntimeSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageSpec(o.GetStorageSpec(), o.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (o *HuggingFaceRuntimeSpec) Default(config *InferenceServicesConfig) {
	o.Container.Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(config, &o.Resources)
}

// GetContainer transforms the resource into a container spec
func (o *HuggingFaceRuntimeSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container {
	return &o.Container
}

func (o *HuggingFaceRuntimeSpec) GetProtocol() constants.InferenceServiceProtocol {
	if o.ProtocolVersion != nil {
		return *o.ProtocolVersion
	}
	return constants.ProtocolV2
}
