/*
Copyright 2021 The KServe Authors.

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
	"fmt"
	"strconv"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// XGBoostSpec defines arguments for configuring XGBoost model serving.
type XGBoostSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var (
	_ ComponentImplementation = &XGBoostSpec{}
	_ PredictorImplementation = &XGBoostSpec{}
)

// Validate returns an error if invalid
func (x *XGBoostSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(x.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (x *XGBoostSpec) Default(config *InferenceServicesConfig) {
	x.Container.Name = constants.InferenceServiceContainerName

	if x.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		x.ProtocolVersion = &defaultProtocol
	}

	if x.RuntimeVersion == nil {
		defaultVersion := config.Predictors.XGBoost.V1.DefaultImageVersion
		if x.ProtocolVersion != nil && *x.ProtocolVersion == constants.ProtocolV2 {
			defaultVersion = config.Predictors.XGBoost.V2.DefaultImageVersion
		}

		x.RuntimeVersion = &defaultVersion
	}

	setResourceRequirementDefaults(&x.Resources)

}

// GetContainer transforms the resource into a container spec
func (x *XGBoostSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	if x.ProtocolVersion == nil || *x.ProtocolVersion == constants.ProtocolV1 {
		return x.getContainerV1(metadata, extensions, config)
	}

	return x.getContainerV2(metadata, extensions, config)
}

func (x *XGBoostSpec) getContainerV1(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	cpuLimit := x.Resources.Limits.Cpu()
	cpuLimit.RoundUp(0)
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, metadata.Name),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
		fmt.Sprintf("%s=%s", "--nthread", strconv.Itoa(int(cpuLimit.Value()))),
	}
	if !utils.IncludesArg(x.Container.Args, constants.ArgumentWorkers) {
		if extensions.ContainerConcurrency != nil {
			arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10)))
		}
	}

	if x.Container.Image == "" {
		x.Container.Image = config.Predictors.XGBoost.V1.ContainerImage + ":" + *x.RuntimeVersion
	}

	x.Container.Name = constants.InferenceServiceContainerName
	x.Container.Args = append(arguments, x.Container.Args...)
	return &x.Container
}

func (x *XGBoostSpec) getContainerV2(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	x.Container.Env = append(
		x.Container.Env,
		x.getEnvVarsV2()...,
	)

	// Append fallbacks for model settings
	x.Container.Env = append(
		x.Container.Env,
		x.getDefaultsV2(metadata)...,
	)

	if x.Container.Image == "" {
		x.Container.Image = config.Predictors.XGBoost.V2.ContainerImage + ":" + *x.RuntimeVersion
	}

	return &x.Container
}

func (x *XGBoostSpec) getEnvVarsV2() []v1.EnvVar {
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

func (x *XGBoostSpec) getDefaultsV2(metadata metav1.ObjectMeta) []v1.EnvVar {
	// These env vars set default parameters that can always be overridden
	// individually through `model-settings.json` config files.
	// These will be used as fallbacks for any missing properties and / or to run
	// without a `model-settings.json` file in place.
	vars := []v1.EnvVar{
		{
			Name:  constants.MLServerModelImplementationEnv,
			Value: constants.MLServerXGBoostImplementation,
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

func (x *XGBoostSpec) GetStorageUri() *string {
	return x.StorageURI
}

func (x *XGBoostSpec) GetProtocol() constants.InferenceServiceProtocol {
	if x.ProtocolVersion != nil {
		return *x.ProtocolVersion
	} else {
		return constants.ProtocolV1
	}
}

func (x *XGBoostSpec) IsMMS(config *InferenceServicesConfig) bool {
	predictorConfig := x.getPredictorConfig(config)
	return predictorConfig.MultiModelServer
}

func (x *XGBoostSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	predictorConfig := x.getPredictorConfig(config)
	supportedFrameworks := predictorConfig.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}

func (x *XGBoostSpec) getPredictorConfig(config *InferenceServicesConfig) *PredictorConfig {
	protocol := x.GetProtocol()
	if protocol == constants.ProtocolV1 {
		return config.Predictors.XGBoost.V1
	} else {
		return config.Predictors.XGBoost.V2
	}
}
