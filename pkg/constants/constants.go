/*
Copyright 2019 kubeflow.org.

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

package constants

import (
	"fmt"
	"knative.dev/serving/pkg/network"
	"os"
	"regexp"
	"strings"

	"k8s.io/api/admissionregistration/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KFServing Constants
var (
	KFServingName           = "kfserving"
	KFServingAPIGroupName   = "serving.kubeflow.org"
	KFServingNamespace      = getEnvOrDefault("POD_NAMESPACE", "kfserving-system")
	KFServingDefaultVersion = "0.2.2"
)

// InferenceService Constants
var (
	InferenceServiceName          = "inferenceservice"
	InferenceServiceAPIName       = "inferenceservices"
	InferenceServicePodLabelKey   = KFServingAPIGroupName + "/" + InferenceServiceName
	InferenceServiceConfigMapName = "inferenceservice-config"
)

// InferenceService Annotations
var (
	InferenceServiceGKEAcceleratorAnnotationKey = KFServingAPIGroupName + "/gke-accelerator"
)

// InferenceService Internal Annotations
var (
	InferenceServiceInternalAnnotationsPrefix        = "internal." + KFServingAPIGroupName
	StorageInitializerSourceUriInternalAnnotationKey = InferenceServiceInternalAnnotationsPrefix + "/storage-initializer-sourceuri"
	LoggerInternalAnnotationKey                      = InferenceServiceInternalAnnotationsPrefix + "/logger"
	LoggerSinkUrlInternalAnnotationKey               = InferenceServiceInternalAnnotationsPrefix + "/logger-sink-url"
	LoggerModeInternalAnnotationKey                  = InferenceServiceInternalAnnotationsPrefix + "/logger-mode"
)

// Controller Constants
var (
	ControllerLabelName             = KFServingName + "-controller-manager"
	DefaultPredictorTimeout   int64 = 60
	DefaultTransformerTimeout int64 = 120
	DefaultExplainerTimeout   int64 = 300
	DefaultScalingTarget            = "1"
)

// Webhook Constants
var (
	WebhookServerName                           = KFServingName + "-webhook-server"
	WebhookServerServiceName                    = WebhookServerName + "-service"
	WebhookServerSecretName                     = WebhookServerName + "-secret"
	InferenceServiceValidatingWebhookConfigName = strings.Join([]string{InferenceServiceName, KFServingAPIGroupName}, ".")
	InferenceServiceMutatingWebhookConfigName   = strings.Join([]string{InferenceServiceName, KFServingAPIGroupName}, ".")
	InferenceServiceValidatingWebhookName       = strings.Join([]string{InferenceServiceName, WebhookServerName, "validator"}, ".")
	InferenceServiceDefaultingWebhookName       = strings.Join([]string{InferenceServiceName, WebhookServerName, "defaulter"}, ".")
	PodMutatorWebhookName                       = strings.Join([]string{InferenceServiceName, WebhookServerName, "pod-mutator"}, ".")
	WebhookFailurePolicy                        = v1beta1.Fail
	EnableKFServingMutatingWebhook              = "enabled"
	EnableWebhookNamespaceSelectorEnvName       = "ENABLE_WEBHOOK_NAMESPACE_SELECTOR"
	EnableWebhookNamespaceSelectorEnvValue      = "enabled"
	IsEnableWebhookNamespaceSelector            = isEnvVarMatched(EnableWebhookNamespaceSelectorEnvName, EnableWebhookNamespaceSelectorEnvValue)
)

// GPU Constants
const (
	NvidiaGPUResourceType = "nvidia.com/gpu"
)

// DefaultModelLocalMountPath is where models will be mounted by the storage-initializer
const DefaultModelLocalMountPath = "/mnt/models"

// InferenceService Environment Variables
const (
	CustomSpecStorageUriEnvVarKey = "STORAGE_URI"
)

type InferenceServiceComponent string

type InferenceServiceVerb string

// InferenceService Component enums
const (
	Predictor   InferenceServiceComponent = "predictor"
	Explainer   InferenceServiceComponent = "explainer"
	Transformer InferenceServiceComponent = "transformer"
)

// InferenceService verb enums
const (
	Predict InferenceServiceVerb = "predict"
	Explain InferenceServiceVerb = "explain"
)

// InferenceService Endpoint Ports
const (
	InferenceServiceDefaultHttpPort   = "8080"
	InferenceServiceDefaultLoggerPort = "8081"
)

// InferenceService default/canary constants
const (
	InferenceServiceDefault = "default"
	InferenceServiceCanary  = "canary"
)

// InferenceService model server args
const (
	ArgumentModelName     = "--model_name"
	ArgumentPredictorHost = "--predictor_host"
	ArgumentHttpPort      = "--http_port"
)

// InferenceService container name
const (
	InferenceServiceContainerName = "kfserving-container"
)

func (e InferenceServiceComponent) String() string {
	return string(e)
}

func (v InferenceServiceVerb) String() string {
	return string(v)
}

func getEnvOrDefault(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func isEnvVarMatched(envVar, matchtedValue string) bool {
	return getEnvOrDefault(envVar, "") == matchtedValue
}

func InferenceServiceURL(scheme, name, namespace, domain string) string {
	return fmt.Sprintf("%s://%s.%s.%s/%s", scheme, name, namespace, domain, PredictPrefix(name))
}

func InferenceServiceHostName(name string, namespace string, domain string) string {
	return fmt.Sprintf("%s.%s.%s", name, namespace, domain)
}

func DefaultPredictorServiceName(name string) string {
	return name + "-" + string(Predictor) + "-" + InferenceServiceDefault
}

func DefaultPredictorServiceURL(name string, namespace string, domain string) string {
	return fmt.Sprintf("%s-%s-%s.%s.%s", name, string(Predictor), InferenceServiceDefault, namespace, domain)
}

func CanaryPredictorServiceURL(name string, namespace string, domain string) string {
	return fmt.Sprintf("%s-%s-%s.%s.%s", name, string(Predictor), InferenceServiceCanary, namespace, domain)
}

func CanaryPredictorServiceName(name string) string {
	return name + "-" + string(Predictor) + "-" + InferenceServiceCanary
}

func DefaultExplainerServiceName(name string) string {
	return name + "-" + string(Explainer) + "-" + InferenceServiceDefault
}

func CanaryExplainerServiceName(name string) string {
	return name + "-" + string(Explainer) + "-" + InferenceServiceCanary
}

func DefaultTransformerServiceName(name string) string {
	return name + "-" + string(Transformer) + "-" + InferenceServiceDefault
}

func CanaryTransformerServiceName(name string) string {
	return name + "-" + string(Transformer) + "-" + InferenceServiceCanary
}

func DefaultServiceName(name string, component InferenceServiceComponent) string {
	return name + "-" + component.String() + "-" + InferenceServiceDefault
}

func CanaryServiceName(name string, component InferenceServiceComponent) string {
	return name + "-" + component.String() + "-" + InferenceServiceCanary
}

func ServiceURL(name string, hostName string) string {
	return fmt.Sprintf("http://%s/v1/models/%s", hostName, name)
}

func PredictPrefix(name string) string {
	return fmt.Sprintf("/v1/models/%s:predict", name)
}

func ExplainPrefix(name string) string {
	return fmt.Sprintf("/v1/models/%s:explain", name)
}

func VirtualServiceHostname(name string, predictorHostName string) string {
	index := strings.Index(predictorHostName, ".")
	return name + predictorHostName[index:]
}

func PredictorURL(metadata v1.ObjectMeta, isCanary bool) string {
	serviceName := DefaultPredictorServiceName(metadata.Name)
	if isCanary {
		serviceName = CanaryPredictorServiceName(metadata.Name)
	}
	return fmt.Sprintf("%s.%s", serviceName, metadata.Namespace)
}

func TransformerURL(metadata v1.ObjectMeta, isCanary bool) string {
	serviceName := DefaultTransformerServiceName(metadata.Name)
	if isCanary {
		serviceName = CanaryTransformerServiceName(metadata.Name)
	}
	return fmt.Sprintf("%s.%s", serviceName, metadata.Namespace)
}

func GetLoggerDefaultUrl(namespace string) string {
	return "http://default-broker." + namespace
}

// Should only match 1..65535, but for simplicity it matches 0-99999.
const portMatch = `(?::\d{1,5})?`

// hostRegExp returns an ECMAScript regular expression to match either host or host:<any port>
// for clusterLocalHost, we will also match the prefixes.
func HostRegExp(host string) string {
	localDomainSuffix := ".svc." + network.GetClusterDomainName()
	if !strings.HasSuffix(host, localDomainSuffix) {
		return exact(regexp.QuoteMeta(host) + portMatch)
	}
	prefix := regexp.QuoteMeta(strings.TrimSuffix(host, localDomainSuffix))
	clusterSuffix := regexp.QuoteMeta("." + network.GetClusterDomainName())
	svcSuffix := regexp.QuoteMeta(".svc")
	return exact(prefix + optional(svcSuffix+optional(clusterSuffix)) + portMatch)
}

func exact(regexp string) string {
	return "^" + regexp + "$"
}

func optional(regexp string) string {
	return "(" + regexp + ")?"
}
