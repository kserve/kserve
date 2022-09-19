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
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PMMLSpec defines arguments for configuring PMML model serving.
type PMMLSpec struct {
	// Contains fields shared across all predictors
	PredictorExtensionSpec `json:",inline"`
}

var (
	_ ComponentImplementation = &PMMLSpec{}
)

// Validate returns an error if invalid
func (p *PMMLSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		ValidateMaxArgumentWorkers(p.Container.Args, 1),
		validateStorageURI(p.GetStorageUri()),
		validateStorageSpec(p.GetStorageSpec(), p.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (p *PMMLSpec) Default(config *InferenceServicesConfig) {
	p.Container.Name = constants.InferenceServiceContainerName
	setResourceRequirementDefaults(&p.Resources)
}

func (p *PMMLSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	return &p.Container
}

func (p *PMMLSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}
