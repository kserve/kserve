package v1alpha2

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	"k8s.io/api/core/v1"
	"strings"
)

var (
	InvalidAlibiRuntimeVersionError = "Alibi RuntimeVersion must be one of %s"
)

func (s *AlibiExplainerSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *AlibiExplainerSpec) CreateExplainerContainer(modelName string, predictorHost string, config *InferenceServicesConfig) *v1.Container {
	var args = []string{
		constants.ArgumentModelName, modelName,
		constants.ArgumentPredictorHost, predictorHost,
	}

	if s.StorageURI != "" {
		args = append(args, "--storage_uri", constants.DefaultModelLocalMountPath)
	}

	args = append(args, string(s.Type))

	for k, v := range s.Config {
		arg := "--" + k + "=" + v
		args = append(args, arg)
	}

	return &v1.Container{
		Image:     config.Explainers.AlibiExplainer.ContainerImage + ":" + s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args:      args,
	}
}

func (s *AlibiExplainerSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Explainers.AlibiExplainer.DefaultImageVersion
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *AlibiExplainerSpec) Validate(config *InferenceServicesConfig) error {
	if !utils.Includes(config.Explainers.AlibiExplainer.AllowedImageVersions, s.RuntimeVersion) {
		return fmt.Errorf(InvalidAlibiRuntimeVersionError, strings.Join(config.Explainers.AlibiExplainer.AllowedImageVersions, ", "))
	}

	return nil
}
