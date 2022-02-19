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
	"context"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelFormat struct {
	// Name of the model format.
	// +required
	Name string `json:"name"`
	// Version of the model format.
	// Used in validating that a predictor is supported by a runtime.
	// Can be "major", "major.minor" or "major.minor.patch".
	// +optional
	Version *string `json:"version,omitempty"`
}

type ModelSpec struct {
	// ModelFormat being served.
	// +required
	ModelFormat ModelFormat `json:"modelFormat"`

	// Specific ClusterServingRuntime/ServingRuntime name to use for deployment.
	// +optional
	Runtime *string `json:"runtime,omitempty"`

	PredictorExtensionSpec `json:",inline"`
}

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
	if m.ProtocolVersion != nil {
		return *m.ProtocolVersion
	}
	return constants.ProtocolV2
}

func (m *ModelSpec) IsMMS(config *InferenceServicesConfig) bool {
	predictorConfig := m.getPredictorConfig(config)
	if predictorConfig != nil {
		return predictorConfig.MultiModelServer
	}
	return false
}

func (m *ModelSpec) IsFrameworkSupported(framework string, config *InferenceServicesConfig) bool {
	predictorConfig := m.getPredictorConfig(config)
	if predictorConfig != nil {
		supportedFrameworks := predictorConfig.SupportedFrameworks
		return isFrameworkIncluded(supportedFrameworks, framework)
	}
	return false
}

type stringSet map[string]struct{}

func (ss stringSet) add(s string) {
	ss[s] = struct{}{}
}

func (ss stringSet) contains(s string) bool {
	_, found := ss[s]
	return found
}

// GetSupportingRuntimes Get a list of ServingRuntimeSpecs that correspond to ServingRuntimes and ClusterServingRuntimes that
// support the given model. If the `isMMS` argument is true, this function will only return ServingRuntimes that are
// ModelMesh compatible, otherwise only single-model serving compatible runtimes will be returned.
func (m *ModelSpec) GetSupportingRuntimes(cl client.Client, namespace string, isMMS bool) ([]v1alpha1.ServingRuntimeSpec, error) {

	// List all namespace-scoped runtimes.
	runtimes := &v1alpha1.ServingRuntimeList{}
	if err := cl.List(context.TODO(), runtimes, client.InNamespace(namespace)); err != nil {
		return nil, err
	}

	// List all cluster-scoped runtimes.
	clusterRuntimes := &v1alpha1.ClusterServingRuntimeList{}
	if err := cl.List(context.TODO(), clusterRuntimes); err != nil {
		return nil, err
	}

	srSpecs := make([]v1alpha1.ServingRuntimeSpec, 0, len(runtimes.Items)+len(clusterRuntimes.Items))
	for i := range runtimes.Items {
		rt := &runtimes.Items[i]
		if !rt.Spec.IsDisabled() && rt.Spec.IsMultiModelRuntime() == isMMS && m.RuntimeSupportsModel(&rt.Spec) {
			srSpecs = append(srSpecs, rt.Spec)
		}
	}

	for i := range clusterRuntimes.Items {
		crt := &clusterRuntimes.Items[i]
		if !crt.Spec.IsDisabled() && crt.Spec.IsMultiModelRuntime() == isMMS && m.RuntimeSupportsModel(&crt.Spec) {
			srSpecs = append(srSpecs, crt.Spec)
		}
	}
	return srSpecs, nil
}

// RuntimeSupportsModel Check if the given runtime supports the specified model.
func (m *ModelSpec) RuntimeSupportsModel(srSpec *v1alpha1.ServingRuntimeSpec) bool {
	// assignment to a runtime depends on the model format labels
	runtimeLabelSet := m.getServingRuntimeSupportedModelFormatLabelSet(srSpec.SupportedModelFormats)
	modelLabel := m.getModelFormatLabel()
	// if the runtime has the model's label, then it supports that model.
	return runtimeLabelSet.contains(modelLabel)
}

func (m *ModelSpec) getModelFormatLabel() string {
	mt := m.ModelFormat
	if mt.Version != nil {
		return "mt:" + mt.Name + ":" + *mt.Version
	}
	return "mt:" + mt.Name
}

func (m *ModelSpec) getServingRuntimeSupportedModelFormatLabelSet(supportedModelFormats []v1alpha1.SupportedModelFormat) stringSet {
	set := make(stringSet, 2*len(supportedModelFormats)+1)

	// model format labels
	for _, t := range supportedModelFormats {
		// If runtime isn't explicitly set, only add labels for modelFormats where AutoSelect is true.
		if m.Runtime != nil || (t.AutoSelect != nil && *t.AutoSelect) {
			set.add("mt:" + t.Name)
			if t.Version != nil {
				set.add("mt:" + t.Name + ":" + *t.Version)
			}
		}
	}
	return set
}

func (m *ModelSpec) getPredictorConfig(config *InferenceServicesConfig) *PredictorConfig {
	switch {
	case m.ModelFormat.Name == constants.SupportedModelSKLearn:
		if m.ProtocolVersion != nil &&
			constants.ProtocolV2 == *m.ProtocolVersion {
			return config.Predictors.SKlearn.V2
		} else {
			return config.Predictors.SKlearn.V1
		}

	case m.ModelFormat.Name == constants.SupportedModelXGBoost:
		if m.ProtocolVersion != nil &&
			constants.ProtocolV2 == *m.ProtocolVersion {
			return config.Predictors.XGBoost.V2
		} else {
			return config.Predictors.XGBoost.V1
		}
	case m.ModelFormat.Name == constants.SupportedModelTensorflow:
		return &config.Predictors.Tensorflow
	case m.ModelFormat.Name == constants.SupportedModelPyTorch:
		return &config.Predictors.PyTorch
	case m.ModelFormat.Name == constants.SupportedModelONNX:
		return &config.Predictors.ONNX
	case m.ModelFormat.Name == constants.SupportedModelPMML:
		return &config.Predictors.PMML
	case m.ModelFormat.Name == constants.SupportedModelLightGBM:
		return &config.Predictors.LightGBM
	case m.ModelFormat.Name == constants.SupportedModelPaddle:
		return &config.Predictors.Paddle
	case m.ModelFormat.Name == constants.SupportedModelTriton:
		return &config.Predictors.Triton
	}
	return nil
}
