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
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MLServerHTTPPortEnv  = "MLSERVER_HTTP_PORT"
	MLServerGRPCPortEnv  = "MLSERVER_GRPC_PORT"
	MLServerModelsDirEnv = "MODELS_DIR"
)

var (
	MLServerISGRPCPort = int32(9000)
	MLServerISRestPort = int32(8080)
)

// MLServerSpec defines the API for configuring MLServer predictors
type MLServerSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if the spec is invalid
func (m *MLServerSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(m.GetStorageUri()),
	})
}

// Default sets some of the spec fields to default values if undefined
func (m *MLServerSpec) Default(config *InferenceServicesConfig) {
	m.Container.Name = constants.InferenceServiceContainerName
	if m.RuntimeVersion == nil {
		m.RuntimeVersion = proto.String(config.Predictors.MLServer.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&m.Resources)
}

// GetContainers transforms the resource into a container spec
func (m *MLServerSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	// TODO: Merge env vars with existing ones
	envVars := m.getEnvVars()
	m.Container.Env = append(envVars, m.Env...)

	if m.Container.Image == "" {
		m.Container.Image = m.getImage(config)
	}

	return &m.Container
}

func (m *MLServerSpec) getEnvVars() []v1.EnvVar {
	return []v1.EnvVar{
		{
			Name:  MLServerHTTPPortEnv,
			Value: fmt.Sprint(MLServerISRestPort),
		},
		{
			Name:  MLServerGRPCPortEnv,
			Value: fmt.Sprint(MLServerISGRPCPort),
		},
		{
			Name:  MLServerModelsDirEnv,
			Value: constants.DefaultModelLocalMountPath,
		},
	}
}

func (m *MLServerSpec) getImage(config *InferenceServicesConfig) string {
	return fmt.Sprintf("%s:%s", config.Predictors.MLServer.ContainerImage, *m.RuntimeVersion)
}

func (m *MLServerSpec) GetStorageUri() *string {
	return m.StorageURI
}
