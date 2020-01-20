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

func TestAlibiExplainer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	allowedAlibiImageVersionsArray := []string{
		DefaultAlibiExplainerRuntimeVersion,
	}
	allowedAlibiImageVersions := strings.Join(allowedAlibiImageVersionsArray, ", ")

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
		"RejectBadRuntimeVersion": {
			spec: AlibiExplainerSpec{
				RuntimeVersion: "",
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidAlibiRuntimeVersionError, allowedAlibiImageVersions)),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Explainers: &ExplainersConfig{
				AlibiExplainer: ExplainerConfig{
					ContainerImage:       "seldon.io/alibi",
					DefaultImageVersion:  "latest",
					AllowedImageVersions: allowedAlibiImageVersionsArray,
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
			constants.ArgumentWorkers,
			"0",
			"--storage_uri",
			"/mnt/models",
			"Anchor",
		},
	}

	// Test Create with config
	container := spec.CreateExplainerContainer("someName", "predictor.svc.cluster.local", config)
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
			"--workers",
			"0",
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
	container := spec.CreateExplainerContainer("someName", "predictor.svc.cluster.local", config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
