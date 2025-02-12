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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// CustomTransformer defines arguments for configuring a custom transformer.
type CustomTransformer struct {
	corev1.PodSpec `json:",inline"`
}

// logger for the custom transformer
var customTransformerLogger = logf.Log.WithName("inferenceservice-v1beta1-custom-transformer")

var _ ComponentImplementation = &CustomTransformer{}

func NewCustomTransformer(podSpec *PodSpec) *CustomTransformer {
	return &CustomTransformer{PodSpec: corev1.PodSpec(*podSpec)}
}

// Validate returns an error if invalid
func (c *CustomTransformer) Validate() error {
	return utils.FirstNonNilError([]error{})
}

// Default sets defaults on the resource
func (c *CustomTransformer) Default(config *InferenceServicesConfig) {
	if len(c.Containers) == 0 {
		c.Containers = append(c.Containers, corev1.Container{})
	}
	c.Containers[0].Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(config, &c.Containers[0].Resources)
}

func (c *CustomTransformer) GetStorageUri() *string {
	// return the CustomSpecStorageUri env variable value if set on the spec
	for _, envVar := range c.Containers[0].Env {
		if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
			return &envVar.Value
		}
	}
	return nil
}

func (c *CustomTransformer) GetStorageSpec() *StorageSpec {
	return nil
}

// GetContainer transforms the resource into a container spec
func (c *CustomTransformer) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig,
	predictorHost ...string,
) *corev1.Container {
	container := &c.Containers[0]
	argumentPredictorHost := fmt.Sprintf("%s.%s", predictorHost[0], metadata.Namespace)
	deploymentMode, ok := metadata.Annotations[constants.DeploymentMode]

	if ok && (deploymentMode == string(constants.ModelMeshDeployment)) {
		// Get predictor host and protocol from annotations in modelmesh deployment mode
		argumentPredictorHost = metadata.Annotations[constants.PredictorHostAnnotationKey]
		argumentPredictorProtocol := metadata.Annotations[constants.PredictorProtocolAnnotationKey]

		// Set predictor protocol if not provided in container arguments
		if !utils.IncludesArg(container.Args, "--protocol") {
			customTransformerLogger.Info("Set predictor protocol based on ModelMesh predictor URL", "protocol", argumentPredictorProtocol)
			container.Args = append(container.Args, "--protocol", argumentPredictorProtocol)
		}
	}

	if !utils.IncludesArg(container.Args, constants.ArgumentModelName) {
		container.Args = append(container.Args, []string{
			constants.ArgumentModelName,
			metadata.Name,
		}...)
	}
	if !utils.IncludesArg(container.Args, constants.ArgumentPredictorHost) {
		container.Args = append(container.Args, []string{
			constants.ArgumentPredictorHost,
			argumentPredictorHost,
		}...)
	}
	if !utils.IncludesArg(container.Args, constants.ArgumentHttpPort) {
		container.Args = append(container.Args, []string{
			constants.ArgumentHttpPort,
			constants.InferenceServiceDefaultHttpPort,
		}...)
	}
	if !utils.IncludesArg(container.Args, constants.ArgumentWorkers) {
		if extensions.ContainerConcurrency != nil {
			container.Args = append(container.Args, constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10))
		}
	}
	return &c.Containers[0]
}

func (c *CustomTransformer) GetProtocol() constants.InferenceServiceProtocol {
	for _, envVar := range c.Containers[0].Env {
		if envVar.Name == constants.CustomSpecProtocolEnvVarKey {
			return constants.InferenceServiceProtocol(envVar.Value)
		}
	}
	return constants.ProtocolV1
}

func (c *CustomTransformer) IsMMS(config *InferenceServicesConfig) bool {
	// TODO: figure out if custom transformer supports MMS
	return false
}
