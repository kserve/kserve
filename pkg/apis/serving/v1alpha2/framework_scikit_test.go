package v1alpha2

import (
	"fmt"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
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
		config := &InferenceEndpointsConfigMap{
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
