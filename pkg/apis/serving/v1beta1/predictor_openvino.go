/*
Copyright 2026 The KServe Authors.

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
)

// OpenVINOSpec defines arguments for configuring OpenVINO model serving.
type OpenVINOSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &OpenVINOSpec{}

// Default sets defaults on the resource
func (o *OpenVINOSpec) Default(config *InferenceServicesConfig) {
	o.Name = constants.InferenceServiceContainerName

	if o.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV2
		o.ProtocolVersion = &defaultProtocol
	}

	setResourceRequirementDefaults(config, &o.Resources)
}

func (o *OpenVINOSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container {
	return &o.Container
}

func (o *OpenVINOSpec) GetProtocol() constants.InferenceServiceProtocol {
	if o.ProtocolVersion != nil {
		return *o.ProtocolVersion
	}
	return constants.ProtocolV2
}
