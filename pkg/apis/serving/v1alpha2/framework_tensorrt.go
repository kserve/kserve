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

	InvalidTensorRTISRuntimeVersionError = "RuntimeVersion must be one of %s"
	TensorRTISGRPCPort                   = int32(9000)
	TensorRTISRestPort                   = int32(8080)
)

func (t *TensorRTSpec) GetStorageUri() string {
	return t.StorageURI
}

func (t *TensorRTSpec) GetContainer(modelName string, config *PredictorsConfig) *v1.Container {
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
			v1.ContainerPort{
				ContainerPort: TensorRTISRestPort,
			},
		},
	}
}

func (t *TensorRTSpec) ApplyDefaults(config *PredictorsConfig) {
	if t.RuntimeVersion == "" {
		t.RuntimeVersion = config.TensorRT.DefaultImageVersion
	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *TensorRTSpec) Validate(config *PredictorsConfig) error {
	if !utils.Includes(config.TensorRT.AllowedImageVersions, t.RuntimeVersion) {
		return fmt.Errorf(InvalidTensorRTISRuntimeVersionError, strings.Join(config.TensorRT.AllowedImageVersions, ", "))
	}

	return nil
}
