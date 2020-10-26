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
	ApulisVisionServingGRPCPort            = "9000"
	ApulisVisionServingRestPort            = "8080"
	ApulisVisionServingGPUSuffix           = "-gpu"
	InvalidApulisVisionRuntimeVersionError = "ApulisVision RuntimeVersion must be one of %s"
	InvalidApulisVisionRuntimeIncludesGPU  = "ApulisVision RuntimeVersion is not GPU enabled but GPU resources are requested. " + InvalidApulisVisionRuntimeVersionError
	InvalidApulisVisionRuntimeExcludesGPU  = "ApulisVision RuntimeVersion is GPU enabled but GPU resources are not requested. " + InvalidApulisVisionRuntimeVersionError
)

func (t *ApulisVisionSpec) GetStorageUri() string {
	return t.StorageURI
}

func (t *ApulisVisionSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &t.Resources
}

func (t *ApulisVisionSpec) GetContainer(modelName string, hasLogging bool, config *InferenceServicesConfig) *v1.Container {
	return &v1.Container{
		Image:     config.Predictors.ApulisVision.ContainerImage + ":" + t.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: t.Resources,
		Args: []string{
			"--port=" + ApulisVisionServingGRPCPort,
			"--rest_api_port=" + constants.GetInferenceServiceHttpPort(hasLogging),
			"--model_name=" + modelName,
			"--model_base_path=" + constants.DefaultModelLocalMountPath,
		},
	}
}

func (t *ApulisVisionSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if t.RuntimeVersion == "" {
		if isGPUEnabled(t.Resources) {
			t.RuntimeVersion = config.Predictors.ApulisVision.DefaultGpuImageVersion
		} else {
			t.RuntimeVersion = config.Predictors.ApulisVision.DefaultImageVersion
		}

	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *ApulisVisionSpec) Validate(config *InferenceServicesConfig) error {
	if !utils.Includes(config.Predictors.ApulisVision.AllowedImageVersions, t.RuntimeVersion) {
		return fmt.Errorf(InvalidApulisVisionRuntimeVersionError, strings.Join(config.Predictors.ApulisVision.AllowedImageVersions, ", "))
	}

	//if isGPUEnabled(t.Resources) && !strings.Contains(t.RuntimeVersion, ApulisVisionServingGPUSuffix) {
	//	return fmt.Errorf(InvalidApulisVisionRuntimeIncludesGPU, strings.Join(config.Predictors.ApulisVision.AllowedImageVersions, ", "))
	//}
	//
	//if !isGPUEnabled(t.Resources) && strings.Contains(t.RuntimeVersion, ApulisVisionServingGPUSuffix) {
	//	return fmt.Errorf(InvalidApulisVisionRuntimeExcludesGPU, strings.Join(config.Predictors.ApulisVision.AllowedImageVersions, ", "))
	//}

	return nil
}
