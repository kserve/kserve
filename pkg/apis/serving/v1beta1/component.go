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

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	InvalidISVCNameFormatError                       = "the InferenceService \"%s\" is invalid: a InferenceService name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
	InvalidProtocol                                  = "invalid protocol %s. Must be one of [%s]"
	MissingStorageURI                                = "the InferenceService %q is invalid: StorageURI must be set for multinode enabled"
	InvalidAutoScalerError                           = "the InferenceService %q is invalid: Multinode only supports 'external' autoscaler(%s)"
	InvalidNotSupportedStorageURIProtocolError       = "the InferenceService %q is invalid: Multinode only supports 'pvc' Storage Protocol(%s)"
	InvalidCustomGPUTypesAnnotationFormatError       = "the InferenceService %q is invalid: invalid format for %s annotation: must be a valid JSON array"
	InvalidUnknownGPUTypeError                       = "the InferenceService %q is invalid: Unknown GPU resource type. Set 'serving.kserve.io/gpu-resource-types' annotation to use custom gpu resource type"
	InvalidWorkerSpecPipelineParallelSizeValueError  = "the InferenceService %q is invalid: WorkerSpec.PipelineParallelSize cannot be less than 2(%s)"
	InvalidWorkerSpecTensorParallelSizeValueError    = "the InferenceService %q is invalid: WorkerSpec.TensorParallelSize cannot be less than 1(%s)"
	DisallowedMultipleContainersInWorkerSpecError    = "the InferenceService %q is invalid: setting multiple containers in workerSpec is not allowed"
	DisallowedWorkerSpecPipelineParallelSizeEnvError = "the InferenceService %q is invalid: setting PIPELINE_PARALLEL_SIZE in environment variables is not allowed"
	DisallowedWorkerSpecTensorParallelSizeEnvError   = "the InferenceService %q is invalid: setting TENSOR_PARALLEL_SIZE in environment variables is not allowed"
)

// Constants
var (
	SupportedStorageSpecURIPrefixList = []string{"s3://", "hdfs://", "webhdfs://"}
)

// ComponentImplementation interface is implemented by predictor, transformer, and explainer implementations
// +kubebuilder:object:generate=false
type ComponentImplementation interface {
	Default(config *InferenceServicesConfig)
	Validate() error
	GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig, predictorHost ...string) *v1.Container
	GetStorageUri() *string
	GetStorageSpec() *StorageSpec
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
	MinReplicas *int `json:"minReplicas,omitempty"`
	// Maximum number of replicas for autoscaling.
	// +optional
	MaxReplicas int `json:"maxReplicas,omitempty"`
	// ScaleTarget specifies the integer target value of the metric type the Autoscaler watches for.
	// concurrency and rps targets are supported by Knative Pod Autoscaler
	// (https://knative.dev/docs/serving/autoscaling/autoscaling-targets/).
	// +optional
	ScaleTarget *int `json:"scaleTarget,omitempty"`
	// AutoScaling to be used for autoscaling spec. Could be used for Keda autoscaling.
	// +optional
	AutoScaling []AutoScalingSpec `json:"autoScaling,omitempty"`
	// ScaleMetric defines the scaling metric type watched by autoscaler
	// possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via
	// Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics).
	// +optional
	ScaleMetric *ScaleMetric `json:"scaleMetric,omitempty"`
	// Type of metric to use. Options are Utilization, or AverageValue.
	// +optional
	ScaleMetricType *v2.MetricTargetType `json:"scaleMetricType,omitempty"`
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

// AutoScalingSpec specifies how to scale based on a single metric
// (only `type` and one other matching field should be set at once).
type AutoScalingSpec struct {
	// type is the type of metric source.  It should be one of "Resource", "External",
	// "Resource" or "External" each mapping to a matching field in the object.
	Type MetricSourceType `json:"type,omitempty"`

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
	// QPS from loadbalancer running outside of cluster).
	// +optional
	External *ExternalMetricSource `json:"external,omitempty"`
}

// MetricSourceType indicates the type of metric.
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
)

type ResourceMetricSource struct {
	// name is the name of the resource in question.
	Name *ScaleMetric `json:"name,omitempty"`

	// target specifies the target value for the given metric
	Target MetricTarget `json:"target,omitempty"`
}

type ExternalMetricSource struct {
	// metric identifies the target metric by name and selector
	Metric MetricIdentifier `json:"metric,omitempty"`

	// target specifies the target value for the given metric
	Target MetricTarget `json:"target,omitempty"`
}

// MetricTarget defines the target value, average value, or average utilization of a specific metric
type MetricTarget struct {
	// type represents whether the metric type is Utilization, Value, or AverageValue
	Type MetricTargetType `json:"type"`

	// value is the target value of the metric (as a quantity).
	// +optional
	Value *resource.Quantity `json:"value,omitempty"`

	// averageValue is the target value of the average of the
	// metric across all relevant pods (as a quantity)
	// +optional
	AverageValue *resource.Quantity `json:"averageValue,omitempty"`

	// averageUtilization is the target value of the average of the
	// resource metric across all relevant pods, represented as a percentage of
	// the requested value of the resource for the pods.
	// Currently only valid for Resource metric source type
	// +optional
	AverageUtilization *int32 `json:"averageUtilization,omitempty"`
}

// MetricTargetType specifies the type of metric being targeted, and should be either
// "Value", "AverageValue", or "Utilization"
type MetricTargetType string

const (
	// UtilizationMetricType declares a MetricTarget is an AverageUtilization value
	UtilizationMetricType MetricTargetType = "Utilization"
	// ValueMetricType declares a MetricTarget is a raw value
	ValueMetricType MetricTargetType = "Value"
	// AverageValueMetricType declares a MetricTarget is an
	AverageValueMetricType MetricTargetType = "AverageValue"
)

type MetricIdentifier struct {
	// MetricsBackend defines the scaling metric type watched by autoscaler
	// possible values are prometheus, graphite.
	// +optional
	Backend *MetricsBackend `json:"backend,omitempty"`
	// Address of MetricsBackend server.
	// +optional
	ServerAddress string `json:"serverAddress,omitempty"`
	// Query to run to get metrics from MetricsBackend
	// +optional
	Query string `json:"query,omitempty"`
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

// Default the ComponentExtensionSpec
func (s *ComponentExtensionSpec) Default(config *InferenceServicesConfig) {}

// Validate the ComponentExtensionSpec
func (s *ComponentExtensionSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateContainerConcurrency(s.ContainerConcurrency),
		validateReplicas(s.MinReplicas, s.MaxReplicas),
		validateLogger(s.Logger),
	})
}

func validateStorageSpec(storageSpec *StorageSpec, storageURI *string) error {
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

func validateReplicas(minReplicas *int, maxReplicas int) error {
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
		if !(logger.Mode == LogAll || logger.Mode == LogRequest || logger.Mode == LogResponse) {
			return errors.New(InvalidLoggerType)
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
	implementationTypes := []string{}
	for i := 0; i < componentType.NumField()-1; i++ {
		implementationTypes = append(implementationTypes, componentType.Field(i).Name)
	}
	return fmt.Errorf(
		"exactly one of [%s] must be specified in %s",
		strings.Join(implementationTypes, ", "),
		componentType.Name(),
	)
}
