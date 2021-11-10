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

package constants

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"knative.dev/serving/pkg/apis/autoscaling"

	"knative.dev/pkg/network"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KServe Constants
var (
	KServeName           = "kserve"
	KServeAPIGroupName   = "serving.kserve.io"
	KServeNamespace      = getEnvOrDefault("POD_NAMESPACE", "kserve")
	KServeDefaultVersion = "v0.5.0"
)

// InferenceService Constants
var (
	InferenceServiceName          = "inferenceservice"
	InferenceServiceAPIName       = "inferenceservices"
	InferenceServicePodLabelKey   = KServeAPIGroupName + "/" + InferenceServiceName
	InferenceServiceConfigMapName = "inferenceservice-config"
)

// TrainedModel Constants
var (
	TrainedModelAllocated = KServeAPIGroupName + "/" + "trainedmodel-allocated"
)

// InferenceService MultiModel Constants
var (
	ModelConfigFileName = "models.json"
)

// Model agent Constants
const (
	AgentContainerName    = "agent"
	AgentConfigMapKeyName = "agent"
	AgentEnableFlag       = "--enable-puller"
	AgentConfigDirArgName = "--config-dir"
	AgentModelDirArgName  = "--model-dir"
)

// InferenceService Annotations
var (
	InferenceServiceGKEAcceleratorAnnotationKey = KServeAPIGroupName + "/gke-accelerator"
	DeploymentMode                              = KServeAPIGroupName + "/deploymentMode"
	EnableRoutingTagAnnotationKey               = KServeAPIGroupName + "/enable-tag-routing"
	AutoscalerClass                             = KServeAPIGroupName + "/autoscalerClass"
	AutoscalerMetrics                           = KServeAPIGroupName + "/metrics"
	TargetUtilizationPercentage                 = KServeAPIGroupName + "/targetUtilizationPercentage"
)

// InferenceService Internal Annotations
var (
	InferenceServiceInternalAnnotationsPrefix        = "internal." + KServeAPIGroupName
	StorageInitializerSourceUriInternalAnnotationKey = InferenceServiceInternalAnnotationsPrefix + "/storage-initializer-sourceuri"
	LoggerInternalAnnotationKey                      = InferenceServiceInternalAnnotationsPrefix + "/logger"
	LoggerSinkUrlInternalAnnotationKey               = InferenceServiceInternalAnnotationsPrefix + "/logger-sink-url"
	LoggerModeInternalAnnotationKey                  = InferenceServiceInternalAnnotationsPrefix + "/logger-mode"
	BatcherInternalAnnotationKey                     = InferenceServiceInternalAnnotationsPrefix + "/batcher"
	BatcherMaxBatchSizeInternalAnnotationKey         = InferenceServiceInternalAnnotationsPrefix + "/batcher-max-batchsize"
	BatcherMaxLatencyInternalAnnotationKey           = InferenceServiceInternalAnnotationsPrefix + "/batcher-max-latency"
	BatcherTimeoutInternalAnnotationKey              = InferenceServiceInternalAnnotationsPrefix + "/batcher-timeout"
	AgentShouldInjectAnnotationKey                   = InferenceServiceInternalAnnotationsPrefix + "/agent"
	AgentModelConfigVolumeNameAnnotationKey          = InferenceServiceInternalAnnotationsPrefix + "/configVolumeName"
	AgentModelConfigMountPathAnnotationKey           = InferenceServiceInternalAnnotationsPrefix + "/configMountPath"
	AgentModelDirAnnotationKey                       = InferenceServiceInternalAnnotationsPrefix + "/modelDir"
)

// Controller Constants
var (
	ControllerLabelName             = KServeName + "-controller-manager"
	DefaultPredictorTimeout   int64 = 60
	DefaultTransformerTimeout int64 = 120
	DefaultExplainerTimeout   int64 = 300
	DefaultReadinessTimeout   int32 = 600
	DefaultScalingTarget            = "1"
	DefaultMinReplicas        int   = 1
)

type AutoscalerClassType string
type AutoscalerMetricsType string

// Autoscaler Default Class
var (
	DefaultAutoscalerClass = AutoscalerClassHPA
)

// Autoscaler Class
var (
	AutoscalerClassHPA AutoscalerClassType = "hpa"
)

// Autoscaler Metrics
var (
	AutoScalerMetricsCPU AutoscalerMetricsType = "cpu"
)

// Autoscaler Class Allowed List
var AutoscalerAllowedClassList = []AutoscalerClassType{
	AutoscalerClassHPA,
}

// Autoscaler Metrics Allowed List
var AutoscalerAllowedMetricsList = []AutoscalerMetricsType{
	AutoScalerMetricsCPU,
}

// Autoscaler Default Metrics Value
var (
	DefaultCPUUtilization int32 = 80
)

// Webhook Constants
var (
	EnableKServeMutatingWebhook            = "enabled"
	EnableWebhookNamespaceSelectorEnvName  = "ENABLE_WEBHOOK_NAMESPACE_SELECTOR"
	EnableWebhookNamespaceSelectorEnvValue = "enabled"
	IsEnableWebhookNamespaceSelector       = isEnvVarMatched(EnableWebhookNamespaceSelectorEnvName, EnableWebhookNamespaceSelectorEnvValue)
	PodMutatorWebhookName                  = KServeName + "-pod-mutator-webhook"
)

// GPU Constants
const (
	NvidiaGPUResourceType = "nvidia.com/gpu"
)

// InferenceService Environment Variables
const (
	CustomSpecStorageUriEnvVarKey       = "STORAGE_URI"
	CustomSpecProtocolEnvVarKey         = "PROTOCOL"
	CustomSpecMultiModelServerEnvVarKey = "MULTI_MODEL_SERVER"
)

type InferenceServiceComponent string

type InferenceServiceVerb string

type InferenceServiceProtocol string

// Knative constants
const (
	KnativeLocalGateway   = "knative-serving/knative-local-gateway"
	KnativeIngressGateway = "knative-serving/knative-ingress-gateway"
	VisibilityLabel       = "serving.knative.dev/visibility"
)

var (
	LocalGatewayHost = "knative-local-gateway.istio-system.svc." + network.GetClusterDomainName()
)

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

// InferenceService protocol enums
const (
	ProtocolV1 InferenceServiceProtocol = "v1"
	ProtocolV2 InferenceServiceProtocol = "v2"
)

// InferenceService Endpoint Ports
const (
	InferenceServiceDefaultHttpPort    = "8080"
	InferenceServiceDefaultAgentPort   = "9081"
	InferenceServiceDefaultBatcherPort = "9082"
	CommonDefaultHttpPort              = 80
)

// Labels to put on kservice
const (
	KServiceComponentLabel = "component"
	KServiceModelLabel     = "model"
	KServiceEndpointLabel  = "endpoint"
)

// Labels for TrainedModel
const (
	ParentInferenceServiceLabel = "inferenceservice"
	InferenceServiceLabel       = "serving.kserve.io/inferenceservice"
)

// InferenceService default/canary constants
const (
	InferenceServiceDefault = "default"
	InferenceServiceCanary  = "canary"
)

// InferenceService model server args
const (
	ArgumentModelName      = "--model_name"
	ArgumentModelDir       = "--model_dir"
	ArgumentModelClassName = "--model_class_name"
	ArgumentPredictorHost  = "--predictor_host"
	ArgumentHttpPort       = "--http_port"
	ArgumentWorkers        = "--workers"
)

// InferenceService container name
const (
	InferenceServiceContainerName = "kserve-container"
)

// DefaultModelLocalMountPath is where models will be mounted by the storage-initializer
const DefaultModelLocalMountPath = "/mnt/models"

// Multi-model InferenceService
const (
	ModelConfigVolumeName = "model-config"
	ModelDirVolumeName    = "model-dir"
	ModelConfigDir        = "/mnt/configs"
	ModelDir              = DefaultModelLocalMountPath
)

var (
	ServiceAnnotationDisallowedList = []string{
		autoscaling.MinScaleAnnotationKey,
		autoscaling.MaxScaleAnnotationKey,
		StorageInitializerSourceUriInternalAnnotationKey,
		"kubectl.kubernetes.io/last-applied-configuration",
	}

	RevisionTemplateLabelDisallowedList = []string{
		VisibilityLabel,
	}
)

// raw k8s deployment, resource exist check result
type CheckResultType int

const (
	CheckResultCreate  CheckResultType = 0
	CheckResultUpdate  CheckResultType = 1
	CheckResultExisted CheckResultType = 2
	CheckResultUnknown CheckResultType = 3
)

type DeploymentModeType string

const (
	Serverless          DeploymentModeType = "Serverless"
	RawDeployment       DeploymentModeType = "RawDeployment"
	ModelMeshDeployment DeploymentModeType = "ModelMesh"
)

// GetRawServiceLabel generate native service label
func GetRawServiceLabel(service string) string {
	return "isvc." + service
}

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
	return fmt.Sprintf("%s://%s.%s.%s%s", scheme, name, namespace, domain, InferenceServicePrefix(name))
}

func InferenceServiceHostName(name string, namespace string, domain string) string {
	return fmt.Sprintf("%s.%s.%s", name, namespace, domain)
}

func DefaultPredictorServiceName(name string) string {
	return name + "-" + string(Predictor) + "-" + InferenceServiceDefault
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

func ModelConfigName(inferenceserviceName string, shardId int) string {
	return fmt.Sprintf("modelconfig-%s-%d", inferenceserviceName, shardId)
}

func InferenceServicePrefix(name string) string {
	return fmt.Sprintf("/v1/models/%s", name)
}

func PredictPath(name string, protocol InferenceServiceProtocol) string {
	if protocol == ProtocolV2 {
		return fmt.Sprintf("/v2/models/%s/infer", name)
	} else {
		return fmt.Sprintf("/v1/models/%s:predict", name)
	}
}

func ExplainPath(name string) string {
	return fmt.Sprintf("/v1/models/%s:explain", name)
}

func PredictPrefix() string {
	return fmt.Sprintf("^/v1/models/[\\w-]+(:predict)?")
}

func ExplainPrefix() string {
	return fmt.Sprintf("^/v1/models/[\\w-]+:explain$")
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
