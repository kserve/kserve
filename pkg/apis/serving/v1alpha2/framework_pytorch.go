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

func (p *PyTorchSpec) GetStorageUri() string {
	return p.StorageURI
}

func (p *PyTorchSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &p.Resources
}

func (p *PyTorchSpec) GetContainer(serviceName string, config *InferenceServicesConfig) *v1.Container {
	modelName := serviceName
	if p.ModelName != "" {
		modelName = p.ModelName
	}
	return &v1.Container{
		Image:     config.Predictors.PyTorch.ContainerImage + ":" + p.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: p.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_class_name=" + p.ModelClassName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
			"--http_port=" + constants.InferenceServiceDefaultHttpPort,
		},
	}
}

func (p *PyTorchSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if p.RuntimeVersion == "" {
		p.RuntimeVersion = config.Predictors.PyTorch.DefaultImageVersion
	}
	if p.ModelClassName == "" {
		p.ModelClassName = DefaultPyTorchModelClassName
	}
	setResourceRequirementDefaults(&p.Resources)
}

func (p *PyTorchSpec) Validate(config *InferenceServicesConfig) error {
	if utils.Includes(config.Predictors.PyTorch.AllowedImageVersions, p.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidPyTorchRuntimeVersionError, strings.Join(config.Predictors.PyTorch.AllowedImageVersions, ", "))
}
