package v1alpha2

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestAIXExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	allowedAIXImageVersionsArray := []string{
		DefaultAIXExplainerRuntimeVersion,
	}
	allowedAIXImageVersions := strings.Join(allowedAIXImageVersionsArray, ", ")

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
		"RejectBadRuntimeVersion": {
			spec: AIXExplainerSpec{
				RuntimeVersion: "",
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidAIXRuntimeVersionError, allowedAIXImageVersions)),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Explainers: &ExplainersConfig{
				AIXExplainer: ExplainerConfig{
					ContainerImage:       "aipipeline/aixexplainer",
					DefaultImageVersion:  "latest",
					AllowedImageVersions: allowedAIXImageVersionsArray,
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