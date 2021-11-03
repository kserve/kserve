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

var (
	_ ComponentImplementation = &ModelSpec{}
)

// Here, the ComponentImplementation interface is implemented in order to maintain the
// component validation logic. This will probably be refactored out eventually.

func (m *ModelSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(m.GetStorageUri()),
	})
}

func (m *ModelSpec) Default(config *InferenceServicesConfig) {}

func (m *ModelSpec) GetStorageUri() *string {
	return m.StorageURI
}

func (m *ModelSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container {
	return &m.Container
}

func (m *ModelSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV2
}

func (m *ModelSpec) IsMMS(config *InferenceServicesConfig) bool {
	return false
}
