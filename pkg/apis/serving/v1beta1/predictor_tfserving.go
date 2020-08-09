/*
Copyright 2020 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strings"
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

// Validate returns an error if invalid
func (t *TFServingSpec) Validate() error {
	if isGPUEnabled(t.Resources) && !strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeIncludesGPU)
	}

	if !isGPUEnabled(t.Resources) && strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeExcludesGPU)
	}
	return nil
}

// Default sets defaults on the resource
func (t *TFServingSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
	if t.RuntimeVersion == "" {
		if isGPUEnabled(t.Resources) {
			t.RuntimeVersion = config.Predictors.Tensorflow.DefaultGpuImageVersion
		} else {
			t.RuntimeVersion = config.Predictors.Tensorflow.DefaultImageVersion
		}
	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *TFServingSpec) GetStorageUri() *string {
	return t.StorageURI
}

// GetContainers transforms the resource into a container spec
func (t *TFServingSpec) GetContainer(modelName string, containerConcurrency *int64, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		"--port=" + TensorflowServingGRPCPort,
		"--rest_api_port=" + TensorflowServingRestPort,
		"--model_name=" + modelName,
		"--model_base_path=" + constants.DefaultModelLocalMountPath,
	}
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.Tensorflow.ContainerImage + ":" + t.RuntimeVersion
	}
	t.Container.Name = constants.InferenceServiceContainerName
	t.Container.Args = arguments
	t.Container.Command = []string{TensorflowEntrypointCommand}
	return &t.Container
}
