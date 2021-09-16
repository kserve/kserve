/*

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
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

var onnxRequestedResource = v1.ResourceRequirements{
	Limits: v1.ResourceList{
		"cpu": resource.Quantity{
			Format: "100",
		},
	},
	Requests: v1.ResourceList{
		"cpu": resource.Quantity{
			Format: "90",
		},
	},
}

var onnxSpec = ONNXSpec{
	StorageURI:     "gs://someUri",
	Resources:      onnxRequestedResource,
	RuntimeVersion: "someAmazingVersion",
}

var onnxSpecWithOtherFilename = ONNXSpec{
	StorageURI:     "gs://someUri/other_name.onnx",
	Resources:      onnxRequestedResource,
	RuntimeVersion: "someAmazingVersion",
}

var onnxSpecWithQueryParameterFilename = ONNXSpec{
	StorageURI:     "https://someUri/other_name.onnx?raw=true",
	Resources:      onnxRequestedResource,
	RuntimeVersion: "someAmazingVersion",
}

var onnxSpecWithBadFileExt = ONNXSpec{
	StorageURI:     "gs://someUri/model.notonnx",
	Resources:      onnxRequestedResource,
	RuntimeVersion: "someUnAmazingVersion",
}

var onnxConfig = InferenceServicesConfig{
	Predictors: &PredictorsConfig{
		ONNX: PredictorConfig{
			ContainerImage: "someOtherImage",
		},
	},
}

func TestCreateOnnxModelServingContainer(t *testing.T) {

	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "someOtherImage:someAmazingVersion",
		Name:      constants.InferenceServiceContainerName,
		Resources: onnxRequestedResource,
		Args: []string{
			"--model_path", "/mnt/models/model.onnx",
			"--http_port", "8080",
			"--grpc_port", "9000",
		},
	}

	// Test Create with config
	container := onnxSpec.GetContainer("someName", 0, &onnxConfig)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}

func TestCreateOnnxModelServingContainerWithOtherFilename(t *testing.T) {

	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "someOtherImage:someAmazingVersion",
		Name:      constants.InferenceServiceContainerName,
		Resources: onnxRequestedResource,
		Args: []string{
			"--model_path", "/mnt/models/other_name.onnx",
			"--http_port", "8080",
			"--grpc_port", "9000",
		},
	}

	// Test Create with config
	container := onnxSpecWithOtherFilename.GetContainer("someName", 0, &onnxConfig)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}

func TestCreateOnnxModelServingContainerWithQueryParameter(t *testing.T) {

	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "someOtherImage:someAmazingVersion",
		Name:      constants.InferenceServiceContainerName,
		Resources: onnxRequestedResource,
		Args: []string{
			"--model_path", "/mnt/models/other_name.onnx",
			"--http_port", "8080",
			"--grpc_port", "9000",
		},
	}

	// Test Create with config
	container := onnxSpecWithQueryParameterFilename.GetContainer("someName", 0, &onnxConfig)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}

func TestOnnxPathValidationSuccess(t *testing.T) {

	g := gomega.NewGomegaWithT(t)
	g.Expect(onnxSpec.Validate(&onnxConfig)).Should(gomega.Succeed())
	g.Expect(onnxSpecWithOtherFilename.Validate(&onnxConfig)).Should(gomega.Succeed())
	g.Expect(onnxSpecWithQueryParameterFilename.Validate(&onnxConfig)).Should(gomega.Succeed())
}

func TestOnnxPathValidationFailure(t *testing.T) {

	g := gomega.NewGomegaWithT(t)
	g.Expect(onnxSpecWithBadFileExt.Validate(&onnxConfig)).ShouldNot(gomega.Succeed())
}
