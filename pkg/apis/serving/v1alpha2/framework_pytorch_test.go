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

func TestFrameworkPytorch(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	allowedPyTorchImageVersionsArray := []string{
		DefaultPytorchRuntimeVersion,
	}
	allowedPyTorchImageVersions := strings.Join(allowedPyTorchImageVersionsArray, ", ")

	scenarios := map[string]struct {
		spec    PyTorchSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: PyTorchSpec{
				RuntimeVersion: DefaultSKLearnRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
		"RejectBadRuntimeVersion": {
			spec: PyTorchSpec{
				RuntimeVersion: "",
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidPyTorchRuntimeVersionError, allowedPyTorchImageVersions)),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceEndpointsConfigMap{
			Predictors: &PredictorsConfig{
				PyTorch: PredictorConfig{
					ContainerImage:       "kfserving/pytorchserver",
					DefaultImageVersion:  "latest",
					AllowedImageVersions: allowedPyTorchImageVersionsArray,
				},
			},
		}
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
	var config = InferenceEndpointsConfigMap{
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
		Resources: requestedResource,
		Args: []string{
			"--model_name=someName",
			"--model_class_name=Net",
			"--model_dir=/mnt/models",
		},
	}

	// Test Create with config
	container := spec.GetContainer("someName", &config)
	g.Expect(container).To(gomega.Equal(expectedContainer))

	// Test Create without config
	expectedContainer.Image = "gcr.io/kfserving/pytorchserver:0.1.0"
	emptyConfig := InferenceEndpointsConfigMap{
		Predictors: &PredictorsConfig{
			PyTorch: PredictorConfig{},
		},
	}
	container = spec.GetContainer("someName", &emptyConfig)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
