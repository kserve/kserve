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
	"errors"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var (
	TensorflowEntrypointCommand         = "/usr/bin/tensorflow_model_server"
	TensorflowServingGRPCPort           = "9000"
	TensorflowServingRestPort           = "8080"
	TensorflowServingGPUSuffix          = "-gpu"
	InvalidTensorflowRuntimeIncludesGPU = "Tensorflow RuntimeVersion is not GPU enabled but GPU resources are requested"
	InvalidTensorflowRuntimeExcludesGPU = "Tensorflow RuntimeVersion is GPU enabled but GPU resources are not requested"
)

// TFServingSpec defines arguments for configuring Tensorflow model serving.
type TFServingSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &TFServingSpec{}

// Validate returns an error if invalid
func (t *TFServingSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		t.validateGPU(),
		validateStorageSpec(t.GetStorageSpec(), t.GetStorageUri()),
	})
}

func (t *TFServingSpec) validateGPU() error {
	if t.RuntimeVersion == nil {
		return nil
	}
	if utils.IsGPUEnabled(t.Resources) && !strings.Contains(*t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return errors.New(InvalidTensorflowRuntimeIncludesGPU)
	}

	if !utils.IsGPUEnabled(t.Resources) && strings.Contains(*t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return errors.New(InvalidTensorflowRuntimeExcludesGPU)
	}
	return nil
}

// Default sets defaults on the resource
func (t *TFServingSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(config, &t.Resources)
}

func (t *TFServingSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container {
	return &t.Container
}

func (t *TFServingSpec) GetProtocol() constants.InferenceServiceProtocol {
	if t.ProtocolVersion != nil {
		return *t.ProtocolVersion
	}
	return constants.ProtocolV1
}
