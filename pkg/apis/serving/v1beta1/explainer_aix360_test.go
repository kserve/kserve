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
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAIXExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AIXExplainer: ExplainerConfig{
				ContainerImage:      "aipipeline/aixexplainer",
				DefaultImageVersion: "latest",
			},
		},
	}

	scenarios := map[string]struct {
		spec    AIXExplainerSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: AIXExplainerSpec{
				ExplainerExtensionSpec: ExplainerExtensionSpec{
					RuntimeVersion: proto.String("latest"),
				},
			},
			matcher: gomega.Succeed(),
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Default(&config)
			res := scenario.spec.Validate()
			if !g.Expect(res).To(scenario.matcher) {
				t.Errorf("got %q, want %q", res, scenario.matcher)
			}
		})
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
		Explainers: ExplainersConfig{
			AIXExplainer: ExplainerConfig{
				ContainerImage:      "aipipeline/aixexplainer",
				DefaultImageVersion: "latest",
			},
		},
	}
	ComponentExtensionSpec := ComponentExtensionSpec{
		MaxReplicas: 2,
	}
	var spec = AIXExplainerSpec{
		Type: "LimeImages",
		ExplainerExtensionSpec: ExplainerExtensionSpec{
			StorageURI: "gs://someUri",
			Container: v1.Container{
				Resources: requestedResource,
			},
			RuntimeVersion: proto.String("0.2.2"),
		},
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "aipipeline/aixexplainer:0.2.2",
		Name:      constants.InferenceServiceContainerName,
		Resources: requestedResource,
		Args: []string{
			constants.ArgumentModelName,
			"someName",
			constants.ArgumentHttpPort,
			constants.InferenceServiceDefaultHttpPort,
			constants.ArgumentPredictorHost,
			"someName-predictor-default.default",
			"--storage_uri",
			"/mnt/models",
			"--explainer_type",
			"LimeImages",
		},
	}

	// Test Create with config
	container := spec.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, &ComponentExtensionSpec, config)
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
		Explainers: ExplainersConfig{
			AIXExplainer: ExplainerConfig{
				ContainerImage:      "aipipeline/aixexplainer",
				DefaultImageVersion: "latest",
			},
		},
	}
	ComponentExtensionSpec := ComponentExtensionSpec{
		MaxReplicas: 2,
	}
	var spec = AIXExplainerSpec{
		Type: "LimeImages",
		ExplainerExtensionSpec: ExplainerExtensionSpec{
			StorageURI: "gs://someUri",
			Container: v1.Container{
				Resources: requestedResource,
			},
			RuntimeVersion: proto.String("0.2.2"),
			Config: map[string]string{
				"num_classes": "10",
				"num_samples": "20",
				"min_weight":  "0",
			},
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
			"--http_port",
			"8080",
			"--predictor_host",
			"someName-predictor-default.default",
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
	container := spec.GetContainer(metav1.ObjectMeta{Name: "someName", Namespace: "default"}, &ComponentExtensionSpec, config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}

func TestAIXExplainerIsMMS(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	config := InferenceServicesConfig{
		Explainers: ExplainersConfig{
			AIXExplainer: ExplainerConfig{
				ContainerImage:      "aipipeline/aixexplainer",
				DefaultImageVersion: "latest",
			},
		},
	}

	// Explainers do not have mms support
	mssCase := false
	scenarios := map[string]struct {
		spec     AIXExplainerSpec
		expected bool
	}{
		"AcceptGoodRuntimeVersion": {
			spec: AIXExplainerSpec{
				ExplainerExtensionSpec: ExplainerExtensionSpec{
					RuntimeVersion: proto.String("latest"),
				},
			},
			expected: mssCase,
		},
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario.spec.Default(&config)
			res := scenario.spec.IsMMS(&config)
			if !g.Expect(res).To(gomega.Equal(scenario.expected)) {
				t.Errorf("got %t, want %t", res, scenario.expected)
			}
		})
	}
}
