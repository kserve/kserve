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

// XGBoostSpec defines arguments for configuring XGBoost model serving.
type XGBoostSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &XGBoostSpec{}

// Default sets defaults on the resource
func (x *XGBoostSpec) Default(config *InferenceServicesConfig) {
	x.Container.Name = constants.InferenceServiceContainerName

	if x.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		x.ProtocolVersion = &defaultProtocol
	}

	setResourceRequirementDefaults(config, &x.Resources)
}

func (x *XGBoostSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container {
	return &x.Container
}

func (x *XGBoostSpec) GetProtocol() constants.InferenceServiceProtocol {
	if x.ProtocolVersion != nil {
		return *x.ProtocolVersion
	} else {
		return constants.ProtocolV1
	}
}
