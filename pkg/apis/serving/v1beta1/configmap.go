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
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubeflow/kfserving/pkg/constants"
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

const (
	IngressConfigKeyName = "ingress"
)

// +kubebuilder:object:generate=false
type ExplainerConfig struct {
	// explainer docker image name
	ContainerImage string `json:"image"`
	// default explainer docker image version
	DefaultImageVersion string `json:"defaultImageVersion"`
}

// +kubebuilder:object:generate=false
type ExplainersConfig struct {
	AlibiExplainer ExplainerConfig `json:"alibi,omitempty"`
	AIXExplainer   ExplainerConfig `json:"aix,omitempty"`
}

// +kubebuilder:object:generate=false
type PredictorConfig struct {
	// predictor docker image name
	ContainerImage string `json:"image"`
	// default predictor docker image version on cpu
	DefaultImageVersion string `json:"defaultImageVersion"`
	// default predictor docker image version on gpu
	DefaultGpuImageVersion string `json:"defaultGpuImageVersion"`
	// Default timeout of predictor for serving a request, in seconds
	DefaultTimeout int64 `json:"defaultTimeout,string,omitempty"`
}

// +kubebuilder:object:generate=false
type PredictorProtocols struct {
	V1 *PredictorConfig `json:"v1,omitempty"`
	V2 *PredictorConfig `json:"v2,omitempty"`
}

// +kubebuilder:object:generate=false
type PredictorsConfig struct {
	Tensorflow PredictorConfig    `json:"tensorflow,omitempty"`
	Triton     PredictorConfig    `json:"triton,omitempty"`
	XGBoost    PredictorProtocols `json:"xgboost,omitempty"`
	SKlearn    PredictorProtocols `json:"sklearn,omitempty"`
	PyTorch    PredictorProtocols `json:"pytorch,omitempty"`
	ONNX       PredictorConfig    `json:"onnx,omitempty"`
	PMML       PredictorConfig    `json:"pmml,omitempty"`
	LightGBM   PredictorConfig    `json:"lightgbm,omitempty"`
}

// +kubebuilder:object:generate=false
type TransformerConfig struct {
	// transformer docker image name
	ContainerImage string `json:"image"`
	// default transformer docker image version
	DefaultImageVersion string `json:"defaultImageVersion"`
}

// +kubebuilder:object:generate=false
type TransformersConfig struct {
	Feast TransformerConfig `json:"feast,omitempty"`
}

// +kubebuilder:object:generate=false
type InferenceServicesConfig struct {
	// Transformer configurations
	Transformers TransformersConfig `json:"transformers"`
	// Predictor configurations
	Predictors PredictorsConfig `json:"predictors"`
	// Explainer configurations
	Explainers ExplainersConfig `json:"explainers"`
}

// +kubebuilder:object:generate=false
type IngressConfig struct {
	IngressGateway     string `json:"ingressGateway,omitempty"`
	IngressServiceName string `json:"ingressService,omitempty"`
}

func NewInferenceServicesConfig(cli client.Client) (*InferenceServicesConfig, error) {
	configMap := &v1.ConfigMap{}
	err := cli.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		return nil, err
	}
	icfg := &InferenceServicesConfig{}
	for _, err := range []error{
		getComponentConfig(PredictorConfigKeyName, configMap, &icfg.Predictors),
		getComponentConfig(ExplainerConfigKeyName, configMap, &icfg.Explainers),
		getComponentConfig(TransformerConfigKeyName, configMap, &icfg.Transformers),
	} {
		if err != nil {
			return nil, err
		}
	}
	return icfg, nil
}

func NewIngressConfig(cli client.Client) (*IngressConfig, error) {
	configMap := &v1.ConfigMap{}
	err := cli.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KFServingNamespace}, configMap)
	if err != nil {
		return nil, err
	}
	ingressConfig := &IngressConfig{}
	if ingress, ok := configMap.Data[IngressConfigKeyName]; ok {
		err := json.Unmarshal([]byte(ingress), &ingressConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to parse ingress config json: %v", err)
		}

		if ingressConfig.IngressGateway == "" || ingressConfig.IngressServiceName == "" {
			return nil, fmt.Errorf("Invalid ingress config, ingressGateway and ingressService are required.")
		}
	}
	return ingressConfig, nil
}

func getComponentConfig(key string, configMap *v1.ConfigMap, componentConfig interface{}) error {
	if data, ok := configMap.Data[key]; ok {
		err := json.Unmarshal([]byte(data), componentConfig)
		if err != nil {
			return fmt.Errorf("Unable to unmarshall %v json string due to %v ", key, err)
		}
	}
	return nil
}
