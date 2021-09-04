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

func TestFrameworkLightGBM(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    LightGBMSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: LightGBMSpec{
				RuntimeVersion: DefaultLightGBMRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Predictors: &PredictorsConfig{
				LightGBM: PredictorConfig{
					ContainerImage:      "kfserving/lgbserver",
					DefaultImageVersion: "latest",
				},
			},
		}
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}

func TestCreateLightGBMContainer(t *testing.T) {
	var requestedResource = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			"cpu": resource.MustParse("100m"),
		},
		Requests: v1.ResourceList{
			"cpu": resource.MustParse("90m"),
		},
	}
	var config = InferenceServicesConfig{
		Predictors: &PredictorsConfig{
			LightGBM: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.1.0",
			},
		},
	}
	var spec = LightGBMSpec{
		StorageURI:     "gs://someUri",
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
			"--model_dir=/mnt/models",
			"--http_port=8080",
			"--nthread=0",
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
			"--model_dir=/mnt/models",
			"--http_port=8080",
			"--nthread=0",
			"--workers=1",
		},
	}

	containerWithPar := spec.GetContainer("someName", 1, &config)
	g.Expect(containerWithPar).To(gomega.Equal(expectedParallelism))
}

func TestCreateLightGBMContainerWithNThread(t *testing.T) {
	var config = InferenceServicesConfig{
		Predictors: &PredictorsConfig{
			LightGBM: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.1.0",
			},
		},
	}
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		nthread     int
		resourceReq v1.ResourceRequirements
		expArgs     []string
	}{
		"TestNThread": {
			nthread: 4,
			resourceReq: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					"cpu": resource.MustParse("1100m"),
				},
				Requests: v1.ResourceList{
					"cpu": resource.MustParse("1100m"),
				},
			},
			expArgs: []string{
				"--model_name=someName",
				"--model_dir=/mnt/models",
				"--http_port=8080",
				"--nthread=4",
			},
		},
	}

	// Test Create with config
	for name, scenario := range scenarios {
		spec := LightGBMSpec{
			StorageURI:     "gs://someUri",
			Resources:      scenario.resourceReq,
			RuntimeVersion: "0.1.0",
			NThread:        scenario.nthread,
		}
		container := spec.GetContainer("someName", 0, &config)

		expContainer := &v1.Container{
			Image:     "someOtherImage:0.1.0",
			Name:      constants.InferenceServiceContainerName,
			Resources: scenario.resourceReq,
			Args:      scenario.expArgs,
		}
		g.Expect(container).To(gomega.Equal(expContainer), fmt.Sprintf("Testing %s", name))
	}
}
