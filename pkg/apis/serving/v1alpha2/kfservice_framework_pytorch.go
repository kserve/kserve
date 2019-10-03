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

// TODO add image name to to configmap
var (
	AllowedPyTorchRuntimeVersions = []string{
		"latest",
		"v0.1.2",
	}
	InvalidPyTorchRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedPyTorchRuntimeVersions, ", ")
	PyTorchServerImageName            = "gcr.io/kfserving/pytorchserver"
	DefaultPyTorchRuntimeVersion      = "latest"
	DefaultPyTorchModelClassName      = "PyTorchModel"
)

var _ Predictor = (*PyTorchSpec)(nil)

func (s *PyTorchSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *PyTorchSpec) CreateModelServingContainer(modelName string, config *PredictorsConfig) *v1.Container {
	imageName := PyTorchServerImageName
	if config.PyTorch.ContainerImage != "" {
		imageName = config.PyTorch.ContainerImage
	}
	return &v1.Container{
		Image:     imageName + ":" + s.RuntimeVersion,
		Resources: s.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_class_name=" + s.ModelClassName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
		},
	}
}

func (s *PyTorchSpec) ApplyDefaults() {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = DefaultPyTorchRuntimeVersion
	}
	if s.ModelClassName == "" {
		s.ModelClassName = DefaultPyTorchModelClassName
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *PyTorchSpec) Validate() error {
	if utils.Includes(AllowedPyTorchRuntimeVersions, s.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidPyTorchRuntimeVersionError)
}
