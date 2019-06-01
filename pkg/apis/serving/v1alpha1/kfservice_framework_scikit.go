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

package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

// TODO add image name to to configmap
var (
	AllowedSKLearnRuntimeVersions = []string{
		"latest",
	}
	InvalidSKLearnRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedSKLearnRuntimeVersions, ", ")
	SKLearnServerImageName            = "gcr.io/kfserving/sklearnserver"
	DefaultSKLearnRuntimeVersion      = "latest"
)

var _ FrameworkHandler = (*SKLearnSpec)(nil)

func (s *SKLearnSpec) CreateModelServingContainer(modelName string, modelServingImageName string) *v1.Container {
	runtimeVersion := DefaultSKLearnRuntimeVersion
	if s.RuntimeVersion != "" {
		runtimeVersion = s.RuntimeVersion
	}
	imageName := SKLearnServerImageName
	if modelServingImageName != "" {
		imageName = modelServingImageName
	}
	return &v1.Container{
		Image:     imageName + ":" + runtimeVersion,
		Resources: s.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_dir=" + s.ModelURI,
		},
	}
}

func (s *SKLearnSpec) ApplyDefaults() {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = DefaultSKLearnRuntimeVersion
	}

	setResourceRequirementDefaults(&s.Resources)
}

func (s *SKLearnSpec) Validate() error {
	if utils.Includes(AllowedSKLearnRuntimeVersions, s.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidSKLearnRuntimeVersionError)
}
