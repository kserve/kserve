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
	"testing"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

func TestFrameworkTensorflow(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    TensorflowSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: TensorflowSpec{
				RuntimeVersion: DefaultTensorflowRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
		"RejectGPUResourcesExcluded": {
			spec: TensorflowSpec{
				RuntimeVersion: DefaultTensorflowRuntimeVersionGPU,
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidTensorflowRuntimeExcludesGPU)),
		},
		"RejectGPUResourcesIncluded": {
			spec: TensorflowSpec{
				RuntimeVersion: DefaultTensorflowRuntimeVersion,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
				},
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidTensorflowRuntimeIncludesGPU)),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Predictors: &PredictorsConfig{
				Tensorflow: PredictorConfig{
					ContainerImage:         "tensorflow/serving",
					DefaultImageVersion:    "latest",
					DefaultGpuImageVersion: "latest-gpu",
				},
			},
		}
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}

func TestTensorflowContainer(t *testing.T) {

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
			Tensorflow: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.1.0",
			},
		},
	}
	var spec = TensorflowSpec{
		StorageURI:     "gs://someUri",
		Resources:      requestedResource,
		RuntimeVersion: "1.14",
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "someOtherImage:1.14",
		Name:      constants.InferenceServiceContainerName,
		Resources: requestedResource,
		Command:   []string{"/usr/bin/tensorflow_model_server"},
		Args: []string{
			"--port=" + TensorflowServingGRPCPort,
			"--rest_api_port=" + TensorflowServingRestPort,
			"--model_name=someName",
			"--model_base_path=/mnt/models",
			"--rest_api_timeout_in_ms=0",
		},
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/v1/models/someName",
				},
			},
			InitialDelaySeconds: constants.DefaultReadinessTimeout,
			PeriodSeconds:       10,
			FailureThreshold:    3,
			SuccessThreshold:    1,
			TimeoutSeconds:      1,
		},
	}

	// Test Create with config
	container := spec.GetContainer("someName", 0, &config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
