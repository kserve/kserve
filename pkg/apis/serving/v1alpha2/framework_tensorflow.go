/*
Copyright 2019 kubeflow.org.
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

package v1alpha2

import (
	"fmt"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
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

func (t *TensorflowSpec) GetStorageUri() string {
	return t.StorageURI
}

func (t *TensorflowSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &t.Resources
}

func (t *TensorflowSpec) GetContainer(serviceName string, config *InferenceServicesConfig) *v1.Container {
	modelName := serviceName
	if t.ModelName != "" {
		modelName = t.ModelName
	}
	return &v1.Container{
		Image:     config.Predictors.Tensorflow.ContainerImage + ":" + t.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Command:   []string{TensorflowEntrypointCommand},
		Resources: t.Resources,
		Args: []string{
			"--port=" + TensorflowServingGRPCPort,
			"--rest_api_port=" + TensorflowServingRestPort,
			"--model_name=" + modelName,
			"--model_base_path=" + constants.DefaultModelLocalMountPath,
		},
	}
}

func (t *TensorflowSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if t.RuntimeVersion == "" {
		if isGPUEnabled(t.Resources) {
			t.RuntimeVersion = config.Predictors.Tensorflow.DefaultGpuImageVersion
		} else {
			t.RuntimeVersion = config.Predictors.Tensorflow.DefaultImageVersion
		}

	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *TensorflowSpec) Validate(config *InferenceServicesConfig) error {
	if !utils.Includes(config.Predictors.Tensorflow.AllowedImageVersions, t.RuntimeVersion) {
		return fmt.Errorf(InvalidTensorflowRuntimeVersionError, strings.Join(config.Predictors.Tensorflow.AllowedImageVersions, ", "))
	}

	if isGPUEnabled(t.Resources) && !strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeIncludesGPU, strings.Join(config.Predictors.Tensorflow.AllowedImageVersions, ", "))
	}

	if !isGPUEnabled(t.Resources) && strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeExcludesGPU, strings.Join(config.Predictors.Tensorflow.AllowedImageVersions, ", "))
	}

	return nil
}
