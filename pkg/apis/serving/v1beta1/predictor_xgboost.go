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
	"strconv"

	"github.com/kserve/kserve/pkg/constants"
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
)

// Default sets defaults on the resource
func (x *XGBoostSpec) Default(config *InferenceServicesConfig) {
	x.Container.Name = constants.InferenceServiceContainerName

	if x.ProtocolVersion == nil {
		defaultProtocol := constants.ProtocolV1
		x.ProtocolVersion = &defaultProtocol
	}

	setResourceRequirementDefaults(&x.Resources)
}

// nolint: unused
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

// nolint: unused
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

func (x *XGBoostSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *v1.Container {
	return &x.Container
}

func (x *XGBoostSpec) GetProtocol() constants.InferenceServiceProtocol {
	if x.ProtocolVersion != nil {
		return *x.ProtocolVersion
	} else {
		return constants.ProtocolV1
	}
}
