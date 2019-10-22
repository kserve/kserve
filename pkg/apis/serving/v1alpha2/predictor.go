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

package v1alpha2

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

type Predictor interface {
	GetStorageUri() string
	GetResourceRequirement() *v1.ResourceRequirements
	GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container
	ApplyDefaults(config *InferenceServicesConfig)
	Validate(config *InferenceServicesConfig) error
}

const (
	// ExactlyOnePredictorViolatedError is a known error message
	ExactlyOnePredictorViolatedError = "Exactly one of [Custom, ONNX, Tensorflow, TensorRT, SKLearn, XGBoost] must be specified in PredictorSpec"
)

var (
	DefaultMemory = resource.MustParse("2Gi")
	DefaultCPU    = resource.MustParse("1")
)

// Returns a URI to the model. This URI is passed to the storage-initializer via the StorageInitializerSourceUriInternalAnnotationKey
func (p *PredictorSpec) GetStorageUri() string {
	predictor, err := getPredictor(p)
	if err != nil {
		return ""
	}
	return predictor.GetStorageUri()
}

// Returns the ResourceRequirements of the model.
func (p *PredictorSpec) GetResourceRequirement() *v1.ResourceRequirements {
	predictor, err := getPredictor(p)
	if err != nil {
		return nil
	}
	return predictor.GetResourceRequirement()
}

func (p *PredictorSpec) GetContainer(modelName string, config *InferenceServicesConfig) *v1.Container {
	predictor, err := getPredictor(p)
	if err != nil {
		return nil
	}
	return predictor.GetContainer(modelName, config)
}

func (p *PredictorSpec) ApplyDefaults(config *InferenceServicesConfig) {
	predictor, err := getPredictor(p)
	if err == nil {
		predictor.ApplyDefaults(config)
	}
}

func (p *PredictorSpec) Validate(config *InferenceServicesConfig) error {
	predictor, err := getPredictor(p)
	if err != nil {
		return err
	}
	if err := predictor.Validate(config); err != nil {
		return err
	}
	if err := validateStorageURI(p.GetStorageUri()); err != nil {
		return err
	}
	if err := validateReplicas(p.MinReplicas, p.MaxReplicas); err != nil {
		return err
	}
	if err := validateResourceRequirement(p.GetResourceRequirement()); err != nil {
		return err
	}

	return nil
}

func validateStorageURI(storageURI string) error {
	if storageURI == "" {
		return nil
	}

	// local path (not some protocol?)
	if !regexp.MustCompile("\\w+?://").MatchString(storageURI) {
		return nil
	}

	// one of the prefixes we know?
	for _, prefix := range SupportedStorageURIPrefixList {
		if strings.HasPrefix(storageURI, prefix) {
			return nil
		}
	}

	azureURIMatcher := regexp.MustCompile(AzureBlobURIRegEx)
	if parts := azureURIMatcher.FindStringSubmatch(storageURI); parts != nil {
		return nil
	}

	return fmt.Errorf(UnsupportedStorageURIFormatError, strings.Join(SupportedStorageURIPrefixList, ", "), storageURI)
}

func validateReplicas(minReplicas int, maxReplicas int) error {
	if minReplicas < 0 {
		return fmt.Errorf(MinReplicasLowerBoundExceededError)
	}
	if maxReplicas < 0 {
		return fmt.Errorf(MaxReplicasLowerBoundExceededError)
	}
	if minReplicas > maxReplicas && maxReplicas != 0 {
		return fmt.Errorf(MinReplicasShouldBeLessThanMaxError)
	}
	return nil
}

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}

	if _, ok := requirements.Requests[v1.ResourceCPU]; !ok {
		requirements.Requests[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Requests[v1.ResourceMemory]; !ok {
		requirements.Requests[v1.ResourceMemory] = DefaultMemory
	}

	if requirements.Limits == nil {
		requirements.Limits = v1.ResourceList{}
	}

	if _, ok := requirements.Limits[v1.ResourceCPU]; !ok {
		requirements.Limits[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Limits[v1.ResourceMemory]; !ok {
		requirements.Limits[v1.ResourceMemory] = DefaultMemory
	}
}

func validateResourceRequirement(requirements *v1.ResourceRequirements) error {
	if requirements == nil {
		return nil
	}

	if cpuRequest, ok := requirements.Requests[v1.ResourceCPU]; ok {
		if cpuRequest.Sign() < 0 {
			return fmt.Errorf(CPURequestLowerBoundExceededError)
		}
		if cpuLimit, ok := requirements.Limits[v1.ResourceCPU]; ok {
			if cpuLimit.Sign() < 0 {
				return fmt.Errorf(CPULimitLowerBoundExceededError)
			}
			if cpuRequest.Cmp(cpuLimit) > 0 && !cpuLimit.IsZero() {
				return fmt.Errorf(CPURequestShouldBeLessThanLimitError)
			}
		}
	}

	if memoryRequest, ok := requirements.Requests[v1.ResourceMemory]; ok {
		if memoryRequest.Sign() < 0 {
			return fmt.Errorf(MemoryRequestLowerBoundExceededError)
		}
		if memoryLimit, ok := requirements.Limits[v1.ResourceMemory]; ok {
			if memoryLimit.Sign() < 0 {
				return fmt.Errorf(MemoryLimitLowerBoundExceededError)
			}
			if memoryRequest.Cmp(memoryLimit) > 0 && !memoryLimit.IsZero() {
				return fmt.Errorf(MemoryRequestShouldBeLessThanLimitError)
			}
		}
	}

	return nil
}

func isGPUEnabled(requirements v1.ResourceRequirements) bool {
	_, ok := requirements.Limits[constants.NvidiaGPUResourceType]
	return ok
}

func getPredictor(predictorSpec *PredictorSpec) (Predictor, error) {
	predictors := []Predictor{}
	if predictorSpec.Custom != nil {
		predictors = append(predictors, predictorSpec.Custom)
	}
	if predictorSpec.XGBoost != nil {
		predictors = append(predictors, predictorSpec.XGBoost)
	}
	if predictorSpec.SKLearn != nil {
		predictors = append(predictors, predictorSpec.SKLearn)
	}
	if predictorSpec.Tensorflow != nil {
		predictors = append(predictors, predictorSpec.Tensorflow)
	}
	if predictorSpec.ONNX != nil {
		predictors = append(predictors, predictorSpec.ONNX)
	}
	if predictorSpec.PyTorch != nil {
		predictors = append(predictors, predictorSpec.PyTorch)
	}
	if predictorSpec.TensorRT != nil {
		predictors = append(predictors, predictorSpec.TensorRT)
	}
	if len(predictors) != 1 {
		err := fmt.Errorf(ExactlyOnePredictorViolatedError)
		klog.Error(err)
		return nil, err
	}
	return predictors[0], nil
}
