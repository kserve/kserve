package v1alpha2

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	"k8s.io/api/core/v1"
	"strings"
)

var (
	AlibiImageName                  = "docker.io/seldonio/alibiexplainer"
	InvalidAlibiRuntimeVersionError = "RuntimeVersion must be one of %s"
)

func (s *AlibiExplainerSpec) GetStorageUri() string {
	return s.StorageURI
}

func (s *AlibiExplainerSpec) CreateExplainerContainer(modelName string, predictorHost string, config *InferenceEndpointsConfigMap) *v1.Container {
	imageName := AlibiImageName
	if config.Explainers.AlibiExplainer.ContainerImage != "" {
		imageName = config.Explainers.AlibiExplainer.ContainerImage
	}

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
		Image:     imageName + ":" + s.RuntimeVersion,
		Resources: s.Resources,
		Args:      args,
	}
}

func (s *AlibiExplainerSpec) ApplyDefaults(config *InferenceEndpointsConfigMap) {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = config.Explainers.AlibiExplainer.DefaultImageVersion
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *AlibiExplainerSpec) Validate(config *InferenceEndpointsConfigMap) error {
	if !utils.Includes(config.Explainers.AlibiExplainer.AllowedImageVersions, s.RuntimeVersion) {
		return fmt.Errorf(InvalidAlibiRuntimeVersionError, strings.Join(config.Explainers.AlibiExplainer.AllowedImageVersions, ", "))
	}

	return nil
}
