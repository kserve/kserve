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
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var (
	_ ComponentImplementation = &TorchServeSpec{}
	_ PredictorImplementation = &TorchServeSpec{}
)

// Validate returns an error if invalid
func (t *TorchServeSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(t.GetStorageUri()),
		t.validateGPU(),
		validateStorageSpec(t.GetStorageSpec(), t.GetStorageUri()),
	})
}

func (t *TorchServeSpec) validateGPU() error {
	if utils.IsGPUEnabled(t.Resources) && !strings.Contains(*t.RuntimeVersion, PyTorchServingGPUSuffix) {
		return fmt.Errorf(InvalidPyTorchRuntimeIncludesGPU)
	}

	if !utils.IsGPUEnabled(t.Resources) && strings.Contains(*t.RuntimeVersion, PyTorchServingGPUSuffix) {
		return fmt.Errorf(InvalidPyTorchRuntimeExcludesGPU)
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
	if t.RuntimeVersion == nil {
		if utils.IsGPUEnabled(t.Resources) {
			t.RuntimeVersion = proto.String(config.Predictors.PyTorch.DefaultGpuImageVersion)
		} else {
			t.RuntimeVersion = proto.String(config.Predictors.PyTorch.DefaultImageVersion)
		}
	}
	setResourceRequirementDefaults(&t.Resources)
}

// GetContainers transforms the resource into a container spec
func (t *TorchServeSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	var tsServiceEnvelope = ""

	if t.ProtocolVersion == nil || *t.ProtocolVersion == constants.ProtocolV1 {
		tsServiceEnvelope = V1ServiceEnvelope
	} else {
		tsServiceEnvelope = V2ServiceEnvelope
	}

	t.Container.Env = append(
		t.Container.Env,
		v1.EnvVar{
			Name:  "TS_SERVICE_ENVELOPE",
			Value: tsServiceEnvelope,
		},
	)

	arguments := []string{
		"torchserve",
		"--start",
		fmt.Sprintf("%s=%s", "--model-store", constants.DefaultModelLocalMountPath+"/model-store"),
		fmt.Sprintf("%s=%s", "--ts-config", constants.DefaultModelLocalMountPath+"/config/config.properties"),
	}

	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.PyTorch.ContainerImage + ":" + *t.RuntimeVersion
	}

	t.Name = constants.InferenceServiceContainerName
	t.Args = append(arguments, t.Args...)

	return &t.Container
}

func (t *TorchServeSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (t *TorchServeSpec) IsMMS(config *InferenceServicesConfig) bool {
	return config.Predictors.PyTorch.MultiModelServer
}

func (t *TorchServeSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	supportedFrameworks := config.Predictors.PyTorch.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}
