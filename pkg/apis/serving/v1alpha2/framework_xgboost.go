/*
Copyright 2019 kubeflow.org.
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

package v1alpha2

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strconv"
)

func (x *XGBoostSpec) GetStorageUri() string {
	return x.StorageURI
}

func (x *XGBoostSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &x.Resources
}

func (x *XGBoostSpec) GetContainer(modelName string, parallelism int, config *InferenceServicesConfig) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, modelName),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
		fmt.Sprintf("%s=%s", "--nthread", strconv.Itoa(x.NThread)),
	}
	if parallelism != 0 {
		arguments = append(arguments, fmt.Sprintf("%s=%s", constants.ArgumentWorkers, strconv.Itoa(parallelism)))
	}
	return &v1.Container{
		Image:     config.Predictors.Xgboost.V1.ContainerImage + ":" + x.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: x.Resources,
		Args:      arguments,
	}
}

func (x *XGBoostSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if x.RuntimeVersion == "" {
		x.RuntimeVersion = config.Predictors.Xgboost.V1.DefaultImageVersion
	}

	setResourceRequirementDefaults(&x.Resources)
	if x.NThread == 0 {
		x.NThread = int(x.Resources.Requests.Cpu().Value())
	}
}

func (x *XGBoostSpec) Validate(config *InferenceServicesConfig) error {
	return nil
}
