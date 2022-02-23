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

const (
	IngressConfigKeyName = "ingress"
	DeployConfigName     = "deploy"

	DefaultDomainTemplate = "{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}"
	DefaultIngressDomain  = "example.com"
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
	ARTExplainer   ExplainerConfig `json:"art,omitempty"`
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
	// Flag to determine if multi-model serving is supported
	MultiModelServer bool `json:"multiModelServer,boolean,omitempty"`
	// frameworks the model agent is able to run
	SupportedFrameworks []string `json:"supportedFrameworks"`
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
	PyTorch    PredictorConfig    `json:"pytorch,omitempty"`
	ONNX       PredictorConfig    `json:"onnx,omitempty"`
	PMML       PredictorConfig    `json:"pmml,omitempty"`
	LightGBM   PredictorConfig    `json:"lightgbm,omitempty"`
	Paddle     PredictorConfig    `json:"paddle,omitempty"`
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
	IngressGateway          string  `json:"ingressGateway,omitempty"`
	IngressServiceName      string  `json:"ingressService,omitempty"`
	LocalGateway            string  `json:"localGateway,omitempty"`
	LocalGatewayServiceName string  `json:"localGatewayService,omitempty"`
	IngressDomain           string  `json:"ingressDomain,omitempty"`
	IngressClassName        *string `json:"ingressClassName,omitempty"`
	DomainTemplate          string  `json:"domainTemplate,omitempty"`
}

// +kubebuilder:object:generate=false
type DeployConfig struct {
	DefaultDeploymentMode string `json:"defaultDeploymentMode,omitempty"`
}

func NewInferenceServicesConfig(cli client.Client) (*InferenceServicesConfig, error) {
	configMap := &v1.ConfigMap{}
	err := cli.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
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
	err := cli.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
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
			return nil, fmt.Errorf("Invalid ingress config, ingressGateway, ingressService are required.")
		}
	}

	if ingressConfig.DomainTemplate == "" {
		ingressConfig.DomainTemplate = DefaultDomainTemplate
	}

	if ingressConfig.IngressDomain == "" {
		ingressConfig.IngressDomain = DefaultIngressDomain
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

func NewDeployConfig(cli client.Client) (*DeployConfig, error) {
	configMap := &v1.ConfigMap{}
	err := cli.Get(context.TODO(), types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
	if err != nil {
		return nil, err
	}
	deployConfig := &DeployConfig{}
	if deploy, ok := configMap.Data[DeployConfigName]; ok {
		err := json.Unmarshal([]byte(deploy), &deployConfig)
		if err != nil {
			return nil, fmt.Errorf("Unable to parse deploy config json: %v", err)
		}

		if deployConfig.DefaultDeploymentMode == "" {
			return nil, fmt.Errorf("Invalid deploy config, defaultDeploymentMode is required.")
		}

		if deployConfig.DefaultDeploymentMode != string(constants.Serverless) &&
			deployConfig.DefaultDeploymentMode != string(constants.RawDeployment) &&
			deployConfig.DefaultDeploymentMode != string(constants.ModelMeshDeployment) {
			return nil, fmt.Errorf("Invalid deployment mode. Supported modes are Serverless," +
				" RawDeployment and ModelMesh")
		}
	}
	return deployConfig, nil
}
