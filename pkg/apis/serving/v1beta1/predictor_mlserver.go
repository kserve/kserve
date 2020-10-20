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
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MLServerSpec defines the API for configuring MLServer predictors
type MLServerSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

// Validate returns an error if the spec is invalid
func (m *MLServerSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(m.GetStorageUri()),
	})
}

// Default sets some of the spec fields to default values if undefined
func (m *MLServerSpec) Default(config *InferenceServicesConfig) {

}

// GetContainers transforms the resource into a container spec
func (m *MLServerSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	return nil
}

func (m *MLServerSpec) GetStorageUri() *string {
	return m.StorageURI
}
