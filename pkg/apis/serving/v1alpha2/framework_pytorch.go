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
	"k8s.io/api/core/v1"
)

// TODO add image name to to configmap
var (
	AllowedPyTorchRuntimeVersions = []string{
		"latest",
	}
	InvalidPyTorchRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedPyTorchRuntimeVersions, ", ")
	PyTorchServerImageName            = "gcr.io/kfserving/pytorchserver"
	DefaultPyTorchRuntimeVersion      = "latest"
	DefaultPyTorchModelClassName      = "PyTorchModel"
)

var _ FrameworkHandler = (*PyTorchSpec)(nil)

func (p *PyTorchSpec) GetModelSourceUri() string {
	return p.ModelURI
}

func (p *PyTorchSpec) CreateModelServingContainer(modelName string, config *FrameworksConfig) *v1.Container {
	imageName := PyTorchServerImageName
	if config.PyTorch.ContainerImage != "" {
		imageName = config.PyTorch.ContainerImage
	}
	return &v1.Container{
		Image:     imageName + ":" + p.RuntimeVersion,
		Resources: p.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_class_name=" + p.ModelClassName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
		},
	}
}

func (p *PyTorchSpec) ApplyDefaults() {
	if p.RuntimeVersion == "" {
		p.RuntimeVersion = DefaultPyTorchRuntimeVersion
	}
	if p.ModelClassName == "" {
		p.ModelClassName = DefaultPyTorchModelClassName
	}
	setResourceRequirementDefaults(&p.Resources)
}

func (p *PyTorchSpec) Validate() error {
	if utils.Includes(AllowedPyTorchRuntimeVersions, p.RuntimeVersion) {
		return nil
	}
	if err := validateReplicas(p.MinReplicas, p.MaxReplicas); err != nil {
		return err
	}
	return fmt.Errorf(InvalidPyTorchRuntimeVersionError)
}
