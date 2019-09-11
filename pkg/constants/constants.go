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
	"os"
	"strings"

	"k8s.io/api/admissionregistration/v1beta1"
)

// KFServing Constants
var (
	KFServingName         = "kfserving"
	KFServingAPIGroupName = "serving.kubeflow.org"
	KFServingNamespace    = getEnvOrDefault("POD_NAMESPACE", "kfserving-system")
)

// KFService Constants
var (
	KFServiceName          = "kfservice"
	KFServiceAPIName       = "kfservices"
	KFServicePodLabelKey   = KFServingAPIGroupName + "/" + KFServiceName
	KFServiceConfigMapName = "kfservice-config"
)

// KFService Annotations
var (
	KFServiceGKEAcceleratorAnnotationKey = KFServingAPIGroupName + "/gke-accelerator"
)

// KFService Internal Annotations
var (
	KFServiceInternalAnnotationsPrefix               = "internal." + KFServingAPIGroupName
	StorageInitializerSourceUriInternalAnnotationKey = KFServiceInternalAnnotationsPrefix + "/storage-initializer-sourceuri"
)

// Controller Constants
var (
	ControllerLabelName        = KFServingName + "-controller-manager"
	DefaultTimeout       int64 = 300
	DefaultScalingTarget       = "1"
)

// Webhook Constants
var (
	WebhookServerName                    = KFServingName + "-webhook-server"
	WebhookServerServiceName             = WebhookServerName + "-service"
	WebhookServerSecretName              = WebhookServerName + "-secret"
	KFServiceValidatingWebhookConfigName = strings.Join([]string{KFServiceName, KFServingAPIGroupName}, ".")
	KFServiceMutatingWebhookConfigName   = strings.Join([]string{KFServiceName, KFServingAPIGroupName}, ".")
	KFServiceValidatingWebhookName       = strings.Join([]string{KFServiceName, WebhookServerName, "validator"}, ".")
	KFServiceDefaultingWebhookName       = strings.Join([]string{KFServiceName, WebhookServerName, "defaulter"}, ".")
	DeploymentMutatorWebhookName         = strings.Join([]string{KFServiceName, WebhookServerName, "deployment-mutator"}, ".")
	WebhookFailurePolicy                 = v1beta1.Fail
)

// GPU Constants
const (
	NvidiaGPUResourceType = "nvidia.com/gpu"
)

// DefaultModelLocalMountPath is where models will be mounted by the storage-initializer
const DefaultModelLocalMountPath = "/mnt/models"

// KFService Environment Variables
const (
	CustomSpecStorageUriEnvVarKey = "STORAGE_URI"
)

type KFServiceEndpoint string

type KFServiceVerb string

// KFService Endpoint enums
const (
	Predictor   KFServiceEndpoint = "predictor"
	Explainer   KFServiceEndpoint = "explainer"
	Transformer KFServiceEndpoint = "transformer"
)

// KFService verb enums
const (
	Predict KFServiceVerb = "predict"
	Explain KFServiceVerb = "explain"
)

// KFService default/canary constants
const (
	KFServiceDefault = "default"
	KFServiceCanary  = "canary"
)

func (e KFServiceEndpoint) String() string {
	return string(e)
}

func (v KFServiceVerb) String() string {
	return string(v)
}

func getEnvOrDefault(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func DefaultPredictorServiceName(name string) string {
	return name + "-" + string(Predictor) + "-" + KFServiceDefault
}

func CanaryPredictorServiceName(name string) string {
	return name + "-" + string(Predictor) + "-" + KFServiceCanary
}

func PredictRouteName(name string) string {
	return name + "-" + string(Predict)
}

func DefaultExplainerServiceName(name string) string {
	return name + "-" + string(Explainer) + "-" + KFServiceDefault
}

func CanaryExplainerServiceName(name string) string {
	return name + "-" + string(Explainer) + "-" + KFServiceCanary
}

func ExplainRouteName(name string) string {
	return name + "-" + string(Explain)
}

func DefaultTransformerServiceName(name string) string {
	return name + "-" + string(Transformer) + "-" + KFServiceDefault
}

func CanaryTransformerServiceName(name string) string {
	return name + "-" + string(Transformer) + "-" + KFServiceCanary
}

func TransformerRouteName(name string) string {
	return name + "-" + string(Transformer)
}

func DefaultServiceName(name string, endpoint KFServiceEndpoint) string {
	return name + "-" + endpoint.String() + "-" + KFServiceDefault
}

func CanaryServiceName(name string, endpoint KFServiceEndpoint) string {
	return name + "-" + endpoint.String() + "-" + KFServiceCanary
}

func RouteName(name string, verb KFServiceVerb) string {
	return name + "-" + verb.String()
}

func VirtualServiceName(name string) string {
	return name + "-vs"
}
