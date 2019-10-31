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
	InvalidPyTorchRuntimeVersionError = "PyTorch RuntimeVersion must be one of %s"
	DefaultPyTorchModelClassName      = "PyTorchModel"
)

var _ Predictor = (*PyTorchSpec)(nil)

func (s *PyTorchSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *PyTorchSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &s.Resources
}

func (s *PyTorchSpec) GetContainer(modelName string, config *InferenceServicesConfig, hasInferenceLogging bool) *v1.Container {
	return &v1.Container{
		Image:     config.Predictors.PyTorch.ContainerImage + ":" + s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_class_name=" + s.ModelClassName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
			"--http_port=" + constants.GetInferenceServiceHttpPort(hasInferenceLogging),
		},
	}
}

func (s *PyTorchSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Predictors.PyTorch.DefaultImageVersion
	}
	if s.ModelClassName == "" {
		s.ModelClassName = DefaultPyTorchModelClassName
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *PyTorchSpec) Validate(config *InferenceServicesConfig) error {
	if utils.Includes(config.Predictors.PyTorch.AllowedImageVersions, s.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidPyTorchRuntimeVersionError, strings.Join(config.Predictors.PyTorch.AllowedImageVersions, ", "))
}
