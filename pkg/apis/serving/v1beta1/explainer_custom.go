/*
Copyright 2021 The KServe Authors.

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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// CustomExplainer defines arguments for configuring a custom explainer.
type CustomExplainer struct {
	corev1.PodSpec `json:",inline"`
}

var _ ComponentImplementation = &CustomExplainer{}

func NewCustomExplainer(podSpec *PodSpec) *CustomExplainer {
	return &CustomExplainer{PodSpec: corev1.PodSpec(*podSpec)}
}

// Validate the spec
func (s *CustomExplainer) Validate() error {
	return utils.FirstNonNilError([]error{})
}

// Default sets defaults on the resource
func (c *CustomExplainer) Default(config *InferenceServicesConfig) {
	if len(c.Containers) == 0 {
		c.Containers = append(c.Containers, corev1.Container{})
	}
	c.Containers[0].Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(config, &c.Containers[0].Resources)
}

func (c *CustomExplainer) GetStorageUri() *string {
	// return the CustomSpecStorageUri env variable value if set on the spec
	for _, envVar := range c.Containers[0].Env {
		if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
			return &envVar.Value
		}
	}
	return nil
}

func (c *CustomExplainer) GetStorageSpec() *StorageSpec {
	return nil
}

// GetContainer transforms the resource into a container spec
func (c *CustomExplainer) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig,
	predictorHost ...string,
) *corev1.Container {
	container := &c.Containers[0]
	if !utils.IncludesArg(container.Args, constants.ArgumentModelName) {
		container.Args = append(container.Args, []string{
			constants.ArgumentModelName,
			metadata.Name,
		}...)
	}
	if !utils.IncludesArg(container.Args, constants.ArgumentPredictorHost) {
		container.Args = append(container.Args, []string{
			constants.ArgumentPredictorHost,
			fmt.Sprintf("%s.%s", predictorHost[0], metadata.Namespace),
		}...)
	}
	container.Args = append(container.Args, []string{
		constants.ArgumentHttpPort,
		constants.InferenceServiceDefaultHttpPort,
	}...)
	if !utils.IncludesArg(container.Args, constants.ArgumentWorkers) {
		if extensions.ContainerConcurrency != nil {
			container.Args = append(container.Args, constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10))
		}
	}
	return &c.Containers[0]
}

func (c *CustomExplainer) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (c *CustomExplainer) IsMMS(config *InferenceServicesConfig) bool {
	// TODO: dynamically figure out if custom explainer supports MMS
	return false
}
