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

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

var (
	// For versioning see https://github.com/triton-inference-server/server/releases
	TritonISGRPCPort = int32(9000)
	TritonISRestPort = int32(8080)
)

func (t *TritonSpec) GetStorageUri() string {
	return t.StorageURI
}

func (t *TritonSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &t.Resources
}

func (t *TritonSpec) GetContainer(modelName string, parallelism int, config *InferenceServicesConfig) *v1.Container {
	return &v1.Container{
		Image:     config.Predictors.Triton.ContainerImage + ":" + t.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: t.Resources,
		Args: []string{
			"trtserver",
			"--model-store=" + constants.DefaultModelLocalMountPath,
			"--allow-poll-model-repository=false",
			"--allow-grpc=true",
			"--allow-http=true",
			"--grpc-port=" + fmt.Sprint(TritonISGRPCPort),
			"--http-port=" + fmt.Sprint(TritonISRestPort),
		},
	}
}

func (t *TritonSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if t.RuntimeVersion == "" {
		t.RuntimeVersion = config.Predictors.Triton.DefaultImageVersion
	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *TritonSpec) Validate(config *InferenceServicesConfig) error {
	return nil
}
