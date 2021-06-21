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
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
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
	_ PredictorImplementation = &PMMLSpec{}
)

// Validate returns an error if invalid
func (p *PMMLSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		ValidateMaxArgumentWorkers(p.Container.Args, 1),
		validateStorageURI(p.GetStorageUri()),
	})
}

// Default sets defaults on the resource
func (p *PMMLSpec) Default(config *InferenceServicesConfig) {
	p.Container.Name = constants.InferenceServiceContainerName
	if p.RuntimeVersion == nil {
		p.RuntimeVersion = proto.String(config.Predictors.PMML.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&p.Resources)
}

// GetContainer transforms the resource into a container spec
func (p *PMMLSpec) GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig,
	predictorHost ...string) *v1.Container {
	arguments := []string{
		fmt.Sprintf("%s=%s", constants.ArgumentModelName, metadata.Name),
		fmt.Sprintf("%s=%s", constants.ArgumentModelDir, constants.DefaultModelLocalMountPath),
		fmt.Sprintf("%s=%s", constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort),
	}

	if p.Container.Image == "" {
		p.Container.Image = config.Predictors.PMML.ContainerImage + ":" + *p.RuntimeVersion
	}
	p.Container.Name = constants.InferenceServiceContainerName
	p.Container.Args = append(arguments, p.Container.Args...)
	return &p.Container
}

func (p *PMMLSpec) GetStorageUri() *string {
	return p.StorageURI
}

func (p *PMMLSpec) GetProtocol() constants.InferenceServiceProtocol {
	return constants.ProtocolV1
}

func (p *PMMLSpec) IsMMS(config *InferenceServicesConfig) bool {
	return config.Predictors.PMML.MultiModelServer
}

func (p *PMMLSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	supportedFrameworks := config.Predictors.PMML.SupportedFrameworks
	return isFrameworkIncluded(supportedFrameworks, framework)
}
