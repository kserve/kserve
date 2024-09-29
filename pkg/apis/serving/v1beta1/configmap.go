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
	"strings"
	"text/template"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/constants"
)

// ConfigMap Keys
const (
	ExplainerConfigKeyName = "explainers"
	IngressConfigKeyName   = "ingress"
	DeployConfigName       = "deploy"
	LocalModelConfigName   = "localModel"
	SecurityConfigName     = "security"
	ServiceConfigName      = "service"
)

const (
	DefaultDomainTemplate = "{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}"
	DefaultIngressDomain  = "example.com"
	DefaultUrlScheme      = "http"
)

// Error messages
const (
	ErrKserveIngressGatewayRequired         = "invalid ingress config - kserveIngressGateway is required"
	ErrInvalidKserveIngressGatewayFormat    = "invalid ingress config - kserveIngressGateway should be in the format <namespace>/<name>"
	ErrInvalidKserveIngressGatewayName      = "invalid ingress config - kserveIngressGateway gateway name is invalid"
	ErrInvalidKserveIngressGatewayNamespace = "invalid ingress config - kserveIngressGateway gateway namespace is invalid"
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
	KserveIngressGateway       string    `json:"kserveIngressGateway,omitempty"`
	IngressGateway             string    `json:"ingressGateway,omitempty"`
	KnativeLocalGatewayService string    `json:"knativeLocalGatewayService,omitempty"`
	LocalGateway               string    `json:"localGateway,omitempty"`
	LocalGatewayServiceName    string    `json:"localGatewayService,omitempty"`
	IngressDomain              string    `json:"ingressDomain,omitempty"`
	IngressClassName           *string   `json:"ingressClassName,omitempty"`
	AdditionalIngressDomains   *[]string `json:"additionalIngressDomains,omitempty"`
	DomainTemplate             string    `json:"domainTemplate,omitempty"`
	UrlScheme                  string    `json:"urlScheme,omitempty"`
	DisableIstioVirtualHost    bool      `json:"disableIstioVirtualHost,omitempty"`
	PathTemplate               string    `json:"pathTemplate,omitempty"`
	DisableIngressCreation     bool      `json:"disableIngressCreation,omitempty"`
}

// +kubebuilder:object:generate=false
type DeployConfig struct {
	DefaultDeploymentMode string `json:"defaultDeploymentMode,omitempty"`
}

// +kubebuilder:object:generate=false
type LocalModelConfig struct {
	Enabled                      bool   `json:"enabled"`
	JobNamespace                 string `json:"jobNamespace"`
	DefaultJobImage              string `json:"defaultJobImage,omitempty"`
	FSGroup                      *int64 `json:"fsGroup,omitempty"`
	JobTTLSecondsAfterFinished   *int32 `json:"jobTTLSecondsAfterFinished,omitempty"`
	ReconcilationFrequencyInSecs *int64 `json:"reconcilationFrequencyInSecs,omitempty"`
}

// +kubebuilder:object:generate=false
type SecurityConfig struct {
	AutoMountServiceAccountToken bool `json:"autoMountServiceAccountToken"`
}

// +kubebuilder:object:generate=false
type ServiceConfig struct {
	// ServiceClusterIPNone is a boolean flag to indicate if the service should have a clusterIP set to None.
	// If the DeploymentMode is Raw, the default value for ServiceClusterIPNone is false when the value is absent.
	ServiceClusterIPNone bool `json:"serviceClusterIPNone,omitempty"`
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

func validateIngressGateway(ingressConfig *IngressConfig) error {
	if ingressConfig.KserveIngressGateway == "" {
		return fmt.Errorf(ErrKserveIngressGatewayRequired)
	}
	splits := strings.Split(ingressConfig.KserveIngressGateway, "/")
	if len(splits) != 2 {
		return fmt.Errorf(ErrInvalidKserveIngressGatewayFormat)
	}
	errs := validation.IsDNS1123Label(splits[0])
	if len(errs) != 0 {
		return fmt.Errorf(ErrInvalidKserveIngressGatewayNamespace)
	}
	errs = validation.IsDNS1123Label(splits[1])
	if len(errs) != 0 {
		return fmt.Errorf(ErrInvalidKserveIngressGatewayName)
	}
	return nil
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
		if ingressConfig.KserveIngressGateway == "" {
			return nil, fmt.Errorf("invalid ingress config - kserveIngressGateway is required")
		}
		if err := validateIngressGateway(ingressConfig); err != nil {
			return nil, err
		}

		if ingressConfig.IngressGateway == "" {
			return nil, fmt.Errorf("invalid ingress config - ingressGateway is required")
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

		if len(ingressConfig.KnativeLocalGatewayService) == 0 {
			ingressConfig.KnativeLocalGatewayService = ingressConfig.LocalGatewayServiceName
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

func NewLocalModelConfig(clientset kubernetes.Interface) (*LocalModelConfig, error) {
	configMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	localModelConfig := &LocalModelConfig{}
	if localModel, ok := configMap.Data[LocalModelConfigName]; ok {
		err := json.Unmarshal([]byte(localModel), &localModelConfig)
		if err != nil {
			return nil, err
		}
	}
	return localModelConfig, nil
}

func NewSecurityConfig(clientset kubernetes.Interface) (*SecurityConfig, error) {
	configMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	securityConfig := &SecurityConfig{}
	if security, ok := configMap.Data[SecurityConfigName]; ok {
		err := json.Unmarshal([]byte(security), &securityConfig)
		if err != nil {
			return nil, err
		}
	}
	return securityConfig, nil
}

func NewServiceConfig(clientset kubernetes.Interface) (*ServiceConfig, error) {
	configMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(context.TODO(), constants.InferenceServiceConfigMapName, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}
	serviceConfig := &ServiceConfig{}
	if service, ok := configMap.Data[ServiceConfigName]; ok {
		err := json.Unmarshal([]byte(service), &serviceConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse service config json: %w", err)
		}
	}
	return serviceConfig, nil
}
