package v1alpha2

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"strings"
)

var (
	AlibiImageName              = "docker.io/seldonio/alibiexplainer"
	DefaultAlibiRuntimeVersion  = "0.2.3"
	AllowedAlibiRuntimeVersions = []string{
		"0.2.3",
	}
	InvalidAlibiRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedAlibiRuntimeVersions, ", ")
	ExplainerModelFileName          = "explainer.dill"
)

func (s *AlibiExplainerSpec) GetModelSourceUri() string {
	return s.StorageURI
}

func (s *AlibiExplainerSpec) CreateExplainerServingContainer(modelName string, predictUrl string, config *ExplainersConfig) *v1.Container {
	imageName := AlibiImageName
	if config.AlibiExplainer.ContainerImage != "" {
		imageName = config.AlibiExplainer.ContainerImage
	}

	var args = []string{
		"--explainer_name", modelName,
		"--predict_url", predictUrl,
		"--type", string(s.Type),
	}

	if s.StorageURI != "" {
		args = append(args, "--storage_uri", constants.DefaultModelLocalMountPath+"/"+ExplainerModelFileName)
	}

	return &v1.Container{
		Image:     imageName + ":" + s.RuntimeVersion,
		Resources: s.Resources,
		Args:      args,
	}
}

func (s *AlibiExplainerSpec) ApplyDefaults() {
	if s.RuntimeVersion == "" {
		s.RuntimeVersion = DefaultAlibiRuntimeVersion
	}
	setResourceRequirementDefaults(&s.Resources)
}

func (s *AlibiExplainerSpec) Validate() error {
	if !utils.Includes(AllowedAlibiRuntimeVersions, s.RuntimeVersion) {
		return fmt.Errorf(InvalidAlibiRuntimeVersionError)
	}

	return nil
}
