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
	"strconv"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomExplainer defines arguments for configuring a custom explainer.
type CustomExplainer struct {
	// This spec is dual purpose.
	// 1) Users may choose to provide a full PodSpec for their explainer.
	// The field PodSpec.Containers is mutually exclusive with other explainers (i.e. Alibi).
	// 2) Users may choose to provide a Explainer (i.e. Alibi) and specify PodSpec
	// overrides in the CustomExplainer PodSpec. They must not provide PodSpec.Containers in this case.
	v1.PodTemplateSpec `json:",inline"`
}

var _ ComponentImplementation = &CustomExplainer{}

// Validate the spec
func (s *CustomExplainer) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(s.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (c *CustomExplainer) Default(config *InferenceServicesConfig) {
	if len(c.Spec.Containers) == 0 {
		c.Spec.Containers = append(c.Spec.Containers, v1.Container{})
	}
	c.Spec.Containers[0].Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(&c.Spec.Containers[0].Resources)
}

func (c *CustomExplainer) GetStorageUri() *string {
	// return the CustomSpecStorageUri env variable value if set on the spec
	for _, envVar := range c.Spec.Containers[0].Env {
		if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
			return &envVar.Value
		}
	}
	return nil
}

// GetContainer transforms the resource into a container spec
func (c *CustomExplainer) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	container := &c.Spec.Containers[0]
	modelNameExists := false
	for _, arg := range container.Args {
		if arg == constants.ArgumentModelName {
			modelNameExists = true
		}
	}
	if !modelNameExists {
		container.Args = append(container.Args, []string{
			constants.ArgumentModelName,
			metadata.Name,
		}...)
	}
	container.Args = append(container.Args, []string{
		constants.ArgumentPredictorHost,
		constants.PredictorURL(metadata, false),
		constants.ArgumentHttpPort,
		constants.InferenceServiceDefaultHttpPort,
	}...)
	if extensions.ContainerConcurrency != nil {
		container.Args = append(container.Args, constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10))
	}
	return &c.Spec.Containers[0]
}
