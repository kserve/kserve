package v1beta1

import (
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

var (
	TensorflowEntrypointCommand          = "/usr/bin/tensorflow_model_server"
	TensorflowServingGRPCPort            = "9000"
	TensorflowServingRestPort            = "8080"
	TensorflowServingGPUSuffix           = "-gpu"
	InvalidTensorflowRuntimeVersionError = "Tensorflow RuntimeVersion must be one of %s"
	InvalidTensorflowRuntimeIncludesGPU  = "Tensorflow RuntimeVersion is not GPU enabled but GPU resources are requested. " + InvalidTensorflowRuntimeVersionError
	InvalidTensorflowRuntimeExcludesGPU  = "Tensorflow RuntimeVersion is GPU enabled but GPU resources are not requested. " + InvalidTensorflowRuntimeVersionError
)

// TFServingSpec defines arguments for configuring Tensorflow model serving.
type TFServingSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (t *TFServingSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TFServingSpec) Default() {}

func (t *TFServingSpec) GetStorageUri() *string {
	return t.StorageURI
}

// GetContainers transforms the resource into a container spec
func (t *TFServingSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		"--port=" + TensorflowServingGRPCPort,
		"--rest_api_port=" + TensorflowServingRestPort,
		"--model_name=" + modelName,
		"--model_base_path=" + constants.DefaultModelLocalMountPath,
	}
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.Tensorflow.ContainerImage
	}
	t.Container.Name = constants.InferenceServiceContainerName
	t.Container.Command = []string{TensorflowEntrypointCommand}
	t.Container.Args = arguments
	return &v1.Container{
		Name:    t.Container.Name,
		Image:   t.Container.Image,
		Args:    t.Container.Args,
		Command: t.Container.Command,
	}
}

