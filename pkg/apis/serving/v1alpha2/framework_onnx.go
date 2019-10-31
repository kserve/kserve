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
	ONNXServingGRPCPort            = "9000"
	ONNXModelFileName              = "model.onnx"
	InvalidONNXRuntimeVersionError = "ONNX RuntimeVersion must be one of %s"
)

func (s *ONNXSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *ONNXSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &s.Resources
}


func (s *ONNXSpec) GetContainer(modelName string, config *InferenceServicesConfig, hasInferenceLogging bool) *v1.Container {
	return &v1.Container{
		Image:     config.Predictors.ONNX.ContainerImage + ":" + s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args: []string{
			"--model_path", constants.DefaultModelLocalMountPath + "/" + ONNXModelFileName,
			"--http_port", constants.GetInferenceServiceHttpPort(hasInferenceLogging),
			"--grpc_port", ONNXServingGRPCPort,
		},
	}
}

func (s *ONNXSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Predictors.ONNX.DefaultImageVersion
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *ONNXSpec) Validate(config *InferenceServicesConfig) error {
	if !utils.Includes(config.Predictors.ONNX.AllowedImageVersions, s.RuntimeVersion) {
		return fmt.Errorf(InvalidONNXRuntimeVersionError, strings.Join(config.Predictors.ONNX.AllowedImageVersions, ", "))
	}

	return nil
}
