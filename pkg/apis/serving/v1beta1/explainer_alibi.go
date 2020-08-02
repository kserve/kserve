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
	Type AlibiExplainerType `json:"type"`
	// The location of a trained explanation model
	StorageURI string `json:"storageUri,omitempty"`
	// Alibi docker image version, defaults to latest Alibi Version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// Alibi explainer container resources, defaults to requests and limits of 1CPU, 2Gi MEM.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Inline custom parameter settings for explainer
	Config map[string]string `json:"config,omitempty"`
}

func (alibi *AlibiExplainerSpec) GetStorageUri() *string {
	return &alibi.StorageURI
}

func (alibi *AlibiExplainerSpec) GetResourceRequirements() *v1.ResourceRequirements {
	// return the ResourceRequirements value if set on the spec
	return &alibi.Resources
}

func (alibi *AlibiExplainerSpec) GetContainer(metadata metav1.ObjectMeta, parallelism int, config *InferenceServicesConfig) *v1.Container {
	var args = []string{
		constants.ArgumentModelName, metadata.Name,
		constants.ArgumentPredictorHost, constants.PredictorURL(metadata, false),
		constants.ArgumentHttpPort, constants.InferenceServiceDefaultHttpPort,
	}
	if parallelism != 0 {
		args = append(args, constants.ArgumentWorkers, strconv.Itoa(parallelism))
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

	return &v1.Container{
		Image:     config.Explainers.AlibiExplainer.ContainerImage + ":" + alibi.RuntimeVersion,
		Name:      constants.InferenceServiceContainerName,
		Resources: alibi.Resources,
		Args:      args,
	}
}

func (alibi *AlibiExplainerSpec) Default(config *InferenceServicesConfig) {
	if alibi.RuntimeVersion == "" {
		alibi.RuntimeVersion = config.Explainers.AlibiExplainer.DefaultImageVersion
	}
	setResourceRequirementDefaults(&alibi.Resources)
}

func (alibi *AlibiExplainerSpec) Validate() error {
	return nil
}
