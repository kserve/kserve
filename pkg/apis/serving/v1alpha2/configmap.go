/*

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

package v1alpha2

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMap Keys
const (
	PredictorConfigKeyName   = "predictors"
	TransformerConfigKeyName = "transformers"
	ExplainerConfigKeyName   = "explainers"
)

// +kubebuilder:object:generate=false
type ExplainerConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion string `json:"defaultImageVersion"`
}

// +kubebuilder:object:generate=false
type ExplainersConfig struct {
	AlibiExplainer ExplainerConfig `json:"alibi,omitempty"`
	AIXExplainer   ExplainerConfig `json:"aix,omitempty"`
}

// +kubebuilder:object:generate=false
type PredictorProtocols struct {
	V1 *PredictorConfig `json:"v1,omitempty"`
	V2 *PredictorConfig `json:"v2,omitempty"`
}

// +kubebuilder:object:generate=false
type PredictorConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion    string `json:"defaultImageVersion"`
	DefaultGpuImageVersion string `json:"defaultGpuImageVersion"`
	// Default timeout of predictor for serving a request, in seconds
	DefaultTimeout int64 `json:"defaultTimeout,string,omitempty"`
}

// +kubebuilder:object:generate=false
type PredictorsConfig struct {
	Tensorflow PredictorConfig    `json:"tensorflow,omitempty"`
	Triton     PredictorConfig    `json:"triton,omitempty"`
	Xgboost    PredictorProtocols `json:"xgboost,omitempty"`
	LightGBM   PredictorConfig    `json:"lightgbm,omitempty"`
	SKlearn    PredictorProtocols `json:"sklearn,omitempty"`
	PyTorch    PredictorConfig    `json:"pytorch,omitempty"`
	ONNX       PredictorConfig    `json:"onnx,omitempty"`
	PMML       PredictorConfig    `json:"pmml,omitempty"`
}

// +kubebuilder:object:generate=false
type TransformerConfig struct {
	ContainerImage string `json:"image"`

	DefaultImageVersion string `json:"defaultImageVersion"`
}

// +kubebuilder:object:generate=false
type TransformersConfig struct {
	Feast TransformerConfig `json:"feast,omitempty"`
}

// +kubebuilder:object:generate=false
type InferenceServicesConfig struct {
	Transformers *TransformersConfig `json:"transformers"`
	Predictors   *PredictorsConfig   `json:"predictors"`
	Explainers   *ExplainersConfig   `json:"explainers"`
}

func GetInferenceServicesConfig(client client.Client) (*InferenceServicesConfig, error) {
	configMap := &v1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
	if err != nil {
		return nil, err
	}

	inferenceServiceConfigMap, err := NewInferenceServicesConfig(configMap)
	if err != nil {
		return nil, err
	}

	return inferenceServiceConfigMap, nil
}

func NewInferenceServicesConfig(configMap *v1.ConfigMap) (*InferenceServicesConfig, error) {
	predictorsConfig, err := getPredictorsConfigs(configMap)
	if err != nil {
		return nil, err
	}
	transformersConfig, err := getTransformersConfigs(configMap)
	if err != nil {
		return nil, err
	}
	explainersConfig, err := getExplainersConfigs(configMap)
	if err != nil {
		return nil, err
	}
	return &InferenceServicesConfig{
		Predictors:   predictorsConfig,
		Transformers: transformersConfig,
		Explainers:   explainersConfig,
	}, nil
}

func getPredictorsConfigs(configMap *v1.ConfigMap) (*PredictorsConfig, error) {
	predictorConfig := &PredictorsConfig{}
	if data, ok := configMap.Data[PredictorConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &predictorConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall %v json string due to %v ", PredictorConfigKeyName, err)
		}
	}
	return predictorConfig, nil
}

func getTransformersConfigs(configMap *v1.ConfigMap) (*TransformersConfig, error) {
	transformerConfig := &TransformersConfig{}
	if data, ok := configMap.Data[TransformerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &transformerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall %v json string due to %v ", TransformerConfigKeyName, err)
		}
	}
	return transformerConfig, nil
}

func getExplainersConfigs(configMap *v1.ConfigMap) (*ExplainersConfig, error) {
	explainerConfig := &ExplainersConfig{}
	if data, ok := configMap.Data[ExplainerConfigKeyName]; ok {
		err := json.Unmarshal([]byte(data), &explainerConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to unmarshall %v json string due to %v ", ExplainerConfigKeyName, err)
		}
	}
	return explainerConfig, nil
}
