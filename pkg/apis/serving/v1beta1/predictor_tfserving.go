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
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	TensorflowEntrypointCommand          = "/usr/bin/tensorflow_model_server"
	TensorflowServingGRPCPort            = "9000"
	TensorflowServingRestPort            = "8080"
	TensorflowServingGPUSuffix           = "-gpu"
	InvalidTensorflowRuntimeVersionError = "Tensorflow RuntimeVersion must be one of %s"
	InvalidTensorflowRuntimeIncludesGPU  = "Tensorflow RuntimeVersion is not GPU enabled but GPU resources are requested. " + InvalidTensorflowRuntimeVersionError
	InvalidTensorflowRuntimeExcludesGPU  = "Tensorflow RuntimeVersion is GPU enabled but GPU resources are not requested. " + InvalidTensorflowRuntimeVersionError
)

// TFServingSpec defines arguments for configuring Tensorflow model serving.
type TFServingSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var (
	_ ComponentImplementation = &TFServingSpec{}
	_ PredictorImplementation = &TFServingSpec{}
)

// Validate returns an error if invalid
func (t *TFServingSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(t.GetStorageUri()),
		t.validateGPU(),
		validateStorageSpec(t.GetStorageSpec(), t.GetStorageUri()),
	})
}

func (t *TFServingSpec) validateGPU() error {
	if utils.IsGPUEnabled(t.Resources) && !strings.Contains(*t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeIncludesGPU)
	}

	if !utils.IsGPUEnabled(t.Resources) && strings.Contains(*t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeExcludesGPU)
	}
	return nil
}

// Default sets defaults on the resource
func (t *TFServingSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
	if t.RuntimeVersion == nil {
		if utils.IsGPUEnabled(t.Resources) {
			t.RuntimeVersion = proto.String(config.Predictors.Tensorflow.DefaultGpuImageVersion)
		} else {
			t.RuntimeVersion = proto.String(config.Predictors.Tensorflow.DefaultImageVersion)
		}
	}
	setResourceRequirementDefaults(&t.Resources)
}

// GetContainers transforms the resource into a container spec
func (t *TFServingSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	// Get the timeout from user input or else use the default timeout
	TimeoutMilliSeconds := 1000 * config.Predictors.Tensorflow.DefaultTimeout
	if extensions.TimeoutSeconds != nil {
		TimeoutMilliSeconds = 1000 * *extensions.TimeoutSeconds
	}
	arguments := []string{
		fmt.Sprintf("%s=%s", "--port", TensorflowServingGRPCPort),
		fmt.Sprintf("%s=%s", "--rest_api_port", TensorflowServingRestPort),
		fmt.Sprintf("%s=%s", "--model_name", metadata.Name),
		fmt.Sprintf("%s=%s", "--model_base_path", constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", "--rest_api_timeout_in_ms", strconv.Itoa(int(TimeoutMilliSeconds))),
	}
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.Tensorflow.ContainerImage + ":" + *t.RuntimeVersion
	}
	t.Container.Name = constants.InferenceServiceContainerName
	arguments = append(arguments, t.Args...)
	t.Container.Args = arguments
	t.Container.Command = []string{TensorflowEntrypointCommand}
	return &t.Container
}

func (t *TFServingSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (t *TFServingSpec) IsMMS(config *InferenceServicesConfig) bool {
	return config.Predictors.Tensorflow.MultiModelServer
}

func (t *TFServingSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	supportedFrameworks := config.Predictors.Tensorflow.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}
