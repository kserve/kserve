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
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kubeflow/kfserving/pkg/constants"
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
			"--model_path", "/mnt/models/someName/model.onnx",
			"--http_port", "8080",
			"--grpc_port", "9000",
		},
	}

	// Test Create with config
	container := onnxSpec.GetContainer("someName", 0, &onnxConfig)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
