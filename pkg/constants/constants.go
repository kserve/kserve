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
	"os"
	"strings"

	"k8s.io/api/admissionregistration/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/serving/pkg/network"
)

// KFServing Constants
var (
	KFServingName         = "kfserving"
	KFServingAPIGroupName = "serving.kubeflow.org"
	KFServingNamespace    = getEnvOrDefault("POD_NAMESPACE", "kfserving-system")
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
	InferenceLoggerSinkUrlInternalAnnotationKey      = InferenceServiceInternalAnnotationsPrefix + "/inference-inference-inferencelogger-sink-url"
	InferenceLoggerLoggingTypeInternalAnnotationKey  = InferenceServiceInternalAnnotationsPrefix + "/inference-inference-logging-type"
	InferenceLoggerSampleInternalAnnotationKey       = InferenceServiceInternalAnnotationsPrefix + "/inference-inference-sample"
)

// Controller Constants
var (
	ControllerLabelName        = KFServingName + "-controller-manager"
	DefaultTimeout       int64 = 10
	DefaultScalingTarget       = "1"
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

type InferenceServiceEndpoint string

type InferenceServiceVerb string

// InferenceService Endpoint enums
const (
	Predictor   InferenceServiceEndpoint = "predictor"
	Explainer   InferenceServiceEndpoint = "explainer"
	Transformer InferenceServiceEndpoint = "transformer"
)

// InferenceService verb enums
const (
	Predict InferenceServiceVerb = "predict"
	Explain InferenceServiceVerb = "explain"
)

// InferenceService Endpoint Ports
const (
	InferenceServiceDefaultHttpPort     = "8080"
	InferenceServiceHttpPortWithLogging = "8081"
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

func (e InferenceServiceEndpoint) String() string {
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

func DefaultPredictorServiceName(name string) string {
	return name + "-" + string(Predictor) + "-" + InferenceServiceDefault
}

func CanaryPredictorServiceName(name string) string {
	return name + "-" + string(Predictor) + "-" + InferenceServiceCanary
}

func PredictRouteName(name string) string {
	return name + "-" + string(Predict)
}

func DefaultExplainerServiceName(name string) string {
	return name + "-" + string(Explainer) + "-" + InferenceServiceDefault
}

func CanaryExplainerServiceName(name string) string {
	return name + "-" + string(Explainer) + "-" + InferenceServiceCanary
}

func ExplainRouteName(name string) string {
	return name + "-" + string(Explain)
}

func DefaultTransformerServiceName(name string) string {
	return name + "-" + string(Transformer) + "-" + InferenceServiceDefault
}

func CanaryTransformerServiceName(name string) string {
	return name + "-" + string(Transformer) + "-" + InferenceServiceCanary
}

func DefaultServiceName(name string, endpoint InferenceServiceEndpoint) string {
	return name + "-" + endpoint.String() + "-" + InferenceServiceDefault
}

func CanaryServiceName(name string, endpoint InferenceServiceEndpoint) string {
	return name + "-" + endpoint.String() + "-" + InferenceServiceCanary
}

func RouteName(name string, verb InferenceServiceVerb) string {
	return name + "-" + verb.String()
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
	serviceHostname := network.GetServiceHostname(serviceName, metadata.Namespace)
	return fmt.Sprintf("http://%s", serviceHostname)
}

func GetInferenceServiceHttpPort(hasLogging bool) string {
	if hasLogging {
		return InferenceServiceHttpPortWithLogging
	} else {
		return InferenceServiceDefaultHttpPort
	}
}
