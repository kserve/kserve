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
)

// SKLearnSpec defines arguments for configuring SKLearn model serving.
type SKLearnSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &SKLearnSpec{}

// Default sets defaults on the resource
func (k *SKLearnSpec) Default(config *InferenceServicesConfig) {
	k.Container.Name = constants.InferenceServiceContainerName

	if k.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		k.ProtocolVersion = &defaultProtocol
	}

	setResourceRequirementDefaults(config, &k.Resources)
}

func (k *SKLearnSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container {
	return &k.Container
}

func (k *SKLearnSpec) GetProtocol() constants.InferenceServiceProtocol {
	if k.ProtocolVersion != nil {
		return *k.ProtocolVersion
	} else {
		return constants.ProtocolV1
	}
}
