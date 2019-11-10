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
	InvalidSKLearnRuntimeVersionError = "SKLearn RuntimeVersion must be one of %s"
)

var _ Predictor = (*SKLearnSpec)(nil)

func (s *SKLearnSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *SKLearnSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &s.Resources
}

func (s *SKLearnSpec) GetContainer(modelName string, config *InferenceServicesConfig, hasLogging bool) *v1.Container {
	return &v1.Container{
		Image:     config.Predictors.SKlearn.ContainerImage + ":" + s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
			"--http_port=" + constants.GetInferenceServiceHttpPort(hasLogging),
		},
	}
}

func (s *SKLearnSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Predictors.SKlearn.DefaultImageVersion
	}

	setResourceRequirementDefaults(&s.Resources)
}

func (s *SKLearnSpec) Validate(config *InferenceServicesConfig) error {
	if utils.Includes(config.Predictors.SKlearn.AllowedImageVersions, s.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidSKLearnRuntimeVersionError, strings.Join(config.Predictors.SKlearn.AllowedImageVersions, ", "))
}
