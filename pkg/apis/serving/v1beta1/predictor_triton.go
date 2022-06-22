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

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	TritonISGRPCPort = int32(9000)
	TritonISRestPort = int32(8080)
)

// TritonSpec defines arguments for configuring Triton model serving.
type TritonSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var (
	_ ComponentImplementation = &TritonSpec{}
	_ PredictorImplementation = &TritonSpec{}
)

// Default sets defaults on the resource
func (t *TritonSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
	if t.RuntimeVersion == nil {
		t.RuntimeVersion = proto.String(config.Predictors.Triton.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&t.Resources)
}

// GetContainers transforms the resource into a container spec
func (t *TritonSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		"tritonserver",
		fmt.Sprintf("%s=%s", "--model-store", constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", "--grpc-port", fmt.Sprint(TritonISGRPCPort)),
		fmt.Sprintf("%s=%s", "--http-port", fmt.Sprint(TritonISRestPort)),
		fmt.Sprintf("%s=%s", "--allow-grpc", "true"),
		fmt.Sprintf("%s=%s", "--allow-http", "true"),
	}
	if extensions.ContainerConcurrency != nil && *extensions.ContainerConcurrency != 0 {
		arguments = append(arguments, fmt.Sprintf("%s=%d", "--http-thread-count", *extensions.ContainerConcurrency))
	}
	// when storageURI is nil we enable explicit load/unload
	if t.StorageURI == nil {
		arguments = append(arguments, fmt.Sprintf("%s=%s", "--model-control-mode", "explicit"))
	}
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.Triton.ContainerImage + ":" + *t.RuntimeVersion
	}
	t.Name = constants.InferenceServiceContainerName
	arguments = append(arguments, t.Args...)
	t.Args = arguments
	return &t.Container
}

func (t *TritonSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV2
}

func (t *TritonSpec) IsMMS(config *InferenceServicesConfig) bool {
	return config.Predictors.Triton.MultiModelServer
}

func (t *TritonSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	supportedFrameworks := config.Predictors.Triton.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}
