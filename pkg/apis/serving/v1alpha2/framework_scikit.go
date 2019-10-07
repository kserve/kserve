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
	AllowedSKLearnRuntimeVersions = []string{
		"latest",
		"v0.1.2",
	}
	InvalidSKLearnRuntimeVersionError = "RuntimeVersion must be one of %s"
	SKLearnServerImageName            = "gcr.io/kfserving/sklearnserver"
)

var _ Predictor = (*SKLearnSpec)(nil)

func (s *SKLearnSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *SKLearnSpec) GetContainer(modelName string, config *PredictorsConfig) *v1.Container {
	imageName := SKLearnServerImageName
	if config.SKlearn.ContainerImage != "" {
		imageName = config.SKlearn.ContainerImage
	}
	return &v1.Container{
		Image:     imageName + ":" + s.RuntimeVersion,
		Resources: s.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
		},
	}
}

func (s *SKLearnSpec) ApplyDefaults(config *PredictorsConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.SKlearn.DefaultImageVersion
	}

	setResourceRequirementDefaults(&s.Resources)
}

func (s *SKLearnSpec) Validate(config *PredictorsConfig) error {
	if utils.Includes(config.SKlearn.AllowedImageVersions, s.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidSKLearnRuntimeVersionError, strings.Join(AllowedSKLearnRuntimeVersions, ", "))
}
