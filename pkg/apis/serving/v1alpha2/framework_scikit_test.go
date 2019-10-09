package v1alpha2

import (
	"fmt"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strings"
	"testing"
)

func TestFrameworkSKLearn(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	allowedSKLearnImageVersionsArray := []string{
		DefaultSKLearnRuntimeVersion,
	}
	allowedSKLearnImageVersions := strings.Join(allowedSKLearnImageVersionsArray, ", ")

	scenarios := map[string]struct {
		spec    SKLearnSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: SKLearnSpec{
				RuntimeVersion: DefaultSKLearnRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
		"RejectBadRuntimeVersion": {
			spec: SKLearnSpec{
				RuntimeVersion: "",
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidSKLearnRuntimeVersionError, allowedSKLearnImageVersions)),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Predictors: &PredictorsConfig{
				SKlearn: PredictorConfig{
					ContainerImage:       "kfserving/sklearnserver",
					DefaultImageVersion:  "latest",
					AllowedImageVersions: allowedSKLearnImageVersionsArray,
				},
			},
		}
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}

func TestCreateSKLearnModelServingContainer(t *testing.T) {

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
			SKlearn: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.1.0",
			},
		},
	}
	var spec = SKLearnSpec{
		StorageURI:     "gs://someUri",
		Resources:      requestedResource,
		RuntimeVersion: "0.1.0",
	}
	g := gomega.NewGomegaWithT(t)

	expectedContainer := &v1.Container{
		Image:     "someOtherImage:0.1.0",
		Resources: requestedResource,
		Args: []string{
			"--model_name=someName",
			"--model_dir=/mnt/models",
		},
	}

	// Test Create with config
	container := spec.GetContainer("someName", &config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
