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
	"strconv"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

var (
	InvalidXGBoostRuntimeVersionError = "XGBoost RuntimeVersion must be one of %s"
)

func (x *XGBoostSpec) GetStorageUri() string {
	return x.StorageURI
}

func (x *XGBoostSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &x.Resources
}

func (x *XGBoostSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	nthread := x.NThread
	if nthread == 0 {
		nthread = int(x.Resources.Requests.Cpu().Value())
	}

	return &v1.Container{
		Image:     config.Predictors.Xgboost.ContainerImage + ":" + x.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: x.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
			"--nthread=" + strconv.Itoa(nthread),
		},
	}
}

func (x *XGBoostSpec) ApplyDefaults(config *InferenceServicesConfig) {
	if x.RuntimeVersion == "" {
		x.RuntimeVersion = config.Predictors.Xgboost.DefaultImageVersion
	}

	setResourceRequirementDefaults(&x.Resources)
}

func (x *XGBoostSpec) Validate(config *InferenceServicesConfig) error {
	if utils.Includes(config.Predictors.Xgboost.AllowedImageVersions, x.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidXGBoostRuntimeVersionError, strings.Join(config.Predictors.Xgboost.AllowedImageVersions, ", "))
}
