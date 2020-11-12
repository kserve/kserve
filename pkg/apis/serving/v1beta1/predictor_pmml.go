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
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PMMLSpec defines arguments for configuring PMML model serving.
type PMMLSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &PMMLSpec{}

// Validate returns an error if invalid
func (k *PMMLSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(k.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (k *PMMLSpec) Default(config *InferenceServicesConfig) {
	k.Container.Name = constants.InferenceServiceContainerName
	if k.RuntimeVersion == nil {
		k.RuntimeVersion = proto.String(config.Predictors.PMML.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&k.Resources)
}

// GetContainer transforms the resource into a container spec
func (k *PMMLSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, metadata.Name),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
	}
	if extensions.ContainerConcurrency != nil {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10)))
	}
	if k.Container.Image == "" {
		k.Container.Image = config.Predictors.PMML.ContainerImage + ":" + *k.RuntimeVersion
	}
	k.Container.Name = constants.InferenceServiceContainerName
	k.Container.Args = arguments
	return &k.Container
}

func (k *PMMLSpec) GetStorageUri() *string {
	return k.StorageURI
}
