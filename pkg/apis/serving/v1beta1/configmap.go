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
	"errors"
	"fmt"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// ConfigMap Keys
const (
	ExplainerConfigKeyName        = "explainers"
	InferenceServiceConfigKeyName = "inferenceService"
	IngressConfigKeyName          = "ingress"
	DeployConfigName              = "deploy"
	LocalModelConfigName          = "localModel"
	SecurityConfigName            = "security"
	ServiceConfigName             = "service"
	ResourceConfigName            = "resource"
	MultiNodeConfigKeyName        = "multiNode"
	OtelCollectorConfigName       = "opentelemetryCollector"
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

type OtelCollectorConfig struct {
	ScrapeInterval         string `json:"scrapeInterval,omitempty"`
	MetricReceiverEndpoint string `json:"metricReceiverEndpoint,omitempty"`
	MetricScalerEndpoint   string `json:"metricScalerEndpoint,omitempty"`
}

// +kubebuilder:object:generate=false
type InferenceServicesConfig struct {
	// Explainer configurations
	Explainers ExplainersConfig `json:"explainers"`
	// ServiceAnnotationDisallowedList is a list of annotations that are not allowed to be propagated to Knative
	// revisions
	ServiceAnnotationDisallowedList []string `json:"serviceAnnotationDisallowedList,omitempty"`
	// ServiceLabelDisallowedList is a list of labels that are not allowed to be propagated to Knative revisions
	ServiceLabelDisallowedList []string `json:"serviceLabelDisallowedList,omitempty"`
	// Resource configurations
	Resource ResourceConfig `json:"resource,omitempty"`
}

// +kubebuilder:object:generate=false
type MultiNodeConfig struct {
	// CustomGPUResourceTypeList is a list of custom GPU resource types that are allowed to be used in the ServingRuntime and inferenceService
	CustomGPUResourceTypeList []string `json:"customGPUResourceTypeList,omitempty"`
}

// +kubebuilder:object:generate=false
type IngressConfig struct {
	EnableGatewayAPI           bool      `json:"enableGatewayApi,omitempty"`
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
type OauthConfig struct {
	Image         string `json:"image"`
	CpuLimit      string `json:"cpuLimit"`
	CpuRequest    string `json:"cpuRequest"`
	MemoryLimit   string `json:"memoryLimit"`
	MemoryRequest string `json:"memoryRequest"`
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
	DisableVolumeManagement      bool   `json:"disableVolumeManagement,omitempty"`
}

// +kubebuilder:object:generate=false
type ResourceConfig struct {
	CPULimit      string `json:"cpuLimit,omitempty"`
	MemoryLimit   string `json:"memoryLimit,omitempty"`
	CPURequest    string `json:"cpuRequest,omitempty"`
	MemoryRequest string `json:"memoryRequest,omitempty"`
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

func GetInferenceServiceConfigMap(ctx context.Context, clientset kubernetes.Interface) (*corev1.ConfigMap, error) {
	if configMap, err := clientset.CoreV1().ConfigMaps(constants.KServeNamespace).Get(
		ctx, constants.InferenceServiceConfigMapName, metav1.GetOptions{}); err != nil {
		return nil, err
	} else {
		return configMap, nil
	}
}

func NewOtelCollectorConfig(isvcConfigMap *corev1.ConfigMap) (*OtelCollectorConfig, error) {
	otelConfig := &OtelCollectorConfig{}
	if otel, ok := isvcConfigMap.Data[OtelCollectorConfigName]; ok {
		err := json.Unmarshal([]byte(otel), otelConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse otel config json: %w", err)
		}
	}
	return otelConfig, nil
}

func NewInferenceServicesConfig(isvcConfigMap *corev1.ConfigMap) (*InferenceServicesConfig, error) {
	icfg := &InferenceServicesConfig{}
	for _, err := range []error{
		getComponentConfig(ExplainerConfigKeyName, isvcConfigMap, &icfg.Explainers),
		getComponentConfig(InferenceServiceConfigKeyName, isvcConfigMap, &icfg),
	} {
		if err != nil {
			return nil, err
		}
	}

	if isvc, ok := isvcConfigMap.Data[InferenceServiceConfigKeyName]; ok {
		errisvc := json.Unmarshal([]byte(isvc), &icfg)
		if errisvc != nil {
			return nil, fmt.Errorf("unable to parse isvc config json: %w", errisvc)
		}
		if icfg.ServiceAnnotationDisallowedList == nil {
			icfg.ServiceAnnotationDisallowedList = constants.ServiceAnnotationDisallowedList
		} else {
			icfg.ServiceAnnotationDisallowedList = append(
				constants.ServiceAnnotationDisallowedList,
				icfg.ServiceAnnotationDisallowedList...)
		}
		if icfg.ServiceLabelDisallowedList == nil {
			icfg.ServiceLabelDisallowedList = constants.RevisionTemplateLabelDisallowedList
		} else {
			icfg.ServiceLabelDisallowedList = append(
				constants.RevisionTemplateLabelDisallowedList,
				icfg.ServiceLabelDisallowedList...)
		}
	}
	return icfg, nil
}

func NewMultiNodeConfig(isvcConfigMap *corev1.ConfigMap) (*MultiNodeConfig, error) {
	mncfg := &MultiNodeConfig{}
	for _, err := range []error{
		getComponentConfig(MultiNodeConfigKeyName, isvcConfigMap, &mncfg),
	} {
		if err != nil {
			return nil, err
		}
	}

	if mncfg.CustomGPUResourceTypeList == nil {
		mncfg.CustomGPUResourceTypeList = []string{}
	}

	// update global GPU resource type list
	utils.UpdateGlobalGPUResourceTypeList(append(mncfg.CustomGPUResourceTypeList, constants.DefaultGPUResourceTypeList...))
	return mncfg, nil
}

func validateIngressGateway(ingressConfig *IngressConfig) error {
	if ingressConfig.KserveIngressGateway == "" {
		return errors.New(ErrKserveIngressGatewayRequired)
	}
	splits := strings.Split(ingressConfig.KserveIngressGateway, "/")
	if len(splits) != 2 {
		return errors.New(ErrInvalidKserveIngressGatewayFormat)
	}
	errs := validation.IsDNS1123Label(splits[0])
	if len(errs) != 0 {
		return errors.New(ErrInvalidKserveIngressGatewayNamespace)
	}
	errs = validation.IsDNS1123Label(splits[1])
	if len(errs) != 0 {
		return errors.New(ErrInvalidKserveIngressGatewayName)
	}
	return nil
}

func NewIngressConfig(isvcConfigMap *corev1.ConfigMap) (*IngressConfig, error) {
	ingressConfig := &IngressConfig{}
	if ingress, ok := isvcConfigMap.Data[IngressConfigKeyName]; ok {
		err := json.Unmarshal([]byte(ingress), &ingressConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse ingress config json: %w", err)
		}
		if ingressConfig.EnableGatewayAPI {
			if ingressConfig.KserveIngressGateway == "" {
				return nil, errors.New("invalid ingress config - kserveIngressGateway is required")
			}
			if err := validateIngressGateway(ingressConfig); err != nil {
				return nil, err
			}
		}
		if ingressConfig.IngressGateway == "" {
			return nil, errors.New("invalid ingress config - ingressGateway is required")
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
				return nil, errors.New("invalid ingress config - ingressDomain is required if pathTemplate is given")
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

func getComponentConfig(key string, configMap *corev1.ConfigMap, componentConfig interface{}) error {
	if data, ok := configMap.Data[key]; ok {
		err := json.Unmarshal([]byte(data), componentConfig)
		if err != nil {
			return fmt.Errorf("unable to unmarshall %v json string due to %w ", key, err)
		}
	}
	return nil
}

func NewDeployConfig(isvcConfigMap *corev1.ConfigMap) (*DeployConfig, error) {
	deployConfig := &DeployConfig{}
	if deploy, ok := isvcConfigMap.Data[DeployConfigName]; ok {
		err := json.Unmarshal([]byte(deploy), &deployConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse deploy config json: %w", err)
		}

		if deployConfig.DefaultDeploymentMode == "" {
			return nil, errors.New("invalid deploy config, defaultDeploymentMode is required")
		}

		if deployConfig.DefaultDeploymentMode != string(constants.Serverless) &&
			deployConfig.DefaultDeploymentMode != string(constants.RawDeployment) &&
			deployConfig.DefaultDeploymentMode != string(constants.ModelMeshDeployment) {
			return nil, errors.New("invalid deployment mode. Supported modes are Serverless," +
				" RawDeployment and ModelMesh")
		}
	}
	return deployConfig, nil
}

func NewLocalModelConfig(isvcConfigMap *corev1.ConfigMap) (*LocalModelConfig, error) {
	localModelConfig := &LocalModelConfig{}
	if localModel, ok := isvcConfigMap.Data[LocalModelConfigName]; ok {
		err := json.Unmarshal([]byte(localModel), &localModelConfig)
		if err != nil {
			return nil, err
		}
	}
	return localModelConfig, nil
}

func NewSecurityConfig(isvcConfigMap *corev1.ConfigMap) (*SecurityConfig, error) {
	securityConfig := &SecurityConfig{}
	if security, ok := isvcConfigMap.Data[SecurityConfigName]; ok {
		err := json.Unmarshal([]byte(security), &securityConfig)
		if err != nil {
			return nil, err
		}
	}
	return securityConfig, nil
}

func NewServiceConfig(isvcConfigMap *corev1.ConfigMap) (*ServiceConfig, error) {
	serviceConfig := &ServiceConfig{ServiceClusterIPNone: true}
	if service, ok := isvcConfigMap.Data[ServiceConfigName]; ok {
		err := json.Unmarshal([]byte(service), &serviceConfig)
		if err != nil {
			return nil, fmt.Errorf("unable to parse service config json: %w", err)
		}
	}
	return serviceConfig, nil
}
