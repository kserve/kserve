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

func TestAIXExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    AIXExplainerSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: AIXExplainerSpec{
				RuntimeVersion: DefaultAIXExplainerRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Explainers: &ExplainersConfig{
				AIXExplainer: ExplainerConfig{
					ContainerImage:      "aipipeline/aixexplainer",
					DefaultImageVersion: "latest",
				},
			},
		}
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}

func TestCreateAIXExplainerContainer(t *testing.T) {

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
	config := &InferenceServicesConfig{
		Explainers: &ExplainersConfig{
			AIXExplainer: ExplainerConfig{
				ContainerImage:      "aipipeline/aixexplainer",
				DefaultImageVersion: "latest",
			},
		},
	}
	var spec = AIXExplainerSpec{
		Type:           "LimeImages",
		StorageURI:     "gs://someUri",
		Resources:      requestedResource,
		RuntimeVersion: "0.2.2",
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "aipipeline/aixexplainer:0.2.2",
		Name:      constants.InferenceServiceContainerName,
		Resources: requestedResource,
		Args: []string{
			constants.ArgumentModelName,
			"someName",
			constants.ArgumentPredictorHost,
			"predictor.svc.cluster.local",
			constants.ArgumentHttpPort,
			constants.InferenceServiceDefaultHttpPort,
			"--storage_uri",
			"/mnt/models",
			"--explainer_type",
			"LimeImages",
		},
	}

	// Test Create with config
	container := spec.CreateExplainerContainer("someName", 0, "predictor.svc.cluster.local", config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}

func TestCreateAIXExplainerContainerWithConfig(t *testing.T) {

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
	config := &InferenceServicesConfig{
		Explainers: &ExplainersConfig{
			AIXExplainer: ExplainerConfig{
				ContainerImage:      "aipipeline/aixexplainer",
				DefaultImageVersion: "latest",
			},
		},
	}
	var spec = AIXExplainerSpec{
		Type:           "LimeImages",
		StorageURI:     "gs://someUri",
		Resources:      requestedResource,
		RuntimeVersion: "0.2.2",
		Config: map[string]string{
			"num_classes": "10",
			"num_samples": "20",
			"min_weight":  "0",
		},
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "aipipeline/aixexplainer:0.2.2",
		Name:      constants.InferenceServiceContainerName,
		Resources: requestedResource,
		Args: []string{
			"--model_name",
			"someName",
			"--predictor_host",
			"predictor.svc.cluster.local",
			"--http_port",
			"8080",
			"--storage_uri",
			"/mnt/models",
			"--explainer_type",
			"LimeImages",
			"--min_weight",
			"0",
			"--num_classes",
			"10",
			"--num_samples",
			"20",
		},
	}

	// Test Create with config
	container := spec.CreateExplainerContainer("someName", 0, "predictor.svc.cluster.local", config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
