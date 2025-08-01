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
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var ONNXFileExt = ".onnx"

// ONNXRuntimeSpec defines arguments for configuring ONNX model serving.
type ONNXRuntimeSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &ONNXRuntimeSpec{}

// Validate returns an error if invalid
func (o *ONNXRuntimeSpec) Validate() error {
	if o.GetStorageUri() != nil {
		if ext := path.Ext(*o.GetStorageUri()); ext != ONNXFileExt && ext != "" {
			return fmt.Errorf("expected storageUri file extension: '%s' but got '%s'", ONNXFileExt, ext)
		}
	}

	return utils.FirstNonNilError([]error{
		validateStorageSpec(o.GetStorageSpec(), o.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (o *ONNXRuntimeSpec) Default(config *InferenceServicesConfig) {
	o.Container.Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(config, &o.Resources)
}

// GetContainers transforms the resource into a container spec
func (o *ONNXRuntimeSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container {
	return &o.Container
}

func (o *ONNXRuntimeSpec) GetProtocol() constants.InferenceServiceProtocol {
	if o.ProtocolVersion != nil {
		return *o.ProtocolVersion
	}
	return constants.ProtocolV1
}
