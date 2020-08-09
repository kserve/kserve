/*
Copyright 2020 kubeflow.org.

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

package v1beta1

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strconv"
)

// SKLearnSpec defines arguments for configuring SKLearn model serving.
type SKLearnSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (k *SKLearnSpec) Validate() error {
	validateStorageURI(k.StorageURI)
	return nil
}

// Default sets defaults on the resource
func (k *SKLearnSpec) Default(config *InferenceServicesConfig) {
	k.Container.Name = constants.InferenceServiceContainerName
	if k.RuntimeVersion == "" {
		k.RuntimeVersion = config.Predictors.SKlearn.DefaultImageVersion
	}
	setResourceRequirementDefaults(&k.Resources)
}

// GetContainer transforms the resource into a container spec
func (k *SKLearnSpec) GetContainer(modelName string, containerConcurrency *int64, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, modelName),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
	}
	if containerConcurrency != nil {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.FormatInt(*containerConcurrency, 10)))
	}
	if k.Container.Image == "" {
		k.Container.Image = config.Predictors.SKlearn.ContainerImage + ":" + k.RuntimeVersion
	}
	k.Container.Name = constants.InferenceServiceContainerName
	k.Container.Args = arguments
	return &k.Container
}

func (k *SKLearnSpec) GetStorageUri() *string {
	return k.StorageURI
}
