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
	v1 "k8s.io/api/core/v1"
)

const (
	TensorflowEntrypointCommand = "/usr/bin/tensorflow_model_server"
	TensorflowServingGRPCPort   = "9000"
	TensorflowServingRestPort   = "8080"
	TensorflowServingImageName  = "tensorflow/serving"

	DefaultTensorflowServingVersion = "1.13.0"
)

func (t *TensorflowSpec) CreateModelServingContainer(modelName string) *v1.Container {
	//TODO(@yuzisun) add configmap for image, default resources, readiness/liveness probe
	return &v1.Container{
		Image:   TensorflowServingImageName + ":" + t.RuntimeVersion,
		Command: []string{TensorflowEntrypointCommand},
		Args: []string{
			"--port=" + TensorflowServingGRPCPort,
			"--rest_api_port=" + TensorflowServingRestPort,
			"--model_name=" + modelName,
			"--model_base_path=" + t.ModelURI,
		},
	}
}

func (t *TensorflowSpec) ApplyDefaults() {
	if t.RuntimeVersion == "" {
		t.RuntimeVersion = DefaultTensorflowServingVersion
	}

	setResourceRequirementDefaults(&t.Resources)
}

func (t *TensorflowSpec) Validate() error {
	// TODO: https://github.com/kubeflow/kfserving/issues/84
	return nil
}
