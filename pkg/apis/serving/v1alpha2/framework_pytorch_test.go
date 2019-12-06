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

func TestFrameworkPytorch(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	allowedPyTorchImageVersionsArray := []string{
		DefaultPyTorchRuntimeVersion,
	}
	allowedPyTorchImageVersions := strings.Join(allowedPyTorchImageVersionsArray, ", ")

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
		"RejectBadRuntimeVersion": {
			spec: PyTorchSpec{
				RuntimeVersion: "",
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidPyTorchRuntimeVersionError, allowedPyTorchImageVersions)),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
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
	container := spec.GetContainer("someName", &config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
