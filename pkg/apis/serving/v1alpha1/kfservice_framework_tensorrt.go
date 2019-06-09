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

package v1alpha1

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	TensorRTISImageName             = "nvcr.io/nvidia/tensorrtserver"
	DefaultTensorRTISRuntimeVersion = "19.05-py3"
	TensorRTISEntrypointCommand     = "trtserver"
	InvalidModelURIError            = "Model URI must be prefixed by gs:// (only Google Cloud Storage paths are supported)"
	TensorRTISGRPCPort              = int32(9000)
	TensorRTISRestPort              = int32(8080)
)

func (t *TensorRTSpec) CreateModelServingContainer(modelName string) *v1.Container {
	// based on example at: https://github.com/NVIDIA/tensorrt-laboratory/blob/master/examples/Deployment/Kubernetes/basic-trtis-deployment/deploy.yml
	return &v1.Container{
		Image:     TensorRTISImageName + ":" + t.RuntimeVersion,
		Command:   []string{TensorRTISEntrypointCommand},
		Resources: t.Resources,
		Args: []string{
			"--model-store=" + t.ModelURI,
			"--allow-poll-model-repository=false",
			"--allow-grpc=true",
			"--allow-http=true",
			"--grpc-port=" + fmt.Sprint(TensorRTISGRPCPort),
			"--rest-port=" + fmt.Sprint(TensorRTISRestPort),
		},
		Ports: []v1.ContainerPort{
			v1.ContainerPort{
				Name:          "http",
				ContainerPort: TensorRTISRestPort,
			},
			v1.ContainerPort{
				Name:          "grpc",
				ContainerPort: TensorRTISGRPCPort,
			},
		},
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/api/health/live",
					Port: intstr.FromString("http"),
				},
			},
		},
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/api/health/ready",
					Port: intstr.FromString("http"),
				},
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
	// TODO: support other sources (https://github.com/kubeflow/kfserving/issues/137)
	if !strings.HasPrefix(t.ModelURI, "gs://") {
		return fmt.Errorf(InvalidModelURIError)
	}
	return nil
}
