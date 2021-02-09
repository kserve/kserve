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

// LightGBMSpec defines arguments for configuring LightGBMSpec model serving.
type LightGBMSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &LightGBMSpec{}

// Validate returns an error if invalid
func (x *LightGBMSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(x.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (x *LightGBMSpec) Default(config *InferenceServicesConfig) {
	x.Container.Name = constants.InferenceServiceContainerName
	if x.RuntimeVersion == nil {
		x.RuntimeVersion = proto.String(config.Predictors.LightGBM.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&x.Resources)

}

// GetContainer transforms the resource into a container spec
func (x *LightGBMSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	cpuLimit := x.Resources.Limits.Cpu()
	cpuLimit.RoundUp(0)
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, metadata.Name),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
		fmt.Sprintf("%s=%s", "--nthread", strconv.Itoa(int(cpuLimit.Value()))),
	}
	if extensions.ContainerConcurrency != nil {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10)))
	}
	if x.Container.Image == "" {
		x.Container.Image = config.Predictors.LightGBM.ContainerImage + ":" + *x.RuntimeVersion
	}
	x.Container.Name = constants.InferenceServiceContainerName
	x.Container.Args = arguments
	return &x.Container
}

func (x *LightGBMSpec) GetStorageUri() *string {
	return x.StorageURI
}

func (x *LightGBMSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (x *LightGBMSpec) IsMMS(config *InferenceServicesConfig) bool {
	return config.Predictors.LightGBM.MultiModelServer
}
