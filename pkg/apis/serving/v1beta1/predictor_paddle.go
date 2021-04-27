package v1beta1

import (
	"fmt"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PaddleServerSpec struct {
	PredictorExtensionSpec `json:",inline"`
}

func (p *PaddleServerSpec) Validate() error {
	// TODO: add GPU support
	return utils.FirstNonNilError([]error{
		validateStorageURI(p.GetStorageUri()),
	})
}

func (p *PaddleServerSpec) Default(config *InferenceServicesConfig) {
	// TODO: add GPU support
	p.Container.Name = constants.InferenceServiceContainerName
	if p.RuntimeVersion == nil {
		p.RuntimeVersion = proto.String(config.Predictors.Paddle.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&p.Resources)
}

// GetContainers transforms the resource into a container spec
func (p *PaddleServerSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, metadata.Name),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
	}

	if !utils.IncludesArg(p.Container.Args, constants.ArgumentWorkers) &&
		extensions.ContainerConcurrency != nil {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10)))
	}

	if p.Container.Image == "" {
		p.Container.Image = config.Predictors.Paddle.ContainerImage + ":" + *p.RuntimeVersion
	}
	p.Container.Name = constants.InferenceServiceContainerName
	p.Args = append(arguments, p.Args...)
	return &p.Container
}

func (p *PaddleServerSpec) GetStorageUri() *string {
	return p.StorageURI
}

func (p *PaddleServerSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (p *PaddleServerSpec) IsMMS(config *InferenceServicesConfig) bool {
	return config.Predictors.Paddle.MultiModelServer
}

func (p *PaddleServerSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	supportedFrameworks := config.Predictors.Paddle.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}
