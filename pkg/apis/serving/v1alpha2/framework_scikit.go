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
	"strconv"

	"github.com/kserve/kserve/pkg/constants"
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

func (s *SKLearnSpec) GetContainer(modelName string, parallelism int, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, modelName),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
	}
	if parallelism != 0 {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.Itoa(parallelism)))
	}
	return &v1.Container{
		Image:     config.Predictors.SKlearn.V1.ContainerImage + ":" + s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args:      arguments,
	}
}

func (s *SKLearnSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Predictors.SKlearn.V1.DefaultImageVersion
	}

	setResourceRequirementDefaults(&s.Resources)
}

func (s *SKLearnSpec) Validate(config *InferenceServicesConfig) error {
	return nil
}
