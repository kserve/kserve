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
	"net/url"
	"path"

	"strconv"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

var (
	ONNXServingRestPort            = "8080"
	ONNXServingGRPCPort            = "9000"
	ONNXFileExt                    = ".onnx"
	DefaultONNXFileName            = "model.onnx"
	InvalidONNXRuntimeVersionError = "ONNX RuntimeVersion must be one of %s"
)

func (s *ONNXSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *ONNXSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &s.Resources
}

func (s *ONNXSpec) GetContainer(modelName string, parallelism int, config *InferenceServicesConfig) *v1.Container {
	uri, _ := url.Parse(s.StorageURI)
	var filename string
	if ext := path.Ext(uri.Path); ext == "" {
		filename = DefaultONNXFileName
	} else {
		filename = path.Base(uri.Path)
	}
	arguments := []string{
		"--model_path", constants.DefaultModelLocalMountPath + "/" + filename,
		"--http_port", ONNXServingRestPort,
		"--grpc_port", ONNXServingGRPCPort,
	}
	if parallelism != 0 {
		arguments = append(arguments, []string{"--num_http_threads", strconv.Itoa(parallelism)}...)
	}
	return &v1.Container{
		Image:     config.Predictors.ONNX.ContainerImage + ":" + s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args:      arguments,
	}
}

func (s *ONNXSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Predictors.ONNX.DefaultImageVersion
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *ONNXSpec) Validate(config *InferenceServicesConfig) error {
	uri, err := url.Parse(s.StorageURI)
	if err != nil {
		return err
	}
	if ext := path.Ext(uri.Path); ext != ONNXFileExt && ext != "" {
		return fmt.Errorf("Expected storageUri file extension: %s but got %s", ONNXFileExt, ext)
	}
	return nil
}
