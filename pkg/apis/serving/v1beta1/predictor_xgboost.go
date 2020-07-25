package v1beta1

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
)

// SKLearnSpec defines arguments for configuring SKLearn model serving.
type XGBoostSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if invalid
func (x *XGBoostSpec) Validate() error {
	return nil
}

// Default sets defaults on the resource
func (x *XGBoostSpec) Default(config *InferenceServicesConfig) {
	x.Container.Name = constants.InferenceServiceContainerName
	if x.RuntimeVersion == "" {
		x.RuntimeVersion = config.Predictors.Xgboost.DefaultGpuImageVersion
	}
	setResourceRequirementDefaults(&x.Resources)
}

// GetContainer transforms the resource into a container spec
func (x *XGBoostSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, modelName),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
	}
	/*if parallelism != 0 {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.Itoa(parallelism)))
	}*/
	if x.Container.Image == "" {
		x.Container.Image = config.Predictors.SKlearn.ContainerImage + ":" + x.RuntimeVersion
	}
	x.Container.Name = constants.InferenceServiceContainerName
	x.Container.Args = arguments
	return &x.Container
}

func (k *XGBoostSpec) GetStorageUri() *string {
	return k.StorageURI
}
