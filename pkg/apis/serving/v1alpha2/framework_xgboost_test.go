package v1alpha2

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strings"
	"testing"
)

func TestFrameworkXgBoost(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	allowedXgBoostImageVersionsArray := []string{
		DefaultXGBoostRuntimeVersion,
	}
	allowedXGBoostImageVersions := strings.Join(allowedXgBoostImageVersionsArray, ", ")

	scenarios := map[string]struct {
		spec    XGBoostSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: XGBoostSpec{
				RuntimeVersion: DefaultXGBoostRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
		"RejectBadRuntimeVersion": {
			spec: XGBoostSpec{
				RuntimeVersion: "",
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidXGBoostRuntimeVersionError, allowedXGBoostImageVersions)),
		},
	}

	for name, scenario := range scenarios {
		config := &InferenceServicesConfig{
			Predictors: &PredictorsConfig{
				Xgboost: PredictorConfig{
					ContainerImage:       "kfserving/xgboostserver",
					DefaultImageVersion:  "latest",
					AllowedImageVersions: allowedXgBoostImageVersionsArray,
				},
			},
		}
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}

func TestCreateXGBoostContainer(t *testing.T) {

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
			Xgboost: PredictorConfig{
				ContainerImage:      "someOtherImage",
				DefaultImageVersion: "0.1.0",
			},
		},
	}
	var spec = XGBoostSpec{
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
		},
	}

	// Test Create with config
	container := spec.GetContainer("someName", false, &config)
	g.Expect(container).To(gomega.Equal(expectedContainer))
}
