package v1beta1

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"
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
	InvalidLoggerType                   = "Invalid logger type"
)

// Constants
var (
	SupportedStorageURIPrefixList = []string{"gs://", "s3://", "pvc://", "file://"}
	AzureBlobURIRegEx             = "https://(.+?).blob.core.windows.net/(.+)"
)

// ComponentImplementation interface is implemented by predictor, transformer, and explainer implementations
// +kubebuilder:object:generate=false
type ComponentImplementation interface {
	Default(config *InferenceServicesConfig)
	Validate() error
	GetContainer(metadata metav1.ObjectMeta, extensions *ComponentExtensionSpec, config *InferenceServicesConfig) *v1.Container
	GetStorageUri() *string
}

// Component interface is implemented by all specs that contain component implentations, e.g. PredictorSpec, ExplainerSpec, TransformerSpec.
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
	// ContainerConcurrency specifies how many requests can be processed concurrently, this sets the hard limit of the container
	// concurrency(https://knative.dev/docs/serving/autoscaling/concurrency).
	// +optional
	ContainerConcurrency *int64 `json:"containerConcurrency,omitempty"`
	// TimeoutSeconds specifies the number of seconds to wait before timing out a request to the component.
	// +optional
	TimeoutSeconds *int64 `json:"timeout,omitempty"`
	// CanaryTrafficPercent defines the traffic split percentage between the candidate revision and the last ready revision
	// +optional
	CanaryTrafficPercent *int `json:"canaryTrafficPercent,omitempty"`
	// Activate request/response logging and logger configurations
	// +optional
	LoggerSpec *LoggerSpec `json:"logger,omitempty"`
	// Activate request batching and batching configurations
	// +optional
	Batcher *Batcher `json:"batcher,omitempty"`
}

// Default the ComponentExtensionSpec
func (s *ComponentExtensionSpec) Default(config *InferenceServicesConfig) {}

// Validate the ComponentExtensionSpec
func (s *ComponentExtensionSpec) Validate() error {
	return utils.FirstNonNilError([]error{
		validateContainerConcurrency(s.ContainerConcurrency),
		validateReplicas(s.MinReplicas, s.MaxReplicas),
		validateLogger(s.LoggerSpec),
	})
}

func validateStorageURI(storageURI *string) error {
	if storageURI == nil {
		return nil
	}

	// local path (not some protocol?)
	if !regexp.MustCompile("\\w+?://").MatchString(*storageURI) {
		return nil
	}

	// one of the prefixes we know?
	for _, prefix := range SupportedStorageURIPrefixList {
		if strings.HasPrefix(*storageURI, prefix) {
			return nil
		}
	}

	azureURIMatcher := regexp.MustCompile(AzureBlobURIRegEx)
	if parts := azureURIMatcher.FindStringSubmatch(*storageURI); parts != nil {
		return nil
	}

	return fmt.Errorf(UnsupportedStorageURIFormatError, strings.Join(SupportedStorageURIPrefixList, ", "), *storageURI)
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
