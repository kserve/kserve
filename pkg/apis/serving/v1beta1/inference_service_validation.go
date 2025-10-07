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
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/serving/pkg/apis/autoscaling"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// regular expressions for validation of isvc name
const (
	IsvcNameFmt                         string = "[a-z]([-a-z0-9]*[a-z0-9])?"
	StorageUriPresentInTransformerError string = "storage uri should not be specified in transformer container"
	InvalidStorageUriConfigError        string = "Setting both StorageURI and StorageURIs is not supported."
)

var (
	// logger for the validation webhook.
	validatorLogger = logf.Log.WithName("inferenceservice-v1beta1-validation-webhook")
	// IsvcRegexp regular expressions for validation of isvc name
	IsvcRegexp = regexp.MustCompile("^" + IsvcNameFmt + "$")
)

// +kubebuilder:object:generate=false
// +k8s:openapi-gen=false

// InferenceServiceValidator is responsible for validating the InferenceService resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type InferenceServiceValidator struct{}

// +kubebuilder:webhook:verbs=create;update,path=/validate-inferenceservices,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=inferenceservices,versions=v1beta1,name=inferenceservice.kserve-webhook-server.validator
var _ webhook.CustomValidator = &InferenceServiceValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *InferenceServiceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	isvc, err := utils.Convert[*InferenceService](obj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceService")
		return nil, err
	}
	validatorLogger.Info("validate create", "name", isvc.Name)
	return validateInferenceService(isvc)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *InferenceServiceValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	isvc, err := utils.Convert[*InferenceService](newObj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceService")
		return nil, err
	}
	oldIsvc, err := utils.Convert[*InferenceService](oldObj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceService")
	}
	validatorLogger.Info("validate update", "name", isvc.Name)
	err = validateDeploymentMode(isvc, oldIsvc)
	if err != nil {
		return nil, err
	}
	return validateInferenceService(isvc)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *InferenceServiceValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	isvc, err := utils.Convert[*InferenceService](obj)
	if err != nil {
		validatorLogger.Error(err, "Unable to convert object to InferenceService")
		return nil, err
	}
	validatorLogger.Info("validate delete", "name", isvc.Name)
	return nil, nil
}

func validateInferenceService(isvc *InferenceService) (admission.Warnings, error) {
	var allWarnings admission.Warnings
	annotations := isvc.Annotations

	if err := validateInferenceServiceName(isvc); err != nil {
		return allWarnings, err
	}

	if err := validateInferenceServiceAutoscaler(isvc); err != nil {
		return allWarnings, err
	}

	if err := validateMultiNodeVariables(isvc); err != nil {
		return allWarnings, err
	}

	if err := validateCollocationStorageURI(isvc.Spec.Predictor); err != nil {
		return allWarnings, err
	}

	if err := validatePredictor(isvc); err != nil {
		return allWarnings, err
	}

	if err := validateMultipleStorageURIs(isvc); err != nil {
		return allWarnings, err
	}

	for _, component := range []Component{
		&isvc.Spec.Predictor,
		isvc.Spec.Transformer,
		isvc.Spec.Explainer,
	} {
		if !reflect.ValueOf(component).IsNil() {
			if err := validateExactlyOneImplementation(component); err != nil {
				return allWarnings, err
			}
			if err := utils.FirstNonNilError([]error{
				component.GetImplementation().Validate(),
				component.GetExtensions().Validate(),
				validateAutoScalingCompExtension(annotations, component.GetExtensions()),
			}); err != nil {
				return allWarnings, err
			}
		}
	}
	return allWarnings, nil
}

func validatePredictor(isvc *InferenceService) error {
	predictor := isvc.Spec.Predictor

	// log predictor
	validatorLogger.Info("Incoming predictor struct", "predictor", predictor)

	// in most of the case, standard predictors will all be packed into `predictor.model`, and decide the backend process through `modelFormat.name``
	switch {
	case predictor.SKLearn != nil && predictor.SKLearn.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.XGBoost != nil && predictor.XGBoost.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.Tensorflow != nil && predictor.Tensorflow.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.PyTorch != nil && predictor.PyTorch.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.Triton != nil && predictor.Triton.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.ONNX != nil && predictor.ONNX.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.HuggingFace != nil && predictor.HuggingFace.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.PMML != nil && predictor.PMML.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.LightGBM != nil && predictor.LightGBM.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.Paddle != nil && predictor.Paddle.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	case predictor.Model != nil && predictor.Model.Name != "":
		return errors.New("the 'name' field is not allowed in standard predictor")
	}
	return nil
}

// validateMultiNodeVariables validates when there is workerSpec set in isvc
func validateMultiNodeVariables(isvc *InferenceService) error {
	if isvc.Spec.Predictor.WorkerSpec != nil {
		if len(isvc.Spec.Predictor.WorkerSpec.Containers) > 1 {
			return fmt.Errorf(DisallowedMultipleContainersInWorkerSpecError, isvc.Name)
		}
		if isvc.Spec.Predictor.Model != nil {
			if _, exists := utils.GetEnvVarValue(isvc.Spec.Predictor.Model.PredictorExtensionSpec.Container.Env, constants.PipelineParallelSizeEnvName); exists {
				return fmt.Errorf(DisallowedWorkerSpecPipelineParallelSizeEnvError, isvc.Name)
			}
			if _, exists := utils.GetEnvVarValue(isvc.Spec.Predictor.Model.PredictorExtensionSpec.Container.Env, constants.TensorParallelSizeEnvName); exists {
				return fmt.Errorf(DisallowedWorkerSpecTensorParallelSizeEnvError, isvc.Name)
			}

			hadUnknownGpuType, err := utils.HasUnknownGpuResourceType(isvc.Spec.Predictor.Model.Resources, isvc.Annotations)
			if err != nil {
				return err
			}
			if hadUnknownGpuType {
				return fmt.Errorf(InvalidUnknownGPUTypeError, isvc.Name)
			}

			if isvc.Spec.Predictor.Model.StorageURI == nil {
				return fmt.Errorf(MissingStorageURI, isvc.Name)
			} else {
				storageProtocol := strings.Split(*isvc.Spec.Predictor.Model.StorageURI, "://")[0]
				if storageProtocol != "pvc" && storageProtocol != "oci" {
					return fmt.Errorf(InvalidNotSupportedStorageURIProtocolError, isvc.Name, storageProtocol)
				}
			}
			if isvc.GetAnnotations()[constants.AutoscalerClass] != string(constants.AutoscalerClassNone) {
				return fmt.Errorf(InvalidAutoScalerError, isvc.Name, isvc.GetAnnotations()[constants.AutoscalerClass])
			}
		}

		// WorkerSpec.PipelineParallelSize should not be less than 1
		if pps := isvc.Spec.Predictor.WorkerSpec.PipelineParallelSize; pps != nil && *pps < constants.DefaultPipelineParallelSize {
			return fmt.Errorf(InvalidWorkerSpecPipelineParallelSizeValueError, isvc.Name, strconv.Itoa(*pps))
		}

		// WorkerSpec.TensorParallelSize should not be less than 1.
		if tps := isvc.Spec.Predictor.WorkerSpec.TensorParallelSize; tps != nil && *tps < constants.DefaultTensorParallelSize {
			return fmt.Errorf(InvalidWorkerSpecTensorParallelSizeValueError, isvc.Name, strconv.Itoa(*tps))
		}

		if isvc.Spec.Predictor.WorkerSpec.Containers != nil {
			for _, container := range isvc.Spec.Predictor.WorkerSpec.Containers {
				hadUnknownGpuType, err := utils.HasUnknownGpuResourceType(container.Resources, isvc.Annotations)
				if err != nil {
					return err
				}
				if hadUnknownGpuType {
					return fmt.Errorf(InvalidUnknownGPUTypeError, isvc.Name)
				}
			}
		}
	}
	return nil
}

// Validate scaling options component extensions
func validateAutoScalingCompExtension(annotations map[string]string, compExtSpec *ComponentExtensionSpec) error {
	deploymentMode := annotations["serving.kserve.io/deploymentMode"]
	annotationClass := annotations[autoscaling.ClassAnnotationKey]
	autoscalerClass := annotations[constants.AutoscalerClass]

	switch deploymentMode {
	case string(constants.Standard):
		switch autoscalerClass {
		case string(constants.AutoscalerClassHPA):
			return validateScalingHPACompExtension(compExtSpec)
		case string(constants.AutoscalerClassKeda):
			return validateScalingKedaCompExtension(compExtSpec)
		}
	default:
		if annotationClass == autoscaling.HPA {
			return validateScalingHPACompExtension(compExtSpec)
		}
		return validateScalingKPACompExtension(compExtSpec)
	}
	return nil
}

// Validation of isvc name
func validateInferenceServiceName(isvc *InferenceService) error {
	if !IsvcRegexp.MatchString(isvc.Name) {
		return fmt.Errorf(InvalidISVCNameFormatError, isvc.Name, IsvcNameFmt)
	}
	return nil
}

// Validation of isvc autoscaler class
func validateInferenceServiceAutoscaler(isvc *InferenceService) error {
	annotations := isvc.ObjectMeta.Annotations
	value, ok := annotations[constants.AutoscalerClass]
	class := constants.AutoscalerClassType(value)
	if ok {
		for _, item := range constants.AutoscalerAllowedClassList {
			if class == item {
				return nil
			}
		}
		return fmt.Errorf("[%s] is not a supported autoscaler class type", value)
	}

	return nil
}

// Validation for allowed HPA metrics
func validateHPAMetrics(metric ScaleMetric) error {
	if slices.Contains(constants.AutoscalerAllowedHPAMetricsList, constants.AutoscalerHPAMetricsType(metric)) {
		return nil
	}
	return fmt.Errorf("[%s] is not a supported metric", metric)
}

func validateTargetUtilization(targetValue int32) error {
	if targetValue < 1 || targetValue > 100 {
		return errors.New("the target utilization percentage should be a [1-100] integer")
	}
	return nil
}

func validateScaleTarget(target MetricTarget) error {
	switch target.Type {
	case UtilizationMetricType:
		if target.AverageUtilization == nil {
			return errors.New("the AverageUtilization type should be set")
		}
		return validateTargetUtilization(*target.AverageUtilization)
	case AverageValueMetricType:
		if target.AverageValue == nil {
			return errors.New("the AverageValue type should be set")
		}
	case ValueMetricType:
		if target.Value == nil {
			return errors.New("the Value type should be set")
		}
	}
	return nil
}

func validateScalingHPACompExtension(compExtSpec *ComponentExtensionSpec) error {
	metric := MetricCPU
	if compExtSpec.ScaleMetric != nil {
		metric = *compExtSpec.ScaleMetric
	}

	err := validateHPAMetrics(metric)
	if err != nil {
		return err
	}

	if compExtSpec.ScaleTarget != nil {
		target := *compExtSpec.ScaleTarget
		if metric == MetricCPU && target < 1 || target > 100 {
			return errors.New("the target utilization percentage should be a [1-100] integer")
		}

		if metric == MetricMemory && target < 1 {
			return errors.New("the target memory should be greater than 1 MiB")
		}
	}

	if compExtSpec.AutoScaling != nil {
		for _, metricSpec := range compExtSpec.AutoScaling.Metrics {
			metricType := metricSpec.Type
			switch metricType {
			case ResourceMetricSourceType:
				if metricSpec.Resource == nil {
					return errors.New("metricSpec.Resource is not set for resource metric source type")
				}
			default:
				return fmt.Errorf("invalid HPA metric source type with value [%s],"+
					"valid metric source types are Resource", metricType)
			}
		}
	}

	return nil
}

func validateScalingKedaCompExtension(compExtSpec *ComponentExtensionSpec) error {
	if compExtSpec.ScaleMetric != nil {
		return errors.New("ScaleMetric is not supported for KEDA")
	}

	if compExtSpec.AutoScaling != nil {
		for _, metric := range compExtSpec.AutoScaling.Metrics {
			metricType := metric.Type
			switch metricType {
			case ResourceMetricSourceType:
				if metric.Resource == nil {
					return errors.New("metricSpec.Resource is not set for resource metric source type")
				}
				switch metric.Resource.Name {
				case ResourceMetricCPU:
					if metric.Resource.Target.Type != UtilizationMetricType {
						return errors.New("the cpu target value type should be Utilization")
					}
				case ResourceMetricMemory:
					if metric.Resource.Target.Type != AverageValueMetricType && metric.Resource.Target.Type != UtilizationMetricType {
						return errors.New("the memory target value type should be AverageValue or Utilization")
					}
					if metric.Resource.Target.Type == AverageValueMetricType {
						quantity := metric.Resource.Target.AverageValue.GetQuantity()
						if quantity.Cmp(resource.MustParse("1Mi")) < 0 {
							return errors.New("the memory target value should be greater than 1 MiB")
						}
					}
				default:
					return fmt.Errorf("resource type %s is not supported", metric.Resource.Name)
				}
				if err := validateScaleTarget(metric.Resource.Target); err != nil {
					return err
				}
			case ExternalMetricSourceType:
				if metric.External == nil {
					return errors.New("metricSpec.External is not set for external metric source type")
				}
				if metric.External.Metric.Query == "" {
					return errors.New("the query should not be empty")
				}
				if metric.External.Target.Value == nil {
					return errors.New("the target threshold value should not be empty")
				}
				if err := validateScaleTarget(metric.External.Target); err != nil {
					return err
				}
			case PodMetricSourceType:
				if metric.PodMetric == nil {
					return errors.New("metricSpec.PodMetric is not set for pod metric source type")
				}
				if metric.PodMetric.Metric.Query == "" {
					return errors.New("the query should not be empty")
				}
				if metric.PodMetric.Target.Value == nil {
					return errors.New("the target threshold value should not be empty")
				}
				if err := validateScaleTarget(metric.PodMetric.Target); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown KEDA metric type with value [%s]."+
					"Valid types are Resource,External,PodMetric", metricType)
			}
		}
	}
	return nil
}

func validateKPAMetrics(metric ScaleMetric) error {
	for _, item := range constants.AutoscalerAllowedKPAMetricsList {
		if item == constants.AutoScalerKPAMetricsType(metric) {
			return nil
		}
	}
	return fmt.Errorf("[%s] is not a supported metric", metric)
}

func validateScalingKPACompExtension(compExtSpec *ComponentExtensionSpec) error {
	if compExtSpec.DeploymentStrategy != nil {
		return errors.New("customizing deploymentStrategy is only supported for raw deployment mode")
	}
	metric := MetricConcurrency
	if compExtSpec.ScaleMetric != nil {
		metric = *compExtSpec.ScaleMetric
	}

	err := validateKPAMetrics(metric)
	if err != nil {
		return err
	}

	if compExtSpec.ScaleTarget != nil {
		target := *compExtSpec.ScaleTarget

		if metric == MetricRPS && target < 1 {
			return errors.New("the target for rps should be greater than 1")
		}
	}

	return nil
}

// validates if transformer container has storage uri or not in collocation of predictor and transformer scenario
func validateCollocationStorageURI(predictorSpec PredictorSpec) error {
	for _, container := range predictorSpec.Containers {
		if container.Name == constants.TransformerContainerName {
			for _, env := range container.Env {
				if env.Name == constants.CustomSpecStorageUriEnvVarKey {
					return errors.New(StorageUriPresentInTransformerError)
				}
			}
			break
		}
	}
	return nil
}

// validates if the deploymentMode specified in the annotation is not different from the one recorded in the status
func validateDeploymentMode(newIsvc *InferenceService, oldIsvc *InferenceService) error {
	statusDeploymentMode := oldIsvc.Status.DeploymentMode
	if len(statusDeploymentMode) != 0 {
		annotations := newIsvc.Annotations
		annotationDeploymentMode, ok := annotations[constants.DeploymentMode]
		if ok && annotationDeploymentMode != statusDeploymentMode {
			return fmt.Errorf("update rejected: deploymentMode cannot be changed from '%s' to '%s'", statusDeploymentMode, annotationDeploymentMode)
		}
	}
	return nil
}

// ValidateStorageURISpec validates that paths are absolute
func validateStorageURISpec(storageUri *StorageUri) error {
	// Validate individual storage URI specification
	if storageUri.Uri == "" {
		return errors.New("storage URI cannot be empty")
	}

	if storageUri.MountPath == "/" {
		return errors.New("storage path cannot be empty")
	}

	if !strings.HasPrefix(storageUri.MountPath, "/") {
		return fmt.Errorf("storage path must be absolute: %s", storageUri.MountPath)
	}

	// Security validation: prevent directory traversal attacks
	if strings.Contains(storageUri.MountPath, "..") {
		return fmt.Errorf("storage path cannot contain '..' for security reasons: %s", storageUri.MountPath)
	}

	return nil
}

// ValidateMultipleStorageURISpecs validates a list of storage URI specifications.
// It ensures that:
// - Each individual URI specification is valid (non-empty URI, absolute path)
// - All non-PVC paths share a common parent directory (not root)
// - PVC paths are unique across the list
//
// Parameters:
//   - storageURIs: List of storage URI specifications to validate
//
// Returns:
//   - error: First validation error encountered, or nil if all validations pass
func validateMultipleStorageURIsSpec(storageUris []StorageUri) error {
	paths := make([]string, 0, len(storageUris))
	pvcPaths := make([]string, 0, len(storageUris))

	if len(storageUris) == 0 {
		return nil
	}

	// Validate each individual StorageUrisSpec
	for _, storageUri := range storageUris {
		if err := validateStorageURISpec(&storageUri); err != nil {
			return err
		}
		if strings.HasPrefix(storageUri.Uri, "pvc://") {
			pvcPaths = append(pvcPaths, storageUri.MountPath)
		} else {
			paths = append(paths, storageUri.MountPath)
		}
	}

	// If only one storage URI, no need to check common parent
	if len(paths) <= 1 {
		return nil
	}

	// Check that PVC paths are unique
	if len(pvcPaths) > 1 {
		pvcPathSet := make(map[string]bool)
		for _, path := range pvcPaths {
			if pvcPathSet[path] {
				return errors.New("PVC storage paths must be unique")
			}
			pvcPathSet[path] = true
		}
	}

	// Validate that paths have a common parent path
	commonParent := utils.FindCommonParentPath(paths)
	if commonParent == "/" {
		return fmt.Errorf("storage paths must have a common parent directory. Current paths: %v have no common parent beyond root", paths)
	}

	return nil
}

func validateMultipleStorageURIs(isvc *InferenceService) error {
	if isvc.Spec.Transformer != nil {
		storageURIs := isvc.Spec.Transformer.StorageUris
		var storageURI *string
		if len(isvc.Spec.Transformer.GetImplementations()) > 0 {
			storageURI = isvc.Spec.Transformer.GetImplementation().GetStorageUri()
		}
		if storageURI != nil && storageURIs != nil {
			return errors.New(InvalidStorageUriConfigError)
		}

		if err := validateMultipleStorageURIsSpec(storageURIs); err != nil {
			return err
		}
	}

	if isvc.Spec.Explainer != nil {
		storageURIs := isvc.Spec.Explainer.StorageUris
		var storageURI *string
		if len(isvc.Spec.Explainer.GetImplementations()) > 0 {
			storageURI = isvc.Spec.Explainer.GetImplementation().GetStorageUri()
		}
		if storageURI != nil && storageURIs != nil {
			return errors.New(InvalidStorageUriConfigError)
		}

		if err := validateMultipleStorageURIsSpec(storageURIs); err != nil {
			return err
		}
	}

	storageURIs := isvc.Spec.Predictor.StorageUris
	var storageURI *string
	if len(isvc.Spec.Predictor.GetImplementations()) > 0 {
		storageURI = isvc.Spec.Predictor.GetImplementation().GetStorageUri()
	}

	if storageURI != nil && storageURIs != nil {
		return errors.New(InvalidStorageUriConfigError)
	}

	if err := validateMultipleStorageURIsSpec(storageURIs); err != nil {
		return err
	}

	return nil
}
