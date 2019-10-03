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
	ONNXServingRestPort        = "8080"
	ONNXServingGRPCPort        = "9000"
	ONNXServingImageName       = "mcr.microsoft.com/onnxruntime/server"
	ONNXModelFileName          = "model.onnx"
	DefaultONNXRuntimeVersion  = "latest"
	AllowedONNXRuntimeVersions = []string{
		DefaultONNXRuntimeVersion,
		"v0.5.0",
	}
	InvalidONNXRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedONNXRuntimeVersions, ", ")
)

func (s *ONNXSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *ONNXSpec) CreateModelServingContainer(modelName string, config *PredictorsConfig) *v1.Container {
	imageName := ONNXServingImageName
	if config.ONNX.ContainerImage != "" {
		imageName = config.ONNX.ContainerImage
	}

	return &v1.Container{
		Image:     imageName + ":" + s.RuntimeVersion,
		Resources: s.Resources,
		Args: []string{
			"--model_path", constants.DefaultModelLocalMountPath + "/" + ONNXModelFileName,
			"--http_port", ONNXServingRestPort,
			"--grpc_port", ONNXServingGRPCPort,
		},
	}
}

func (s *ONNXSpec) ApplyDefaults() {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = DefaultONNXRuntimeVersion
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *ONNXSpec) Validate() error {
	if !utils.Includes(AllowedONNXRuntimeVersions, s.RuntimeVersion) {
		return fmt.Errorf(InvalidONNXRuntimeVersionError)
	}

	return nil
}
