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
	"fmt"
	"reflect"
	"strings"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Known error messages
const (
	MinReplicasShouldBeLessThanMaxError = "MinReplicas cannot be greater than MaxReplicas."
	MinReplicasLowerBoundExceededError  = "MinReplicas cannot be less than 0."
	MaxReplicasLowerBoundExceededError  = "MaxReplicas cannot be less than 0."
	ParallelismLowerBoundExceededError  = "Parallelism cannot be less than 0."
	UnsupportedStorageURIFormatError    = "storageUri, must be one of: [%s] or match https://{}.blob.core.windows.net/{}/{} or be an absolute or relative local path. StorageUri [%s] is not supported."
	UnsupportedStorageSpecFormatError   = "storage.spec.type, must be one of: [%s]. storage.spec.type [%s] is not supported."
	InvalidLoggerType                   = "Invalid logger type"
	InvalidISVCNameFormatError          = "The InferenceService \"%s\" is invalid: a InferenceService name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
	InvalidProtocol                     = "Invalid protocol %s. Must be one of [%s]"
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
	// ScaleMetric defines the scaling metric type watched by autoscaler
	// possible values are concurrency, rps, cpu, memory. concurrency, rps are supported via
	// Knative Pod Autoscaler(https://knative.dev/docs/serving/autoscaling/autoscaling-metrics).
	// +optional
	ScaleMetric *ScaleMetric `json:"scaleMetric,omitempty"`
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
	// Labels that will be add to the component pod.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations that will be add to the component pod.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
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
	if storageSpec != nil && storageURI != nil {
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
		return fmt.Errorf(MinReplicasLowerBoundExceededError)
	}
	if maxReplicas < 0 {
		return fmt.Errorf(MaxReplicasLowerBoundExceededError)
	}
	if *minReplicas > maxReplicas && maxReplicas != 0 {
		return fmt.Errorf(MinReplicasShouldBeLessThanMaxError)
	}
	return nil
}

func validateContainerConcurrency(containerConcurrency *int64) error {
	if containerConcurrency == nil {
		return nil
	}
	if *containerConcurrency < 0 {
		return fmt.Errorf(ParallelismLowerBoundExceededError)
	}
	return nil
}

func validateLogger(logger *LoggerSpec) error {
	if logger != nil {
		if !(logger.Mode == LogAll || logger.Mode == LogRequest || logger.Mode == LogResponse) {
			return fmt.Errorf(InvalidLoggerType)
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
		"Exactly one of [%s] must be specified in %s",
		strings.Join(implementationTypes, ", "),
		componentType.Name(),
	)
}
