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

package v1alpha2

import (
	"fmt"
	"testing"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestFrameworkPytorch(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    PyTorchSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PyTorchSpec{
				RuntimeVersion: DefaultPyTorchRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
		"RejectGPUResourcesExcluded": {
			spec: PyTorchSpec{
				RuntimeVersion: DefaultPyTorchRuntimeVersionGPU,
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidPyTorchRuntimeExcludesGPU)),
		},
		"RejectGPUResourcesIncluded": {
			spec: PyTorchSpec{
				RuntimeVersion: DefaultPyTorchRuntimeVersion,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
				},
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidPyTorchRuntimeIncludesGPU)),
		},
	}

	config := &InferenceServicesConfig{
		Predictors: &PredictorsConfig{
			PyTorch: PredictorConfig{
				ContainerImage:      "kfserving/pytorchserver",
				DefaultImageVersion: "0.1.0",
			},
		},
	}
	for name, scenario := range scenarios {
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}

func TestCreatePytorchModelServingContainer(t *testing.T) {

	var requestedResource = v1.ResourceRequirements{
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
	var config = InferenceServicesConfig{
		Predictors: &PredictorsConfig{
			PyTorch: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.1.0",
			},
		},
	}
	var spec = PyTorchSpec{
		StorageURI:     "gs://someUri",
		ModelClassName: "Net",
		Resources:      requestedResource,
		RuntimeVersion: "0.1.0",
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "someOtherImage:0.1.0",
		Name:      constants.InferenceServiceContainerName,
		Resources: requestedResource,
		Args: []string{
			"--model_name=someName",
			"--model_class_name=Net",
			"--model_dir=/mnt/models",
			"--http_port=8080",
		},
	}

	// Test Create with config
	container := spec.GetContainer("someName", 0, &config)
	g.Expect(container).To(gomega.Equal(expectedContainer))

	// Test Parallelism
	expectedParallelism := &v1.Container{
		Image:     "someOtherImage:0.1.0",
		Name:      constants.InferenceServiceContainerName,
		Resources: requestedResource,
		Args: []string{
			"--model_name=someName",
			"--model_class_name=Net",
			"--model_dir=/mnt/models",
			"--http_port=8080",
			"--workers=1",
		},
	}

	containerWithPar := spec.GetContainer("someName", 1, &config)
	g.Expect(containerWithPar).To(gomega.Equal(expectedParallelism))
}
