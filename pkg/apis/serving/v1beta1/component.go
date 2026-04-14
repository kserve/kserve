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
	"errors"
	"fmt"
	"reflect"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// Known error messages
const (
	MinReplicasShouldBeLessThanMaxError              = "'MinReplicas' cannot be greater than MaxReplicas"
	MinReplicasLowerBoundExceededError               = "'MinReplicas' cannot be less than 0"
	MaxReplicasLowerBoundExceededError               = "'MaxReplicas' cannot be less than 0"
	ParallelismLowerBoundExceededError               = "parallelism cannot be less than 0"
	UnsupportedStorageURIFormatError                 = "storageUri, must be one of: [%s] or match https://{}.blob.core.windows.net/{}/{} or be an absolute or relative local path. StorageUri [%s] is not supported"
	UnsupportedStorageSpecFormatError                = "storage.spec.type, must be one of: [%s]. storage.spec.type [%s] is not supported"
	InvalidLoggerType                                = "invalid logger type"
	InvalidLoggerStorageConfigError                  = "invalid logger storage configuration"
	InvalidISVCNameFormatError                       = "the InferenceService \"%s\" is invalid: a InferenceService name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
	InvalidProtocol                                  = "invalid protocol %s. Must be one of [%s]"
	MissingStorageURI                                = "the InferenceService %q is invalid: StorageURI must be set for multinode enabled"
	InvalidAutoScalerError                           = "the InferenceService %q is invalid: Multinode only supports 'none' autoscaler(%s)"
	InvalidNotSupportedStorageURIProtocolError       = "the InferenceService %q is invalid: Multinode only supports 'pvc' and 'oci' Storage Protocol(%s)"
	InvalidUnknownGPUTypeError                       = "the InferenceService %q is invalid: Unknown GPU resource type. Set 'serving.kserve.io/gpu-resource-types' annotation to use custom gpu resource type"
	InvalidWorkerSpecPipelineParallelSizeValueError  = "the InferenceService %q is invalid: WorkerSpec.PipelineParallelSize cannot be less than 1(%s)"
	InvalidWorkerSpecTensorParallelSizeValueError    = "the InferenceService %q is invalid: WorkerSpec.TensorParallelSize cannot be less than 1(%s)"
	DisallowedMultipleContainersInWorkerSpecError    = "the InferenceService %q is invalid: setting multiple containers in workerSpec is not allowed"
	DisallowedWorkerSpecPipelineParallelSizeEnvError = "the InferenceService %q is invalid: setting PIPELINE_PARALLEL_SIZE in environment variables is not allowed"
	DisallowedWorkerSpecTensorParallelSizeEnvError   = "the InferenceService %q is invalid: setting TENSOR_PARALLEL_SIZE in environment variables is not allowed"
)

// SupportedStorageSpecURIPrefixList Constants
var (
	SupportedStorageSpecURIPrefixList = []string{"s3://", "hdfs://", "webhdfs://"}
)

// ComponentImplementation interface is implemented by predictor, transformer, and explainer implementations
// +kubebuilder:object:generate=false
type ComponentImplementation interface {
	Default(config *InferenceServicesConfig)
	Validate() error
	GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *corev1.Container
	GetStorageUri() *string
	GetStorageSpec() *ModelStorageSpec
	GetProtocol() constants.InferenceServiceProtocol
}

// Component interface is implemented by all specs that contain component implementations, e.g. PredictorSpec, ExplainerSpec, TransformerSpec.
// +kubebuilder:object:generate=false
type Component interface {
	GetImplementation() ComponentImplementation
	GetImplementations() []ComponentImplementation
	GetExtensions() *ComponentExtensionSpec
}

// ComponentExtensionSpec defines the deployment configuration for a given InferenceService component
type ComponentExtensionSpec struct {
	// Minimum number of replicas, defaults to 1 but can be set to 0 to enable scale-to-zero.
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// Maximum number of replicas for autoscaling.
	// +optional
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
	// ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for.
	// concurrency and rps targets are supported by Knative Pod Autoscaler
	// (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/).
	// +optional
	ScaleTarget *int32 `json:"scaleTarget,omitempty"`
	// ScaleMetric defines the scaling metric type watched by autoscaler.
	// possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via
	// Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics).
	// +optional
	ScaleMetric *ScaleMetric `json:"scaleMetric,omitempty"`
	// Type of metric to use. Options are Utilization, or AverageValue.
	// +optional
	ScaleMetricType *MetricTargetType `json:"scaleMetricType,omitempty"`
	// AutoScaling autoscaling spec which is backed up HPA or KEDA.
	// +optional
	AutoScaling *AutoScalingSpec `json:"autoScaling,omitempty"`
	// ContainerConcurrency specifies how many requests can be processed concurrently, this sets the hard limit of the container
	// concurrency(https://knative.dev/docs/serving/autoscaling/concurrency).
	// +optional
	ContainerConcurrency *int64 `json:"containerConcurrency,omitempty"`
	// TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component.
	// +optional
	TimeoutSeconds *int64 `json:"timeout,omitempty"`
	// CanaryTrafficPercent defines the traffic split percentage between the candidate revision and the last ready revision
	// +optional
	CanaryTrafficPercent *int64 `json:"canaryTrafficPercent,omitempty"`
	// Activate request/response logging and logger configurations
	// +optional
	Logger *LoggerSpec `json:"logger,omitempty"`
	// Activate request batching and batching configurations
	// +optional
	Batcher *Batcher `json:"batcher,omitempty"`
	// Labels that will be added to the component pod.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations that will be added to the component pod.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// The deployment strategy to use to replace existing pods with new ones. Only applicable for raw deployment mode.
	// +optional
	DeploymentStrategy *appsv1.DeploymentStrategy `json:"deploymentStrategy,omitempty"`
}

type AutoScalingSpec struct {
	// metrics is a list of metrics spec to be used for autoscaling
	Metrics []MetricsSpec `json:"metrics,omitempty"`
	// Behavior contains the scaling behavior configuration for the Horizontal Pod Autoscaler.
	// +optional
	Behavior *autoscalingv2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty"`
	// KEDA contains KEDA-specific autoscaling configuration.
	// When specified, KEDA ScaledObject will be used for autoscaling.
	// +optional
	KEDA *KEDAScalingConfig `json:"keda,omitempty"`
}

// KEDAScalingConfig configures KEDA ScaledObject-specific options.
// These fields map directly to KEDA ScaledObject spec fields.
type KEDAScalingConfig struct {
	// PollingInterval is the interval in seconds to check each trigger on.
	// Must be at least 1 second. Default is 30 seconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	PollingInterval *int32 `json:"pollingInterval,omitempty"`

	// CooldownPeriod is the period in seconds to wait after the last trigger reported active
	// before scaling the resource back to its minimum replica count.
	// A value of 0 means scale down immediately with no cooldown.
	// Default is 300 seconds (5 minutes).
	// +optional
	// +kubebuilder:validation:Minimum=0
	CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`

	// InitialCooldownPeriod is the period in seconds to wait after the ScaledObject is created
	// before KEDA starts evaluating triggers. Useful for model deployments where the model
	// takes time to load before it can serve traffic, preventing premature scale-up decisions.
	// +optional
	// +kubebuilder:validation:Minimum=0
	InitialCooldownPeriod *int32 `json:"initialCooldownPeriod,omitempty"`

	// IdleReplicaCount is the number of replicas KEDA will scale the resource down to
	// when there are no triggers active. This must be less than minReplicas.
	// If not set, KEDA will not scale below minReplicas.
	// +optional
	// +kubebuilder:validation:Minimum=0
	IdleReplicaCount *int32 `json:"idleReplicaCount,omitempty"`

	// Fallback defines the replica count to maintain when the scaler is in a fallback state
	// (e.g., when metrics are unavailable). This allows the deployment to hold a safe
	// replica count during metric outages rather than scaling to zero.
	// +optional
	Fallback *KEDAFallback `json:"fallback,omitempty"`

	// Advanced specifies advanced KEDA configuration options.
	// This includes HPA behavior configuration and restore-to-original replica count settings.
	// +optional
	Advanced *KEDAAdvancedConfig `json:"advanced,omitempty"`
}

// KEDAFallback defines fallback configuration for KEDA ScaledObject.
type KEDAFallback struct {
	// FailureThreshold is the number of consecutive failures before fallback is triggered.
	// +kubebuilder:validation:Minimum=0
	FailureThreshold int32 `json:"failureThreshold"`

	// Replicas is the number of replicas to maintain when in fallback mode.
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas"`

	// Behavior defines how fallback replicas are determined.
	// Valid values: static, currentReplicas, currentReplicasIfHigher, currentReplicasIfLower
	// Default: static
	// +optional
	// +kubebuilder:default=static
	// +kubebuilder:validation:Enum=static;currentReplicas;currentReplicasIfHigher;currentReplicasIfLower
	Behavior string `json:"behavior,omitempty"`
}

// KEDAAdvancedConfig specifies advanced scaling options for KEDA.
type KEDAAdvancedConfig struct {
	// HorizontalPodAutoscalerConfig specifies HPA-related configuration.
	// +optional
	HorizontalPodAutoscalerConfig *KEDAHPAConfig `json:"horizontalPodAutoscalerConfig,omitempty"`

	// RestoreToOriginalReplicaCount specifies whether to restore the original replica count
	// when the ScaledObject is deleted.
	// +optional
	RestoreToOriginalReplicaCount *bool `json:"restoreToOriginalReplicaCount,omitempty"`

	// ScalingModifiers describes advanced scaling logic options like formula.
	// +optional
	ScalingModifiers *KEDAScalingModifiers `json:"scalingModifiers,omitempty"`
}

// KEDAHPAConfig specifies horizontal scale config for KEDA-managed HPA.
type KEDAHPAConfig struct {
	// Behavior contains the scaling behavior configuration.
	// This is the same as the top-level Behavior field but applies when using KEDA.
	// +optional
	Behavior *autoscalingv2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty"`

	// Name is the name of the HPA resource created by KEDA.
	// If not set, KEDA will auto-generate the name.
	// NOTE: This field is typically not needed as KEDA manages the HPA name.
	// +optional
	Name string `json:"name,omitempty"`
}

// KEDAScalingModifiers describes advanced scaling logic options.
type KEDAScalingModifiers struct {
	// Formula is a custom formula to calculate the desired replica count.
	// +optional
	Formula string `json:"formula,omitempty"`

	// Target is the target value for the formula.
	// +optional
	Target string `json:"target,omitempty"`

	// ActivationTarget is the activation threshold for the formula.
	// +optional
	ActivationTarget string `json:"activationTarget,omitempty"`

	// MetricType specifies the metric type for the formula.
	// Valid values: AverageValue, Value
	// +optional
	// +kubebuilder:validation:Enum=AverageValue;Value
	MetricType string `json:"metricType,omitempty"`
}

// MetricsSpec specifies how to scale based on a single metric
// (only `type` and one other matching field should be set at once).
type MetricsSpec struct {
	// type is the type of metric source.  It should be one of "Resource", "External", "PodMetric".
	// "Resource" or "External" each mapping to a matching field in the object.
	Type MetricSourceType `json:"type"`

	// resource refers to a resource metric (such as those specified in
	// requests and limits) known to Kubernetes describing each pod in the
	// current scale target (e.g. CPU or memory). Such metrics are built in to
	// Kubernetes, and have special scaling options on top of those available
	// to normal per-pod metrics using the "pods" source.
	// +optional
	Resource *ResourceMetricSource `json:"resource,omitempty"`

	// external refers to a global metric that is not associated
	// with any Kubernetes object. It allows autoscaling based on information
	// coming from components running outside of cluster
	// (for example length of queue in cloud messaging service, or
	// QPS from load balancer running outside of cluster).
	// +optional
	External *ExternalMetricSource `json:"external,omitempty"`

	// pods refers to a metric describing each pod in the current scale target
	// (for example, transactions-processed-per-second).  The values will be
	// averaged together before being compared to the target value.
	// +optional
	PodMetric *PodMetricSource `json:"podmetric,omitempty"`

	// Name is the name of the trigger. Must be unique within the ScaledObject.
	// Used by KEDA to identify triggers.
	// +optional
	Name string `json:"name,omitempty"`

	// UseCachedMetrics determines whether KEDA should use cached metrics.
	// Not supported for cpu, memory, or cron scalers.
	// +optional
	UseCachedMetrics bool `json:"useCachedMetrics,omitempty"`
}

// MetricSourceType indicates the type of metric.
// +kubebuilder:validation:Enum=Resource;External;PodMetric
type MetricSourceType string

const (
	// ResourceMetricSourceType is a resource metric known to Kubernetes, as
	// specified in requests and limits, describing each pod in the current
	// scale target (e.g. CPU or memory).  Such metrics are built in to
	// Kubernetes, and have special scaling options on top of those available
	// to normal per-pod metrics (the "pods" source).
	ResourceMetricSourceType MetricSourceType = "Resource"
	// ExternalMetricSourceType is a global metric that is not associated
	// with any Kubernetes object. It allows autoscaling based on information
	// coming from components running outside of cluster
	// (for example length of queue in cloud messaging service, or
	// QPS from loadbalancer running outside of cluster).
	ExternalMetricSourceType MetricSourceType = "External"
	// PodMetricSourceType indicates a metric describing each pod in the current
	// scale target (for example, transactions-processed-per-second).  The values
	// will be averaged together before being compared to the target value.
	PodMetricSourceType MetricSourceType = "PodMetric"
)

type ResourceMetricSource struct {
	// name is the name of the resource in question.
	Name ResourceMetric `json:"name"`

	// target specifies the target value for the given metric
	Target MetricTarget `json:"target"`
}

type ExternalMetricSource struct {
	// metric identifies the target metric by name and selector
	Metric ExternalMetrics `json:"metric"`

	// authenticationRef is a reference to the authentication information
	// for more information see: https://keda.sh/docs/2.17/scalers/prometheus/#authentication-parameters
	// +optional
	Authentication *ExtMetricAuthentication `json:"authenticationRef,omitempty"`

	// target specifies the target value for the given metric
	Target MetricTarget `json:"target"`
}

// PodMetricSource indicates how to scale on a metric describing each pod in
// the current scale target (for example, transactions-processed-per-second).
// The values will be averaged together before being compared to the target
// value.
type PodMetricSource struct {
	// metric identifies the target metric by name and selector
	Metric PodMetrics `json:"metric"`

	// target specifies the target value for the given metric
	Target MetricTarget `json:"target"`
}

type AuthenticationRef struct {
	// name is the name of the authentication secret
	Name string `json:"name"`
}

type ExtMetricAuthentication struct {
	// authenticationRef is a reference to the authentication information
	// for more information see: https://keda.sh/docs/2.17/scalers/prometheus/#authentication-parameters
	AuthenticationRef AuthenticationRef `json:"authenticationRef"`
	// authModes defines the authentication modes for the metrics backend
	// possible values are bearer, basic, tls.
	// for more information see: https://keda.sh/docs/2.17/scalers/prometheus/#authentication-parameters
	// +optional
	AuthModes string `json:"authModes,omitempty"`
}

// MetricTarget defines the target value, average value, or average utilization of a specific metric
type MetricTarget struct {
	// type represents whether the metric type is Utilization, Value, or AverageValue
	// +optional
	Type MetricTargetType `json:"type"`

	// value is the target value of the metric (as a quantity).
	// +optional
	Value *MetricQuantity `json:"value,omitempty"`

	// averageValue is the target value of the average of the
	// metric across all relevant pods (as a quantity)
	// +optional
	AverageValue *MetricQuantity `json:"averageValue,omitempty"`

	// averageUtilization is the target value of the average of the
	// resource metric across all relevant pods, represented as a percentage of
	// the requested value of the resource for the pods.
	// Currently only valid for Resource metric source type
	// +optional
	AverageUtilization *int32 `json:"averageUtilization,omitempty"`
}

// MetricTargetType specifies the type of metric being targeted, and should be either
// "Value", "AverageValue", or "Utilization"
// +kubebuilder:validation:Enum=Utilization;Value;AverageValue
type MetricTargetType string

const (
	// UtilizationMetricType declares a MetricTarget is an AverageUtilization value
	UtilizationMetricType MetricTargetType = "Utilization"
	// ValueMetricType declares a MetricTarget is a raw value
	ValueMetricType MetricTargetType = "Value"
	// AverageValueMetricType declares a MetricTarget is an
	AverageValueMetricType MetricTargetType = "AverageValue"
)

type ExternalMetrics struct {
	// MetricsBackend defines the scaling metric type watched by autoscaler
	// possible values are prometheus, graphite.
	// +optional
	Backend MetricsBackend `json:"backend"`
	// Address of MetricsBackend server.
	// +optional
	ServerAddress string `json:"serverAddress,omitempty"`
	// Query to run to get metrics from MetricsBackend
	// +optional
	Query string `json:"query,omitempty"`
	// For namespaced query
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type PodMetrics struct {
	// Backend defines the scaling metric type watched by the autoscaler.
	// Possible value: opentelemetry.
	// +optional
	Backend PodsMetricsBackend `json:"backend"`
	// ServerAddress specifies the address of the PodsMetricsBackend server.
	// +optional
	ServerAddress string `json:"serverAddress,omitempty"`
	// MetricNames is the list of metric names in the backend.
	// +optional
	MetricNames []string `json:"metricNames,omitempty"`
	// Query specifies the query to run to get metrics from the PodsMetricsBackend.
	// +optional
	Query string `json:"query,omitempty"`
	// OperationOverTime specifies the operation to aggregate the metrics over time.
	// Possible values are last_one, avg, max, min, rate, count. Default is 'last_one'.
	// +optional
	OperationOverTime string `json:"operationOverTime,omitempty"`
}

// ScaleMetric enum
// +kubebuilder:validation:Enum=cpu;memory;concurrency;rps
type ScaleMetric string

const (
	MetricCPU         ScaleMetric = "cpu"
	MetricMemory      ScaleMetric = "memory"
	MetricConcurrency ScaleMetric = "concurrency"
	MetricRPS         ScaleMetric = "rps"
)

// ResourceMetric enum
// +kubebuilder:validation:Enum=cpu;memory
type ResourceMetric string

const (
	ResourceMetricCPU    ResourceMetric = "cpu"
	ResourceMetricMemory ResourceMetric = "memory"
)

// Default the ComponentExtensionSpec
func (s *ComponentExtensionSpec) Default(config *InferenceServicesConfig) {
	// Apply defaults for KEDA configuration
	if s.AutoScaling != nil && s.AutoScaling.KEDA != nil {
		keda := s.AutoScaling.KEDA

		// Set default polling interval to 30 seconds if not specified
		if keda.PollingInterval == nil {
			defaultPollingInterval := int32(30)
			keda.PollingInterval = &defaultPollingInterval
		}

		// Set default cooldown period to 300 seconds (5 minutes) if not specified
		if keda.CooldownPeriod == nil {
			defaultCooldownPeriod := int32(300)
			keda.CooldownPeriod = &defaultCooldownPeriod
		}

		// Set default fallback behavior to "static" if fallback is specified but behavior is not
		if keda.Fallback != nil && keda.Fallback.Behavior == "" {
			keda.Fallback.Behavior = "static"
		}
	}
}

// Validate the ComponentExtensionSpec
func (s *ComponentExtensionSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateContainerConcurrency(s.ContainerConcurrency),
		validateReplicas(s.MinReplicas, s.MaxReplicas),
		validateLogger(s.Logger),
	})
}

func validateStorageSpec(storageSpec *ModelStorageSpec, storageURI *string) error {
	if storageSpec == nil {
		return nil
	}
	if storageURI != nil {
		if utils.IsPrefixSupported(*storageURI, SupportedStorageSpecURIPrefixList) {
			return nil
		} else {
			return fmt.Errorf(UnsupportedStorageURIFormatError, strings.Join(SupportedStorageSpecURIPrefixList, ", "), *storageURI)
		}
	}
	if storageSpec.Parameters != nil {
		for k, v := range *storageSpec.Parameters {
			if k == "type" {
				if utils.IsPrefixSupported(v+"://", SupportedStorageSpecURIPrefixList) {
					return nil
				} else {
					return fmt.Errorf(UnsupportedStorageSpecFormatError, strings.Join(SupportedStorageSpecURIPrefixList, ", "), v)
				}
			}
		}
	}
	return nil
}

func validateReplicas(minReplicas *int32, maxReplicas int32) error {
	if minReplicas == nil {
		minReplicas = &constants.DefaultMinReplicas
	}
	if *minReplicas < 0 {
		return errors.New(MinReplicasLowerBoundExceededError)
	}
	if maxReplicas < 0 {
		return errors.New(MaxReplicasLowerBoundExceededError)
	}
	if *minReplicas > maxReplicas && maxReplicas != 0 {
		return errors.New(MinReplicasShouldBeLessThanMaxError)
	}
	return nil
}

func validateContainerConcurrency(containerConcurrency *int64) error {
	if containerConcurrency == nil {
		return nil
	}
	if *containerConcurrency < 0 {
		return errors.New(ParallelismLowerBoundExceededError)
	}
	return nil
}

func validateLogger(logger *LoggerSpec) error {
	if logger != nil {
		if logger.Mode != LogAll && logger.Mode != LogRequest && logger.Mode != LogResponse {
			return errors.New(InvalidLoggerType)
		}
		if logger.Storage != nil {
			if logger.Storage.Path == nil || logger.Storage.Parameters == nil || logger.Storage.StorageKey == nil {
				return errors.New(InvalidLoggerStorageConfigError)
			}
		}
	}

	return nil
}

func validateExactlyOneImplementation(component Component) error {
	if len(component.GetImplementations()) != 1 {
		return ExactlyOneErrorFor(component)
	}
	return nil
}

// FirstNonNilComponent returns the first non nil object or returns nil
func FirstNonNilComponent(objects []ComponentImplementation) ComponentImplementation {
	if results := NonNilComponents(objects); len(results) > 0 {
		return results[0]
	}
	return nil
}

// NonNilComponents returns components that are not nil
func NonNilComponents(objects []ComponentImplementation) (results []ComponentImplementation) {
	for _, object := range objects {
		if !reflect.ValueOf(object).IsNil() {
			results = append(results, object)
		}
	}
	return results
}

// ExactlyOneErrorFor creates an error for the component's one-of semantic.
func ExactlyOneErrorFor(component Component) error {
	componentType := reflect.ValueOf(component).Type().Elem()
	implementationTypes := make([]string, 0, componentType.NumField()-1)
	for i := range componentType.NumField() - 1 {
		implementationTypes = append(implementationTypes, componentType.Field(i).Name)
	}
	return fmt.Errorf(
		"exactly one of [%s] must be specified in %s",
		strings.Join(implementationTypes, ", "),
		componentType.Name(),
	)
}
