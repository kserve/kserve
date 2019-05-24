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
	v1 "k8s.io/api/core/v1"
)

const (
	SKLearnServingGRPCPort  = "9000"
	SKLearnServingRestPort  = "8080"
	SKLearnServingImageName = "gcr.io/kfserving/sklearnserver"

	DefaultSKLearnServingVersion = "latest"
)

var _ FrameworkHandler = (*SKLearnSpec)(nil)

func (s *SKLearnSpec) CreateModelServingContainer(modelName string, configs map[string]string) *v1.Container {
	sklearnServingImage := SKLearnServingImageName
	if image, ok := configs[SKLearnServingImageConfigName]; ok {
		sklearnServingImage = image
	}
	return &v1.Container{
		Image:     sklearnServingImage + ":" + s.RuntimeVersion,
		Resources: s.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_dir=" + s.ModelURI,
		},
	}
}

func (s *SKLearnSpec) ApplyDefaults() {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = DefaultSKLearnServingVersion
	}

	setResourceRequirementDefaults(&s.Resources)
}

func (s *SKLearnSpec) Validate() error {
	return nil
}
