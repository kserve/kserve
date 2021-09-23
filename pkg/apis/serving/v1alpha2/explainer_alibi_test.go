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
	"fmt"
	"testing"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAlibiExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		spec    AlibiExplainerSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: AlibiExplainerSpec{
				RuntimeVersion: DefaultAlibiExplainerRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Explainers: &ExplainersConfig{
				AlibiExplainer: ExplainerConfig{
					ContainerImage:      "seldon.io/alibi",
					DefaultImageVersion: "latest",
				},
			},
		}
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}

func TestCreateAlibiExplainerContainer(t *testing.T) {

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
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "seldon.io/alibi",
				DefaultImageVersion: "latest",
			},
		},
	}
	var spec = AlibiExplainerSpec{
		Type:           "Anchor",
		StorageURI:     "gs://someUri",
		Resources:      requestedResource,
		RuntimeVersion: "0.1.0",
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "seldon.io/alibi:0.1.0",
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
			"Anchor",
		},
	}

	// Test Create with config
	container := spec.CreateExplainerContainer("someName", 0, "predictor.svc.cluster.local", config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}

func TestCreateAlibiExplainerContainerWithConfig(t *testing.T) {

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
			AlibiExplainer: ExplainerConfig{
				ContainerImage:      "seldon.io/alibi",
				DefaultImageVersion: "latest",
			},
		},
	}
	var spec = AlibiExplainerSpec{
		Type:           "AnchorText",
		StorageURI:     "gs://someUri",
		Resources:      requestedResource,
		RuntimeVersion: "0.1.0",
		Config: map[string]string{
			"threshold":    "0.95",
			"use_unk":      "False",
			"sample_proba": "0.5",
		},
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "seldon.io/alibi:0.1.0",
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
			"AnchorText",
			"--sample_proba",
			"0.5",
			"--threshold",
			"0.95",
			"--use_unk",
			"False",
		},
	}

	// Test Create with config
	container := spec.CreateExplainerContainer("someName", 0, "predictor.svc.cluster.local", config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
