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
	"context"
	"fmt"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	knserving "knative.dev/serving/pkg/apis/serving"
)

func (c *CustomSpec) GetStorageUri() string {
	// return the CustomSpecStorageUri env variable value if set on the spec
	for _, envVar := range c.Container.Env {
		if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
			return envVar.Value
		}
	}
	return ""
}

func (c *CustomSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &c.Container.Resources
}

func (c *CustomSpec) GetContainer(modelName string, parallelism int, config *InferenceServicesConfig) *v1.Container {
	return &c.Container
}

func (c *CustomSpec) CreateExplainerContainer(modelName string, parallelism int, predictUrl string, config *InferenceServicesConfig) *v1.Container {
	return &c.Container
}

func (c *CustomSpec) ApplyDefaults(config *InferenceServicesConfig) {
	setResourceRequirementDefaults(&c.Container.Resources)
}

func (c *CustomSpec) Validate(config *InferenceServicesConfig) error {
	err := knserving.ValidateContainer(context.TODO(), c.Container, sets.String{})
	if err != nil {
		return fmt.Errorf("Custom container validation error: %s", err.Error())
	}
	if c.GetStorageUri() != "" && c.Container.Name != constants.InferenceServiceContainerName {
		return fmt.Errorf("Custom container validation error: container name must be %q", constants.InferenceServiceContainerName)
	}
	return nil
}
