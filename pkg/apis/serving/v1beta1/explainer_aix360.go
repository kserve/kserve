package v1beta1

import (
	"sort"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

type AIXExplainerType string

const (
	AIXLimeImageExplainer AIXExplainerType = "LimeImages"
)

// AIXExplainerSpec defines the arguments for configuring an AIX Explanation Server
type AIXExplainerSpec struct {
	// The type of AIX explainer
	Type AIXExplainerType `json:"type"`
	// The location of a trained explanation model
	StorageURI string `json:"storageUri,omitempty"`
	// Defaults to latest AIX Version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Defaults to requests and limits of 1CPU, 2Gb MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
}

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

func (s *AIXExplainerSpec) Default(config *InferenceServicesConfig) {
	s.Name = constants.InferenceServiceContainerName
	if s.RuntimeVersion == nil {
		s.RuntimeVersion = proto.String(config.Explainers.AIXExplainer.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&s.Resources)
}
