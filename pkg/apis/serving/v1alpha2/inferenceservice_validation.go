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

	runtime "k8s.io/apimachinery/pkg/runtime"
)

// Known error messages
const (
	MinReplicasShouldBeLessThanMaxError = "MinReplicas cannot be greater than MaxReplicas."
	MinReplicasLowerBoundExceededError  = "MinReplicas cannot be less than 0."
	MaxReplicasLowerBoundExceededError  = "MaxReplicas cannot be less than 0."
	TrafficBoundsExceededError          = "TrafficPercent must be between [0, 100]."
	TrafficProvidedWithoutCanaryError   = "Canary must be specified when CanaryTrafficPercent > 0."
	UnsupportedStorageURIFormatError    = "storageUri, must be one of: [%s] or match https://{}.blob.core.windows.net/{}/{} or be an absolute or relative local path. StorageUri [%s] is not supported."
)

var (
	SupportedStorageURIPrefixList = []string{"gs://", "s3://", "pvc://", "file://"}
	AzureBlobURIRegEx             = "https://(.+?).blob.core.windows.net/(.+)"
)

// ValidateCreate implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Validator
func (isvc *InferenceService) ValidateCreate() error {
	return isvc.validate()
}

// ValidateUpdate implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Validator
func (isvc *InferenceService) ValidateUpdate(old runtime.Object) error {
	return isvc.validate()
}

func (isvc *InferenceService) validate() error {
	logger.Info("Validating InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	if err := validateInferenceService(isvc); err != nil {
		logger.Info("Failed to validate InferenceService", "namespace", isvc.Namespace, "name", isvc.Name,
			"error", err.Error())
		return err
	}
	logger.Info("Successfully validated InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	return nil
}

func validateInferenceService(isvc *InferenceService) error {
	if isvc == nil {
		return fmt.Errorf("Unable to validate, InferenceService is nil")
	}
	if err := validateModelSpec(&isvc.Spec.Default.Predictor); err != nil {
		return err
	}

	if isvc.Spec.Canary != nil {
		if err := validateModelSpec(&isvc.Spec.Canary.Predictor); err != nil {
			return err
		}
	}

	if err := validateCanaryTrafficPercent(isvc.Spec); err != nil {
		return err
	}
	return nil
}

func validateModelSpec(spec *PredictorSpec) error {
	if spec == nil {
		return nil
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	if err := validateStorageURI(spec.GetStorageUri()); err != nil {
		return err
	}
	if err := validateReplicas(spec.MinReplicas, spec.MaxReplicas); err != nil {
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

func validateCanaryTrafficPercent(spec InferenceServiceSpec) error {
	if spec.Canary == nil && spec.CanaryTrafficPercent != 0 {
		return fmt.Errorf(TrafficProvidedWithoutCanaryError)
	}

	if spec.CanaryTrafficPercent < 0 || spec.CanaryTrafficPercent > 100 {
		return fmt.Errorf(TrafficBoundsExceededError)
	}
	return nil
}
