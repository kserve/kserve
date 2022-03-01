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

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkglogging "knative.dev/pkg/logging"
)

// CustomTransformer defines arguments for configuring a custom transformer.
type CustomTransformer struct {
	v1.PodSpec `json:",inline"`
}

var _ ComponentImplementation = &CustomTransformer{}

func NewCustomTransformer(podSpec *PodSpec) *CustomTransformer {
	return &CustomTransformer{PodSpec: v1.PodSpec(*podSpec)}
}

// Validate returns an error if invalid
func (c *CustomTransformer) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(c.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (c *CustomTransformer) Default(config *InferenceServicesConfig) {
	if len(c.Containers) == 0 {
		c.Containers = append(c.Containers, v1.Container{})
	}
	c.Containers[0].Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(&c.Containers[0].Resources)
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

// GetContainers transforms the resource into a container spec
func (c *CustomTransformer) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	container := &c.Containers[0]
	argumentPredictorHost := fmt.Sprintf("%s.%s", constants.DefaultPredictorServiceName(metadata.Name), metadata.Namespace)

	deploymentMode, ok := metadata.Annotations[constants.DeploymentMode]
	logger, _ := pkglogging.NewLogger("", "INFO")
	logger.Infof("========isvc deploymentMode========>> %s", deploymentMode)
	if ok && (deploymentMode == string(constants.ModelMeshDeployment)) {
		mm_service_name := config.ModelMesh.ModelMeshServiceName
		mm_http_port := config.ModelMesh.ModelMeshHTTPPort
		argumentPredictorHost = fmt.Sprintf("%s.%s:%d", mm_service_name, metadata.Namespace, mm_http_port)
		argumentPredictorHost = metadata.Annotations["predictor-host"]
		argumentPredictorProtocol := metadata.Annotations["predictor-protocol"]

		if utils.IncludesArg(container.Args, "--protocol") {
			for i := range container.Args {
				logger.Infof("========container.Args[i]========>> %s", container.Args[i])
			}
		} else {
			logger.Infof("========argumentPredictorProtocol========>> %s", argumentPredictorProtocol)
			container.Args = append(container.Args, []string{
				"--protocol",
				argumentPredictorProtocol,
			}...)
		}
	}

	if !utils.IncludesArg(container.Args, constants.ArgumentModelName) {
		container.Args = append(container.Args, []string{
			constants.ArgumentModelName,
			metadata.Name,
		}...)
	}
	logger.Infof("========isvc argumentPredictorHost========>> %s", argumentPredictorHost)
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
	return constants.ProtocolV1
}

func (c *CustomTransformer) IsMMS(config *InferenceServicesConfig) bool {
	// TODO: figure out if custom transformer supports MMS
	return false
}
