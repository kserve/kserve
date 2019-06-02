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
	InvalidModelRepositoryURIError  = "Model repository URI must be prefixed by gs:// (only Google Cloud Storage paths are supported)"
)

func (t *TensorRTSpec) CreateModelServingContainer(modelName string) *v1.Container {
	// based on example at: https://github.com/NVIDIA/tensorrt-laboratory/blob/master/examples/Deployment/Kubernetes/basic-trtis-deployment/deploy.yml
	return &v1.Container{
		Image:     TensorRTISImageName + ":" + t.RuntimeVersion,
		Command:   []string{TensorRTISEntrypointCommand},
		Resources: t.Resources,
		Args: []string{
			"--model-store=" + t.ModelRepositoryURI,
		},
		Ports: []v1.ContainerPort{
			v1.ContainerPort{
				Name:          "http",
				ContainerPort: 8000,
			},
			v1.ContainerPort{
				Name:          "grpc",
				ContainerPort: 8001,
			},
		},
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/api/health/live",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       5,
		},
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/api/health/ready",
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       5,
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
	if !strings.HasPrefix(t.ModelRepositoryURI, "gs://") {
		return fmt.Errorf(InvalidModelRepositoryURIError)
	}
	return nil
}
