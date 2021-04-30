/*
Copyright 2020 kubeflow.org.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"strconv"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CatBoostSpec defines arguments for configuring CatBoost model serving.
type CatBoostSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var (
	_ ComponentImplementation = &CatBoostSpec{}
	_ PredictorImplementation = &CatBoostSpec{}
)

// Validate returns an error if invalid
func (x *CatBoostSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(x.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (x *CatBoostSpec) Default(config *InferenceServicesConfig) {
	x.Container.Name = constants.InferenceServiceContainerName

	if x.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV2
		x.ProtocolVersion = &defaultProtocol
	}

	if x.RuntimeVersion == nil {
		defaultVersion := config.Predictors.CatBoost.DefaultImageVersion
		x.RuntimeVersion = &defaultVersion
	}

	setResourceRequirementDefaults(&x.Resources)

}

// GetContainer transforms the resource into a container spec
func (x *CatBoostSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	return x.getContainerV2(metadata, extensions, config)
}

func (x *CatBoostSpec) getContainerV2(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	x.Container.Env = append(
		x.Container.Env,
		x.getEnvVars()...,
	)

	// Append fallbacks for model settings
	x.Container.Env = append(
		x.Container.Env,
		x.getDefaults(metadata)...,
	)

	if x.Container.Image == "" {
		x.Container.Image = config.Predictors.CatBoost.ContainerImage + ":" + *x.RuntimeVersion
	}

	return &x.Container
}

func (x *CatBoostSpec) getEnvVars() []v1.EnvVar {
	vars := []v1.EnvVar{
		{
			Name:  constants.MLServerHTTPPortEnv,
			Value: strconv.Itoa(int(constants.MLServerISRestPort)),
		},
		{
			Name:  constants.MLServerGRPCPortEnv,
			Value: strconv.Itoa(int(constants.MLServerISGRPCPort)),
		},
		{
			Name:  constants.MLServerModelsDirEnv,
			Value: constants.DefaultModelLocalMountPath,
		},
	}

	if x.StorageURI == nil {
		vars = append(
			vars,
			v1.EnvVar{
				Name:  constants.MLServerLoadModelsStartupEnv,
				Value: strconv.FormatBool(false),
			},
		)
	}

	return vars
}

func (x *CatBoostSpec) getDefaults(metadata metav1.ObjectMeta) []v1.EnvVar {
	// These env vars set default parameters that can always be overriden
	// individually through `model-settings.json` config files.
	// These will be used as fallbacks for any missing properties and / or to run
	// without a `model-settings.json` file in place.
	vars := []v1.EnvVar{
		{
			Name:  constants.MLServerModelImplementationEnv,
			Value: constants.MLServerCatBoostImplementation,
		},
	}

	if x.StorageURI != nil {
		// These env vars only make sense as a default for non-MMS servers
		vars = append(
			vars,
			v1.EnvVar{
				Name:  constants.MLServerModelNameEnv,
				Value: metadata.Name,
			},
			v1.EnvVar{
				Name:  constants.MLServerModelURIEnv,
				Value: constants.DefaultModelLocalMountPath,
			},
		)
	}

	return vars
}

func (x *CatBoostSpec) GetStorageUri() *string {
	return x.StorageURI
}

func (x *CatBoostSpec) GetProtocol() constants.InferenceServiceProtocol {
	if x.ProtocolVersion != nil {
		return *x.ProtocolVersion
	} else {
		return constants.ProtocolV2
	}
}

func (x *CatBoostSpec) IsMMS(config *InferenceServicesConfig) bool {
	predictorConfig := x.getPredictorConfig(config)
	return predictorConfig.MultiModelServer
}

func (x *CatBoostSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	predictorConfig := x.getPredictorConfig(config)
	supportedFrameworks := predictorConfig.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}

func (x *CatBoostSpec) getPredictorConfig(config *InferenceServicesConfig) *PredictorConfig {
	return &config.Predictors.CatBoost
}
