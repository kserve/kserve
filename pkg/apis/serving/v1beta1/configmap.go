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
	"text/template"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/constants"
)

// ConfigMap Keys
const (
	ExplainerConfigKeyName = "explainers"
)

const (
	IngressConfigKeyName = "ingress"
	DeployConfigName     = "deploy"

	DefaultDomainTemplate = "{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}"
	DefaultIngressDomain  = "example.com"

	DefaultUrlScheme = "http"
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
	ARTExplainer ExplainerConfig `json:"art,omitempty"`
}

// +kubebuilder:object:generate=false
type InferenceServicesConfig struct {
	// Explainer configurations
	Explainers ExplainersConfig `json:"explainers"`
}

// +kubebuilder:object:generate=false
type IngressConfig struct {
	IngressGateway           string    `json:"ingressGateway,omitempty"`
	IngressServiceName       string    `json:"ingressService,omitempty"`
	LocalGateway             string    `json:"localGateway,omitempty"`
	LocalGatewayServiceName  string    `json:"localGatewayService,omitempty"`
	IngressDomain            string    `json:"ingressDomain,omitempty"`
	IngressClassName         *string   `json:"ingressClassName,omitempty"`
	AdditionalIngressDomains *[]string `json:"additionalIngressDomains,omitempty"`
	DomainTemplate           string    `json:"domainTemplate,omitempty"`
	UrlScheme                string    `json:"urlScheme,omitempty"`
	DisableIstioVirtualHost  bool      `json:"disableIstioVirtualHost,omitempty"`
	PathTemplate             string    `json:"pathTemplate,omitempty"`
	DisableIngressCreation   bool      `json:"disableIngressCreation,omitempty"`
}

// +kubebuilder:object:generate=false
type DeployConfig struct {
	DefaultDeploymentMode string `json:"defaultDeploymentMode,omitempty"`
}

func NewInferenceServicesConfig(clientset kubernetes.Interface) (*InferenceServicesConfig, error) {
	configMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	icfg := &InferenceServicesConfig{}
	for _, err := range []error{
		getComponentConfig(ExplainerConfigKeyName, configMap, &icfg.Explainers),
	} {
		if err != nil {
			return nil, err
		}
	}
	return icfg, nil
}

func NewIngressConfig(clientset kubernetes.Interface) (*IngressConfig, error) {
	configMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	ingressConfig := &IngressConfig{}
	if ingress, ok := configMap.Data[IngressConfigKeyName]; ok {
		err := json.Unmarshal([]byte(ingress), &ingressConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse ingress config json: %w", err)
		}

		if ingressConfig.IngressGateway == "" || ingressConfig.IngressServiceName == "" {
			return nil, fmt.Errorf("invalid ingress config - ingressGateway and ingressService are required")
		}
		if ingressConfig.PathTemplate != "" {
			// TODO: ensure that the generated path is valid, that is:
			// * both Name and Namespace are used to avoid collisions
			// * starts with a /
			// For now simply check that this is a valid template.
			_, err := template.New("path-template").Parse(ingressConfig.PathTemplate)
			if err != nil {
				return nil, fmt.Errorf("invalid ingress config, unable to parse pathTemplate: %w", err)
			}
			if ingressConfig.IngressDomain == "" {
				return nil, fmt.Errorf("invalid ingress config - ingressDomain is required if pathTemplate is given")
			}
		}
	}

	if ingressConfig.DomainTemplate == "" {
		ingressConfig.DomainTemplate = DefaultDomainTemplate
	}

	if ingressConfig.IngressDomain == "" {
		ingressConfig.IngressDomain = DefaultIngressDomain
	}

	if ingressConfig.UrlScheme == "" {
		ingressConfig.UrlScheme = DefaultUrlScheme
	}

	return ingressConfig, nil
}

func getComponentConfig(key string, configMap *v1.ConfigMap, componentConfig interface{}) error {
	if data, ok := configMap.Data[key]; ok {
		err := json.Unmarshal([]byte(data), componentConfig)
		if err != nil {
			return fmt.Errorf("unable to unmarshall %v json string due to %w ", key, err)
		}
	}
	return nil
}

func NewDeployConfig(clientset kubernetes.Interface) (*DeployConfig, error) {
	configMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	deployConfig := &DeployConfig{}
	if deploy, ok := configMap.Data[DeployConfigName]; ok {
		err := json.Unmarshal([]byte(deploy), &deployConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse deploy config json: %w", err)
		}

		if deployConfig.DefaultDeploymentMode == "" {
			return nil, fmt.Errorf("invalid deploy config, defaultDeploymentMode is required")
		}

		if deployConfig.DefaultDeploymentMode != string(constants.Serverless) &&
			deployConfig.DefaultDeploymentMode != string(constants.RawDeployment) &&
			deployConfig.DefaultDeploymentMode != string(constants.ModelMeshDeployment) {
			return nil, fmt.Errorf("invalid deployment mode. Supported modes are Serverless," +
				" RawDeployment and ModelMesh")
		}
	}
	return deployConfig, nil
}
