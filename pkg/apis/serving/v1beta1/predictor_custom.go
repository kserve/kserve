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
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomPredictor defines arguments for configuring a custom server.
type CustomPredictor struct {
	v1.PodSpec `json:",inline"`
}

var _ ComponentImplementation = &CustomPredictor{}

func NewCustomPredictor(podSpec *PodSpec) *CustomPredictor {
	return &CustomPredictor{PodSpec: v1.PodSpec(*podSpec)}
}

// Validate returns an error if invalid
func (c *CustomPredictor) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(c.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (c *CustomPredictor) Default(config *InferenceServicesConfig) {
	if len(c.Containers) == 0 {
		c.Containers = append(c.Containers, v1.Container{})
	}
	c.Containers[0].Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(&c.Containers[0].Resources)
}

func (c *CustomPredictor) GetStorageUri() *string {
	// return the CustomSpecStorageUri env variable value if set on the spec
	for _, envVar := range c.Containers[0].Env {
		if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
			return &envVar.Value
		}
	}
	return nil
}

// GetContainers transforms the resource into a container spec
func (c *CustomPredictor) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	return &c.Containers[0]
}

func (c *CustomPredictor) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}
