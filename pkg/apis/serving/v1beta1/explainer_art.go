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
	"fmt"
	"sort"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ARTExplainerType string

const (
	ARTSquareAttackExplainer ARTExplainerType = "SquareAttack"
)

// ARTExplainerType defines the arguments for configuring an ART Explanation Server
type ARTExplainerSpec struct {
	// The type of ART explainer
	Type ARTExplainerType `json:"type"`
	// Contains fields shared across all explainers
	ExplainerExtensionSpec `json:",inline"`
}

var _ ComponentImplementation = &ARTExplainerSpec{}

func (s *ARTExplainerSpec) GetStorageUri() *string {
	if s.StorageURI == "" {
		return nil
	}
	return &s.StorageURI
}

func (s *ARTExplainerSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &s.Resources
}

func (s *ARTExplainerSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig,
	predictorHost ...string) *v1.Container {
	var args = []string{
		constants.ArgumentModelName, metadata.Name,
		constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort,
	}
	if !utils.IncludesArg(s.Container.Args, constants.ArgumentPredictorHost) {
		args = append(args, constants.ArgumentPredictorHost,
			fmt.Sprintf("%s.%s", predictorHost[0], metadata.Namespace))

	}
	if !utils.IncludesArg(s.Container.Args, constants.ArgumentWorkers) {
		if extensions.ContainerConcurrency != nil {
			args = append(args, constants.ArgumentWorkers, strconv.FormatInt(*extensions.ContainerConcurrency, 10))
		}
	}
	if s.StorageURI != "" {
		args = append(args, "--storage_uri", constants.DefaultModelLocalMountPath)
	}

	args = append(args, "--adversary_type", string(s.Type))

	// Order explainer config map keys
	var keys []string
	for k, _ := range s.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--"+k)
		args = append(args, s.Config[k])
	}
	args = append(args, s.Args...)
	return &v1.Container{
		Image:     config.Explainers.ARTExplainer.ContainerImage + ":" + *s.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: s.Resources,
		Args:      args,
	}
}

func (s *ARTExplainerSpec) Default(config *InferenceServicesConfig) {
	s.Name = constants.InferenceServiceContainerName
	if s.RuntimeVersion == nil {
		s.RuntimeVersion = proto.String(config.Explainers.ARTExplainer.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&s.Resources)
}

// Validate the spec
func (s *ARTExplainerSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateStorageURI(s.GetStorageUri()),
	})
}

func (s *ARTExplainerSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (s *ARTExplainerSpec) IsMMS(config *InferenceServicesConfig) bool {
	return false
}
