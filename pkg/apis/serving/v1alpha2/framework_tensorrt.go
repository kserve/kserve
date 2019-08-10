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
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

var (
	DefaultTensorRTISImageName = "nvcr.io/nvidia/tensorrtserver"
	// For versioning see https://github.com/NVIDIA/tensorrt-inference-server/releases
	DefaultTensorRTISRuntimeVersion  = "19.05-py3"
	AllowedTensorRTISRuntimeVersions = []string{
		"19.05-py3",
	}
	InvalidTensorRTISRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedTensorRTISRuntimeVersions, ", ")
	TensorRTISGRPCPort                   = int32(9000)
	TensorRTISRestPort                   = int32(8080)
)

func (t *TensorRTSpec) GetModelSourceUri() string {
	return t.ModelURI
}

func (t *TensorRTSpec) CreateModelServingContainer(modelName string, config *FrameworksConfig) *v1.Container {
	imageName := DefaultTensorRTISImageName
	if config.TensorRT.ContainerImage != "" {
		imageName = config.TensorRT.ContainerImage
	}

	// based on example at: https://github.com/NVIDIA/tensorrt-laboratory/blob/master/examples/Deployment/Kubernetes/basic-trtis-deployment/deploy.yml
	return &v1.Container{
		Image:     imageName + ":" + t.RuntimeVersion,
		Resources: t.Resources,
		Args: []string{
			"trtserver",
			"--model-store=" + constants.DefaultModelLocalMountPath,
			"--allow-poll-model-repository=false",
			"--allow-grpc=true",
			"--allow-http=true",
			"--grpc-port=" + fmt.Sprint(TensorRTISGRPCPort),
			"--http-port=" + fmt.Sprint(TensorRTISRestPort),
		},
		Ports: []v1.ContainerPort{
			{
				ContainerPort: TensorRTISRestPort,
			},
		},
	}
}

func (t *TensorRTSpec) ApplyDefaults() {
	if t.RuntimeVersion == "" {
		t.RuntimeVersion = DefaultTensorRTISRuntimeVersion
	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *TensorRTSpec) Validate() error {
	if !utils.Includes(AllowedTensorRTISRuntimeVersions, t.RuntimeVersion) {
		return fmt.Errorf(InvalidTensorRTISRuntimeVersionError)
	}
	if err := validateReplicas(t.MinReplicas, t.MaxReplicas); err != nil {
		return err
	}
	return nil
}
