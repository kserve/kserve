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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// CustomPredictor defines arguments for configuring a custom server.
type CustomPredictor struct {
	corev1.PodSpec `json:",inline"`
}

var (
	_ ComponentImplementation = &CustomPredictor{}
	_ PredictorImplementation = &CustomPredictor{}
)

func NewCustomPredictor(podSpec *PodSpec) *CustomPredictor {
	return &CustomPredictor{PodSpec: corev1.PodSpec(*podSpec)}
}

// Validate returns an error if invalid
func (c *CustomPredictor) Validate() error {
	return utils.FirstNonNilError([]error{
		c.validateCustomProtocol(),
	})
}

func (c *CustomPredictor) validateCustomProtocol() error {
	for _, envVar := range c.Containers[0].Env {
		if envVar.Name == constants.CustomSpecProtocolEnvVarKey {
			if envVar.Value == string(constants.ProtocolV1) || envVar.Value == string(constants.ProtocolV2) {
				return nil
			} else {
				return fmt.Errorf(InvalidProtocol, strings.Join([]string{string(constants.ProtocolV1), string(constants.ProtocolV2)}, ", "), envVar.Value)
			}
		}
	}
	return nil
}

// Default sets defaults on the resource
func (c *CustomPredictor) Default(config *InferenceServicesConfig) {
	if len(c.Containers) == 0 {
		c.Containers = append(c.Containers, corev1.Container{})
	}
	if len(c.Containers) == 1 || len(c.Containers[0].Name) == 0 {
		c.Containers[0].Name = constants.InferenceServiceContainerName
	}
	setResourceRequirementDefaults(config, &c.Containers[0].Resources)
}

func (c *CustomPredictor) GetStorageUri() *string {
	// return the CustomSpecStorageUri env variable value if set on the spec
	for _, container := range c.Containers {
		if container.Name == constants.InferenceServiceContainerName {
			for _, envVar := range container.Env {
				if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
					return &envVar.Value
				}
			}
			break
		}
	}
	return nil
}

func (c *CustomPredictor) GetStorageSpec() *StorageSpec {
	return nil
}

// GetContainer transforms the resource into a container spec
func (c *CustomPredictor) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig,
	predictorHost ...string,
) *corev1.Container {
	for _, container := range c.Containers {
		if container.Name == constants.InferenceServiceContainerName {
			return &container
		}
	}
	return nil
}

func (c *CustomPredictor) GetProtocol() constants.InferenceServiceProtocol {
	// Handle collocation of transformer and predictor scenario
	for _, container := range c.Containers {
		if container.Name == constants.TransformerContainerName {
			for _, envVar := range container.Env {
				if envVar.Name == constants.CustomSpecProtocolEnvVarKey {
					return constants.InferenceServiceProtocol(envVar.Value)
				}
			}
			return constants.ProtocolV1
		}
	}
	for _, envVar := range c.Containers[0].Env {
		if envVar.Name == constants.CustomSpecProtocolEnvVarKey {
			return constants.InferenceServiceProtocol(envVar.Value)
		}
	}
	return constants.ProtocolV1
}
