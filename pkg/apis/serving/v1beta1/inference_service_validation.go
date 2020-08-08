/*
Copyright 2020 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"regexp"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strings"
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

var (
	SupportedStorageURIPrefixList = []string{"gs://", "s3://", "pvc://", "file://"}
	AzureBlobURIRegEx             = "https://(.+?).blob.core.windows.net/(.+)"
	// logger for the validation webhook.
	validatorLogger = logf.Log.WithName("inferenceservice-v1beta1-validation-webhook")
)

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateCreate() error {
	validatorLogger.Info("validate create", "name", isvc.Name)

	predictor, err := isvc.Spec.Predictor.GetPredictor()
	if err != nil {
		return err
	}
	if err = predictor.Validate(); err != nil {
		return err
	}

	transformer, err := isvc.Spec.Transformer.GetTransformer()
	if err != nil {
		return err
	}
	if err = transformer.Validate(); err != nil {
		return err
	}

	explainer, err := isvc.Spec.Explainer.GetExplainer()
	if err != nil {
		return err
	}
	if err = explainer.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateUpdate(old runtime.Object) error {
	validatorLogger.Info("validate update", "name", isvc.Name)

	predictor, err := isvc.Spec.Predictor.GetPredictor()
	if err != nil {
		return err
	}
	if err = predictor.Validate(); err != nil {
		return err
	}

	transformer, err := isvc.Spec.Transformer.GetTransformer()
	if err != nil {
		return err
	}
	if err = transformer.Validate(); err != nil {
		return err
	}

	explainer, err := isvc.Spec.Explainer.GetExplainer()
	if err != nil {
		return err
	}
	if err = explainer.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateDelete() error {
	validatorLogger.Info("validate delete", "name", isvc.Name)
	return nil
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

func validateContainerConcurrency(concurrency int) error {
	if concurrency < 0 {
		return fmt.Errorf(ParallelismLowerBoundExceededError)
	}
	return nil
}

func isGPUEnabled(requirements v1.ResourceRequirements) bool {
	_, ok := requirements.Limits[constants.NvidiaGPUResourceType]
	return ok
}

func validateLogger(logger *LoggerSpec) error {
	if logger != nil {
		if !(logger.Mode == LogAll || logger.Mode == LogRequest || logger.Mode == LogResponse) {
			return fmt.Errorf(InvalidLoggerType)
		}
	}
	return nil
}

// GetIntReference returns the pointer for the integer input
func GetIntReference(number int) *int {
	num := number
	return &num
}
