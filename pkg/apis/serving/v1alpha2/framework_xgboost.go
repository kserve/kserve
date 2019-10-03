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
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
)

// TODO add image name to to configmap
var (
	AllowedXGBoostRuntimeVersions = []string{
		"latest",
		"v0.1.2",
	}
	InvalidXGBoostRuntimeVersionError = "RuntimeVersion must be one of " + strings.Join(AllowedXGBoostRuntimeVersions, ", ")
	XGBoostServerImageName            = "gcr.io/kfserving/xgbserver"
	DefaultXGBoostRuntimeVersion      = "latest"
)

func (x *XGBoostSpec) GetStorageUri() string {
	return x.StorageURI
}

func (x *XGBoostSpec) CreateModelServingContainer(modelName string, config *PredictorsConfig) *v1.Container {
	imageName := XGBoostServerImageName
	if config.Xgboost.ContainerImage != "" {
		imageName = config.Xgboost.ContainerImage
	}
	return &v1.Container{
		Image:     imageName + ":" + x.RuntimeVersion,
		Resources: x.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_dir=" + constants.DefaultModelLocalMountPath,
		},
	}
}

func (x *XGBoostSpec) ApplyDefaults() {
	if x.RuntimeVersion == "" {
		x.RuntimeVersion = DefaultXGBoostRuntimeVersion
	}

	setResourceRequirementDefaults(&x.Resources)
}

func (x *XGBoostSpec) Validate() error {
	if utils.Includes(AllowedXGBoostRuntimeVersions, x.RuntimeVersion) {
		return nil
	}
	return fmt.Errorf(InvalidXGBoostRuntimeVersionError)
}
