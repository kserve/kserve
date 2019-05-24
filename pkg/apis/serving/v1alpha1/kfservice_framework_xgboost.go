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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
)

const (
	XGBoostServingGRPCPort  = "9000"
	XGBoostServingRestPort  = "8080"
	XGBoostServingImageName = "gcr.io/kfserving/xgbserver"

	DefaultXGBoostServingVersion = "latest"
)

var _ FrameworkHandler = (*XGBoostSpec)(nil)

func (x *XGBoostSpec) CreateModelServingContainer(modelName string, configs map[string]string) *v1.Container {
	xgboostServingImage := XGBoostServingImageName
	if image, ok := configs[XGBoostServingImageConfigName]; ok {
		xgboostServingImage = image
	}
	return &v1.Container{
		Image:     xgboostServingImage + ":" + x.RuntimeVersion,
		Resources: x.Resources,
		Args: []string{
			"--model_name=" + modelName,
			"--model_dir=" + x.ModelURI,
		},
	}
}

func (x *XGBoostSpec) ApplyDefaults() {
	if x.RuntimeVersion == "" {
		x.RuntimeVersion = DefaultXGBoostServingVersion
	}

	setResourceRequirementDefaults(&x.Resources)
}

func (x *XGBoostSpec) Validate() error {
	return nil
}
