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
)

// TorchServeSpec defines arguments for configuring PyTorch model serving.
type TorchServeSpec struct {
	// Defaults PyTorch model class name to 'PyTorchModel'
	ModelClassName string `json:"modelClassName,omitempty"`
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (t *TorchServeSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TorchServeSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
}

// GetContainers transforms the resource into a container spec
func (t *TorchServeSpec) GetContainer(modelName string, containerConcurrency int, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		"torchserve",
		"--start",
		fmt.Sprintf("%s=%s", "--model-store=", constants.DefaultModelLocalMountPath),
	}

	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.PyTorch.ContainerImage + ":" + t.RuntimeVersion
	}
	t.Name = constants.InferenceServiceContainerName
	t.Args = arguments
	return &t.Container
}

func (t *TorchServeSpec) GetStorageUri() *string {
	return t.StorageURI
}
