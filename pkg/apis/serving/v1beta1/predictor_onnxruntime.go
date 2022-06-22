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

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	ONNXServingRestPort = "8080"
	ONNXServingGRPCPort = "9000"
	ONNXFileExt         = ".onnx"
	DefaultONNXFileName = "model.onnx"
)

// ONNXRuntimeSpec defines arguments for configuring ONNX model serving.
type ONNXRuntimeSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var (
	_ ComponentImplementation = &ONNXRuntimeSpec{}
	_ PredictorImplementation = &ONNXRuntimeSpec{}
)

// Validate returns an error if invalid
func (o *ONNXRuntimeSpec) Validate() error {
	if o.GetStorageUri() != nil {
		if ext := path.Ext(*o.GetStorageUri()); ext != ONNXFileExt && ext != "" {
			return fmt.Errorf("Expected storageUri file extension: '%s' but got '%s'", ONNXFileExt, ext)
		}
	}

	return utils.FirstNonNilError([]error{
		validateStorageURI(o.GetStorageUri()),
		validateStorageSpec(o.GetStorageSpec(), o.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (o *ONNXRuntimeSpec) Default(config *InferenceServicesConfig) {
	o.Container.Name = constants.InferenceServiceContainerName
	if o.RuntimeVersion == nil {
		o.RuntimeVersion = proto.String(config.Predictors.ONNX.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&o.Resources)
}

// GetContainers transforms the resource into a container spec
func (o *ONNXRuntimeSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	filename := DefaultONNXFileName

	if o.GetStorageUri() != nil {
		if ext := path.Ext(*o.GetStorageUri()); ext != "" {
			filename = path.Base(*o.GetStorageUri())
		}
	}

	arguments := []string{
		fmt.Sprintf("%s=%s", "--model_path", constants.DefaultModelLocalMountPath+"/"+filename),
		fmt.Sprintf("%s=%s", "--http_port", ONNXServingRestPort),
		fmt.Sprintf("%s=%s", "--grpc_port", ONNXServingGRPCPort),
	}

	if o.Container.Image == "" {
		o.Container.Image = config.Predictors.ONNX.ContainerImage + ":" + *o.RuntimeVersion
	}
	o.Name = constants.InferenceServiceContainerName
	o.Args = append(arguments, o.Args...)
	return &o.Container
}

func (o *ONNXRuntimeSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (o *ONNXRuntimeSpec) IsMMS(config *InferenceServicesConfig) bool {
	return config.Predictors.ONNX.MultiModelServer
}

func (o *ONNXRuntimeSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	supportedFrameworks := config.Predictors.ONNX.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}
