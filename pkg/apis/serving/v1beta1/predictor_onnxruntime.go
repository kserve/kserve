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
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

// ONNXRuntimeSpec defines arguments for configuring ONNX model serving.
type ONNXRuntimeSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (o *ONNXRuntimeSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (o *ONNXRuntimeSpec) Default(config *InferenceServicesConfig) {
	o.Container.Name = constants.InferenceServiceContainerName
	if o.RuntimeVersion == "" {
		o.RuntimeVersion = config.Predictors.ONNX.DefaultGpuImageVersion
	}
	setResourceRequirementDefaults(&o.Resources)
}

// GetContainers transforms the resource into a container spec
func (o *ONNXRuntimeSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	return &v1.Container{}
}

func (o *ONNXRuntimeSpec) GetStorageUri() *string {
	return o.StorageURI
}
