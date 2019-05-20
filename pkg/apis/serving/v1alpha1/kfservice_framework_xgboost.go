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
	XGBoostEntrypointCommand = "python"
	XGBoostServingGRPCPort   = "9000"
	XGBoostServingRestPort   = "8080"
	XGBoostServingImageName  = "animeshsingh/xgboostserver"

	DefaultXGBoostServingVersion = "latest"
)

func (x *XGBoostSpec) CreateModelServingContainer(modelName string) *v1.Container {
	//TODO(@animeshsingh) add configmap for image, default resources, readiness/liveness probe
	return &v1.Container{
		Image:     XGBoostServingImageName + ":" + x.RuntimeVersion,
		Command:   []string{XGBoostEntrypointCommand},
		Resources: x.Resources,
		Args: []string{
			// TODO: Allow setting rest and grpc ports @animeshsingh
			// "--port=" + XGBoostServingGRPCPort,
			// "--rest_api_port=" + XGBoostServingRestPort,
			"-m",
			"xgbserver",
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
