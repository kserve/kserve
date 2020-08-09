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
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
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

var _ Component = &TritonSpec{}

// Validate returns an error if invalid
func (t *TritonSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TritonSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
	if t.RuntimeVersion == nil {
		t.RuntimeVersion = proto.String(config.Predictors.Triton.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&t.Resources)
}

// GetContainers transforms the resource into a container spec
func (t *TritonSpec) GetContainer(metadata metav1.ObjectMeta, containerConcurrency *int64, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", "--model-store", constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", "--grpc-port", fmt.Sprint(TritonISGRPCPort)),
		fmt.Sprintf("%s=%s", "--http-port", fmt.Sprint(TritonISRestPort)),
		fmt.Sprintf("%s=%s", "--allow-poll-model-repository", "false"),
		fmt.Sprintf("%s=%s", "--allow-grpc", "true"),
		fmt.Sprintf("%s=%s", "--allow-http", "true"),
	}

	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.Triton.ContainerImage + ":" + *t.RuntimeVersion
	}
	t.Name = constants.InferenceServiceContainerName
	t.Command = []string{"trtserver"}
	t.Args = arguments
	return &t.Container
}

func (t *TritonSpec) GetStorageUri() *string {
	return t.StorageURI
}
