package v1alpha2

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

var (
	InvalidAIXRuntimeVersionError = "AIX RuntimeVersion must be one of %s"
)

func (s *AIXExplainerSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *AIXExplainerSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &s.Resources
}

func (s *AIXExplainerSpec) CreateExplainerContainer(modelName string, parallelism int, predictorHost string, config *InferenceServicesConfig) *v1.Container {
	var args = []string{
		constants.ArgumentModelName, modelName,
		constants.ArgumentPredictorHost, predictorHost,
		constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort,
	}
	if parallelism != 0 {
		args = append(args, constants.ArgumentWorkers, strconv.Itoa(parallelism))
	}
	if s.StorageURI != "" {
		args = append(args, "--storage_uri", constants.DefaultModelLocalMountPath)
	}

	args = append(args, "--explainer_type", string(s.Type))

	// Order explainer config map keys
	var keys []string
	for k, _ := range s.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--"+k)
		args = append(args, s.Config[k])
	}

	return &v1.Container{
		Image:     config.Explainers.AIXExplainer.ContainerImage + ":" + s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args:      args,
	}
}

func (s *AIXExplainerSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Explainers.AIXExplainer.DefaultImageVersion
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *AIXExplainerSpec) Validate(config *InferenceServicesConfig) error {
	if !utils.Includes(config.Explainers.AIXExplainer.AllowedImageVersions, s.RuntimeVersion) {
		return fmt.Errorf(InvalidAIXRuntimeVersionError, strings.Join(config.Explainers.AIXExplainer.AllowedImageVersions, ", "))
	}

	return nil
}