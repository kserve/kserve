/*
Copyright 2019 kubeflow.org.

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
	"strings"
	"testing"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

const (
	DefaultTensorflowRuntimeVersion    = "latest"
	DefaultTensorflowRuntimeVersionGPU = "latest-gpu"
)

func TestFrameworkTensorflor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	allowedTFServingImageVersionsArray := []string{
		"latest",
		"latest-gpu",
	}
	allowedTFServingImageVersions := strings.Join(allowedTFServingImageVersionsArray, ", ")

	scenarios := map[string]struct {
		spec    TensorflowSpec
		matcher types.GomegaMatcher
	}{
		"AcceptGoodRuntimeVersion": {
			spec: TensorflowSpec{
				RuntimeVersion: DefaultTensorflowRuntimeVersion,
			},
			matcher: gomega.Succeed(),
		},
		"RejectBadRuntimeVersion": {
			spec: TensorflowSpec{
				RuntimeVersion: "",
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidTensorflowRuntimeVersionError, allowedTFServingImageVersions)),
		},
		"RejectGPUResourcesExcluded": {
			spec: TensorflowSpec{
				RuntimeVersion: DefaultTensorflowRuntimeVersionGPU,
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidTensorflowRuntimeExcludesGPU, allowedTFServingImageVersions)),
		},
		"RejectGPUResourcesIncluded": {
			spec: TensorflowSpec{
				RuntimeVersion: DefaultTensorflowRuntimeVersion,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{constants.NvidiaGPUResourceType: resource.MustParse("1")},
				},
			},
			matcher: gomega.MatchError(fmt.Sprintf(InvalidTensorflowRuntimeIncludesGPU, allowedTFServingImageVersions)),
		},
	}

	for name, scenario := range scenarios {
		config := &PredictorsConfig{
			Tensorflow: PredictorConfig{
				ContainerImage:         "tensorflow/serving",
				DefaultImageVersion:    "latest",
				DefaultGpuImageVersion: "latest-gpu",
				AllowedImageVersions:   allowedTFServingImageVersionsArray,
			},
		}
		g.Expect(scenario.spec.Validate(config)).Should(scenario.matcher, fmt.Sprintf("Testing %s", name))
	}
}
