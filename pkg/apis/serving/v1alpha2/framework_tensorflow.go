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

package v1alpha2

import (
	"fmt"
	"strings"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

var (
	TensorflowEntrypointCommand         = "/usr/bin/tensorflow_model_server"
	TensorflowServingGRPCPort           = "9000"
	TensorflowServingRestPort           = "8080"
	TensorflowServingGPUSuffix          = "-gpu"
	InvalidTensorflowRuntimeIncludesGPU = "Tensorflow RuntimeVersion is not GPU enabled but GPU resources are requested. "
	InvalidTensorflowRuntimeExcludesGPU = "Tensorflow RuntimeVersion is GPU enabled but GPU resources are not requested. "
)

func (t *TensorflowSpec) GetStorageUri() string {
	return t.StorageURI
}

func (t *TensorflowSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &t.Resources
}

func (t *TensorflowSpec) GetContainer(modelName string, parallelism int, config *InferenceServicesConfig) *v1.Container {

	arguments := []string{
		"--port=" + TensorflowServingGRPCPort,
		"--rest_api_port=" + TensorflowServingRestPort,
		"--model_name=" + modelName,
		"--model_base_path=" + constants.DefaultModelLocalMountPath,
		"--rest_api_timeout_in_ms=" + fmt.Sprint(1000*config.Predictors.Tensorflow.DefaultTimeout),
	}

	return &v1.Container{
		Image:     config.Predictors.Tensorflow.ContainerImage + ":" + t.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Command:   []string{TensorflowEntrypointCommand},
		Resources: t.Resources,
		Args:      arguments,
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/v1/models/" + modelName,
				},
			},
			InitialDelaySeconds: constants.DefaultReadinessTimeout,
			PeriodSeconds:       10,
			FailureThreshold:    3,
			SuccessThreshold:    1,
			TimeoutSeconds:      1,
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
	if isGPUEnabled(t.Resources) && !strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeIncludesGPU)
	}

	if !isGPUEnabled(t.Resources) && strings.Contains(t.RuntimeVersion, TensorflowServingGPUSuffix) {
		return fmt.Errorf(InvalidTensorflowRuntimeExcludesGPU)
	}

	return nil
}
