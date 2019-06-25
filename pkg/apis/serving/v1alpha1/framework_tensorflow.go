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

package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

var (
	TensorflowEntrypointCommand        = "/usr/bin/tensorflow_model_server"
	TensorflowServingGRPCPort          = "9000"
	TensorflowServingRestPort          = "8080"
	TensorflowServingImageName         = "tensorflow/serving"
	DefaultTensorflowRuntimeVersion    = "latest"
	TensorflowServingGPUSuffix         = "-gpu"
	DefaultTensorflowRuntimeVersionGPU = DefaultTensorflowRuntimeVersion + TensorflowServingGPUSuffix
	AllowedTensorflowRuntimeVersions   = []string{
		DefaultTensorflowRuntimeVersion,
		DefaultTensorflowRuntimeVersionGPU,
		"1.13.0",
		"1.13.0" + TensorflowServingGPUSuffix,
		"1.12.0",
		"1.12.0" + TensorflowServingGPUSuffix,
		"1.11.0",
		"1.11.0" + TensorflowServingGPUSuffix,
	}
	InvalidTensorflowRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedTensorflowRuntimeVersions, ", ")
	InvalidTensorflowRuntimeIncludesGPU  = "RuntimeVersion is not GPU enabled but GPU resources are requested. " + InvalidTensorflowRuntimeVersionError
	InvalidTensorflowRuntimeExcludesGPU  = "RuntimeVersion is GPU enabled but GPU resources are not requested. " + InvalidTensorflowRuntimeVersionError
)

func (t *TensorflowSpec) GetModelSourceUri() string {
	return t.ModelURI
}

func (t *TensorflowSpec) CreateModelServingContainer(modelName string, config *FrameworksConfig) *v1.Container {
	imageName := TensorflowServingImageName
	if config.Tensorflow.ContainerImage != "" {
		imageName = config.Tensorflow.ContainerImage
	}

	return &v1.Container{
		Image:     imageName + ":" + t.RuntimeVersion,
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

func (t *TensorflowSpec) ApplyDefaults() {
	if t.RuntimeVersion == "" {
		if isGPUEnabled(t.Resources) {
			t.RuntimeVersion = DefaultTensorflowRuntimeVersionGPU
		} else {
			t.RuntimeVersion = DefaultTensorflowRuntimeVersion
		}

	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *TensorflowSpec) Validate() error {
	if !utils.Includes(AllowedTensorflowRuntimeVersions, t.RuntimeVersion) {
		return fmt.Errorf(InvalidTensorflowRuntimeVersionError)
	}

	if isGPUEnabled(t.Resources) && !strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeIncludesGPU)
	}

	if !isGPUEnabled(t.Resources) && strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeExcludesGPU)
	}

	return nil
}
