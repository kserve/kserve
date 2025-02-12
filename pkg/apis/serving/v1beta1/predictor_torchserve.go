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

const (
	PyTorchServingGPUSuffix          = "-gpu"
	InvalidPyTorchRuntimeIncludesGPU = "PyTorch RuntimeVersion is not GPU enabled but GPU resources are requested. "
	InvalidPyTorchRuntimeExcludesGPU = "PyTorch RuntimeVersion is GPU enabled but GPU resources are not requested. "
	V1ServiceEnvelope                = "kserve"
	V2ServiceEnvelope                = "kservev2"
)

// TorchServeSpec defines arguments for configuring PyTorch model serving.
type TorchServeSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &TorchServeSpec{}

// Validate returns an error if invalid
func (t *TorchServeSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		t.validateGPU(),
		validateStorageSpec(t.GetStorageSpec(), t.GetStorageUri()),
	})
}

func (t *TorchServeSpec) validateGPU() error {
	if t.RuntimeVersion == nil {
		return nil
	}
	if utils.IsGPUEnabled(t.Resources) && !strings.Contains(*t.RuntimeVersion, PyTorchServingGPUSuffix) {
		return errors.New(InvalidPyTorchRuntimeIncludesGPU)
	}

	if !utils.IsGPUEnabled(t.Resources) && strings.Contains(*t.RuntimeVersion, PyTorchServingGPUSuffix) {
		return errors.New(InvalidPyTorchRuntimeExcludesGPU)
	}
	return nil
}

// Default sets defaults on the resource
func (t *TorchServeSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
	if t.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		t.ProtocolVersion = &defaultProtocol
	}
	setResourceRequirementDefaults(config, &t.Resources)
}

func (t *TorchServeSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container {
	return &t.Container
}

func (t *TorchServeSpec) GetProtocol() constants.InferenceServiceProtocol {
	if t.ProtocolVersion != nil {
		return *t.ProtocolVersion
	}
	return constants.ProtocolV1
}
