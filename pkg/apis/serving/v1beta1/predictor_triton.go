package v1beta1

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

var (
	// For versioning see https://github.com/NVIDIA/triton-inference-server/releases
	TritonISGRPCPort = int32(9000)
	TritonISRestPort = int32(8080)
)

// TritonSpec defines arguments for configuring Triton model serving.
type TritonSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (t *TritonSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TritonSpec) Default() {}

// GetContainers transforms the resource into a container spec
func (t *TritonSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		"trtserver",
		fmt.Sprintf("%s=%s", "--model-store=", constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", "--grpc-port=", fmt.Sprint(TritonISGRPCPort)),
		fmt.Sprintf("%s=%s", "--http-port=", fmt.Sprint(TritonISRestPort)),
		"--allow-poll-model-repository=false",
		"--allow-grpc=true",
		"--allow-http=true",
	}
	t.Args = arguments
	t.Name = constants.InferenceServiceContainerName
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.Triton.ContainerImage + ":" + *t.RuntimeVersion
	}
	return &t.Container
}

func (t *TritonSpec) GetStorageUri() *string {
	return t.StorageURI
}
