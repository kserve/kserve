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
	"fmt"

	"github.com/kubeflow/kfserving/pkg/constants"
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

func (c *CustomSpec) GetContainer(modelName string, config *PredictorsConfig) *v1.Container {
	return &c.Container
}
func (c *CustomSpec) CreateExplainerContainer(modelName string, predictUrl string, config *ExplainersConfig) *v1.Container {
	return &c.Container
}

func (c *CustomSpec) ApplyDefaults() {
	setResourceRequirementDefaults(&c.Container.Resources)
}

func (c *CustomSpec) Validate() error {
	err := knserving.ValidateContainer(c.Container, sets.String{})
	if err != nil {
		return fmt.Errorf("Custom container validation error: %s", err.Error())
	}
	return nil
}
