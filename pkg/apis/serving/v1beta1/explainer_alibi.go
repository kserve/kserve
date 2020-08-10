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
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
	"strconv"
)

// AlibiExplainerType is the explanation method
type AlibiExplainerType string

// AlibiExplainerType Enum
const (
	AlibiAnchorsTabularExplainer  AlibiExplainerType = "AnchorTabular"
	AlibiAnchorsImageExplainer    AlibiExplainerType = "AnchorImages"
	AlibiAnchorsTextExplainer     AlibiExplainerType = "AnchorText"
	AlibiCounterfactualsExplainer AlibiExplainerType = "Counterfactuals"
	AlibiContrastiveExplainer     AlibiExplainerType = "Contrastive"
)

// AlibiExplainerSpec defines the arguments for configuring an Alibi Explanation Server
type AlibiExplainerSpec struct {
	// The type of Alibi explainer
	// Valid values are:
	// - "AnchorTabular";
	// - "AnchorImages";
	// - "AnchorText";
	// - "Counterfactuals";
	// - "Contrastive";
	Type AlibiExplainerType `json:"type"`
	// The location of a trained explanation model
	StorageURI string `json:"storageUri,omitempty"`
	// Alibi docker image version, defaults to latest Alibi Version
	RuntimeVersion *string `json:"runtimeVersion,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
	// Container enables overrides for the predictor.
	// Each framework will have different defaults that are populated in the underlying container spec.
	// +optional
	v1.Container `json:",inline"`
}

var _ Component = &AlibiExplainerSpec{}

func (alibi *AlibiExplainerSpec) GetStorageUri() *string {
	return &alibi.StorageURI
}

func (alibi *AlibiExplainerSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &alibi.Resources
}

func (alibi *AlibiExplainerSpec) GetContainer(metadata metav1.ObjectMeta, containerConcurrency *int64, config *InferenceServicesConfig) *v1.Container {
	var args = []string{
		constants.ArgumentModelName, metadata.Name,
		constants.ArgumentPredictorHost, constants.PredictorURL(metadata, false),
		constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort,
	}
	if containerConcurrency != nil {
		args = append(args, constants.ArgumentWorkers, strconv.FormatInt(*containerConcurrency, 10))
	}
	if alibi.StorageURI != "" {
		args = append(args, "--storage_uri", constants.DefaultModelLocalMountPath)
	}

	args = append(args, string(alibi.Type))

	// Order explainer config map keys
	var keys []string
	for k, _ := range alibi.Config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--"+k)
		args = append(args, alibi.Config[k])
	}
	if alibi.Container.Image == "" {
		alibi.Image = config.Explainers.AlibiExplainer.ContainerImage + ":" + *alibi.RuntimeVersion
	}
	alibi.Name = constants.InferenceServiceContainerName
	alibi.Args = args
	return &alibi.Container
}

func (alibi *AlibiExplainerSpec) Default(config *InferenceServicesConfig) {
	alibi.Name = constants.InferenceServiceContainerName
	if alibi.RuntimeVersion == nil {
		alibi.RuntimeVersion = proto.String(config.Explainers.AlibiExplainer.DefaultImageVersion)
	}
	setResourceRequirementDefaults(&alibi.Resources)
}

func (alibi *AlibiExplainerSpec) Validate() error {
	return nil
}
