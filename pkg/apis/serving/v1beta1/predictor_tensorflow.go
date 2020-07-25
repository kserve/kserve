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

// TensorflowSpec defines arguments for configuring Tensorflow model serving.
type TensorflowSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (t *TensorflowSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (t *TensorflowSpec) Default(config *InferenceServicesConfig) {
	t.Container.Name = constants.InferenceServiceContainerName
	if t.RuntimeVersion == "" {
		if isGPUEnabled(t.Resources) {
			t.RuntimeVersion = config.Predictors.Tensorflow.DefaultGpuImageVersion
		} else {
			t.RuntimeVersion = config.Predictors.Tensorflow.DefaultImageVersion
		}
	}
	setResourceRequirementDefaults(&t.Resources)
}

func (t *TensorflowSpec) GetStorageUri() *string {
	return t.StorageURI
}

// GetContainers transforms the resource into a container spec
func (t *TensorflowSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		"--port=" + TensorflowServingGRPCPort,
		"--rest_api_port=" + TensorflowServingRestPort,
		"--model_name=" + modelName,
		"--model_base_path=" + constants.DefaultModelLocalMountPath,
	}
	if t.Container.Image == "" {
		t.Container.Image = config.Predictors.Tensorflow.ContainerImage + ":" + t.RuntimeVersion
	}
	t.Container.Command = []string{TensorflowEntrypointCommand}
	t.Container.Args = arguments
	return &t.Container
}
