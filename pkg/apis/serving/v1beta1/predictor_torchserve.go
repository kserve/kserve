/*

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
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultPyTorchModelClassName     = "PyTorchModel"
	PyTorchServingGPUSuffix          = "-gpu"
	InvalidPyTorchRuntimeIncludesGPU = "PyTorch RuntimeVersion is not GPU enabled but GPU resources are requested. "
	InvalidPyTorchRuntimeExcludesGPU = "PyTorch RuntimeVersion is GPU enabled but GPU resources are not requested. "
	InvalidInferenceProtocolVersion  = "PyTorch ProtocolVersion v2 is not supported"
)

// TorchServeSpec defines arguments for configuring PyTorch model serving.
type TorchServeSpec struct {
	// When this field is specified KFS chooses the KFServer implementation, otherwise KFS uses the TorchServe implementation
	// +optional
	ModelClassName string `json:"modelClassName,omitempty"`
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
		t.validateProtocol(),
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

func (t *TorchServeSpec) validateProtocol() error {
	if t.ProtocolVersion != nil && *t.ProtocolVersion == constants.ProtocolV2 {
		return fmt.Errorf("Invalid inference protocol version")
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
		if t.ProtocolVersion != nil && *t.ProtocolVersion == constants.ProtocolV2 {
			if utils.IsGPUEnabled(t.Resources) {
				t.RuntimeVersion = proto.String(config.Predictors.PyTorch.V2.DefaultGpuImageVersion)
			} else {
				t.RuntimeVersion = proto.String(config.Predictors.PyTorch.V2.DefaultImageVersion)
			}
		} else {
			if t.ModelClassName != "" {
				if utils.IsGPUEnabled(t.Resources) {
					t.RuntimeVersion = proto.String(config.Predictors.PyTorch.V1.DefaultGpuImageVersion)
				} else {
					t.RuntimeVersion = proto.String(config.Predictors.PyTorch.V1.DefaultImageVersion)
				}
			} else {
				if utils.IsGPUEnabled(t.Resources) {
					t.RuntimeVersion = proto.String(config.Predictors.PyTorch.V2.DefaultGpuImageVersion)
				} else {
					t.RuntimeVersion = proto.String(config.Predictors.PyTorch.V2.DefaultImageVersion)
				}
			}
		}
	}

	setResourceRequirementDefaults(&t.Resources)
}

// GetContainers transforms the resource into a container spec
func (t *TorchServeSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	if t.ProtocolVersion == nil || *t.ProtocolVersion == constants.ProtocolV1 {
		if t.ModelClassName != "" {
			return t.GetContainerV1(metadata, extensions, config)
		} else {
			return t.GetContainerV2(metadata, extensions, config)
		}
	}
	return t.GetContainerV2(metadata, extensions, config)
}

func (t *TorchServeSpec) GetContainerV1(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, metadata.Name),
		fmt.Sprintf("%s=%s", constants.ArgumentModelClassName, t.ModelClassName),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
	}
	if utils.IsGPUEnabled(t.Resources) {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, "1"))
	} else if extensions.ContainerConcurrency != nil {
		if !utils.IncludesArg(t.Container.Args, constants.ArgumentWorkers) {
			arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10)))
		}
	}
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.PyTorch.V1.ContainerImage + ":" + *t.RuntimeVersion
	}
	t.Name = constants.InferenceServiceContainerName
	t.Args = append(arguments, t.Args...)
	return &t.Container
}

func (t *TorchServeSpec) GetContainerV2(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		"torchserve",
		"--start",
		fmt.Sprintf("%s=%s", "--model-store", constants.DefaultModelLocalMountPath+"/model-store"),
		fmt.Sprintf("%s=%s", "--ts-config", constants.DefaultModelLocalMountPath+"/config/config.properties"),
	}
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.PyTorch.V2.ContainerImage + ":" + *t.RuntimeVersion
	}
	t.Name = constants.InferenceServiceContainerName
	t.Args = append(arguments, t.Args...)
	return &t.Container
}

func (t *TorchServeSpec) GetStorageUri() *string {
	return t.StorageURI
}

func (t *TorchServeSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (t *TorchServeSpec) IsMMS(config *InferenceServicesConfig) bool {
	predictorConfig := t.getPredictorConfig(config)
	return predictorConfig.MultiModelServer
}

func (t *TorchServeSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	predictorConfig := t.getPredictorConfig(config)
	supportedFrameworks := predictorConfig.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}

func (t *TorchServeSpec) getPredictorConfig(config *InferenceServicesConfig) *PredictorConfig {
	protocol := t.GetProtocol()
	if protocol == constants.ProtocolV1 {
		return config.Predictors.PyTorch.V1
	} else {
		return config.Predictors.PyTorch.V2
	}
}
