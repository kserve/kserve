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

package v1alpha2

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Known error messages
const (
	MinReplicasShouldBeLessThanMaxError = "MinReplicas cannot be greater than MaxReplicas."
	MinReplicasLowerBoundExceededError  = "MinReplicas cannot be less than 0."
	MaxReplicasLowerBoundExceededError  = "MaxReplicas cannot be less than 0."
	ParallelismLowerBoundExceededError  = "Parallelism cannot be less than 0."
	TrafficBoundsExceededError          = "TrafficPercent must be between [0, 100]."
	TrafficProvidedWithoutCanaryError   = "Canary must be specified when CanaryTrafficPercent > 0."
	UnsupportedStorageURIFormatError    = "storageUri, must be one of: [%s] or match https://{}.blob.core.windows.net/{}/{} or be an absolute or relative local path. StorageUri [%s] is not supported."
	InvalidISVCNameFormatError          = "The InferenceService \"%s\" is invalid: a InferenceService name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
)

// Validation for isvc name
const (
	IsvcNameFmt string = "[a-z]([-a-z0-9]*[a-z0-9])?"
)

var (
	SupportedStorageURIPrefixList = []string{"gs://", "s3://", "pvc://", "file://", "https://", "http://"}
	AzureBlobURL                  = "blob.core.windows.net"
	AzureBlobURIRegEx             = "https://(.+?).blob.core.windows.net/(.+)"
	IsvcRegexp                    = regexp.MustCompile("^" + IsvcNameFmt + "$")
)

var _ webhook.Validator = &InferenceService{}

// ValidateCreate implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Validator
func (isvc *InferenceService) ValidateCreate() error {
	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		panic("Failed to create client in validator")
	}
	return isvc.validate(client)
}

// ValidateUpdate implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Validator
func (isvc *InferenceService) ValidateUpdate(old runtime.Object) error {
	client, err := client.New(config.GetConfigOrDie(), client.Options{})
	if err != nil {
		panic("Failed to create client in validator")
	}
	return isvc.validate(client)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateDelete() error {
	return nil
}

func (isvc *InferenceService) validate(client client.Client) error {
	logger.Info("Validating InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	if err := validateInferenceService(isvc, client); err != nil {
		logger.Info("Failed to validate InferenceService", "namespace", isvc.Namespace, "name", isvc.Name,
			"error", err.Error())
		return err
	}
	logger.Info("Successfully validated InferenceService", "namespace", isvc.Namespace, "name", isvc.Name)
	return nil
}

func validateInferenceService(isvc *InferenceService, client client.Client) error {
	if isvc == nil {
		return fmt.Errorf("Unable to validate, InferenceService is nil")
	}

	if err := validateInferenceServiceName(isvc); err != nil {
		return err
	}

	endpoints := []*EndpointSpec{
		&isvc.Spec.Default,
		isvc.Spec.Canary,
	}

	for _, endpoint := range endpoints {
		if err := validateEndpoint(endpoint, client); err != nil {
			return err
		}
	}

	if err := validateCanaryTrafficPercent(isvc.Spec); err != nil {
		return err
	}
	return nil
}

func validateEndpoint(endpoint *EndpointSpec, client client.Client) error {
	if endpoint == nil {
		return nil
	}
	configMap, err := GetInferenceServicesConfig(client)
	if err != nil {
		return err
	}
	// validate predictor
	if err := endpoint.Predictor.Validate(configMap); err != nil {
		return err
	}

	// validate transformer
	if endpoint.Transformer != nil {
		if err := endpoint.Transformer.Validate(configMap); err != nil {
			return err
		}
	}

	// validate explainer
	if endpoint.Explainer != nil {
		if err := endpoint.Explainer.Validate(configMap); err != nil {
			return err
		}
	}
	return nil
}

func validateCanaryTrafficPercent(spec InferenceServiceSpec) error {
	if spec.CanaryTrafficPercent == nil {
		return nil
	}

	if spec.Canary == nil && *spec.CanaryTrafficPercent != 0 {
		return fmt.Errorf(TrafficProvidedWithoutCanaryError)
	}

	if *spec.CanaryTrafficPercent < 0 || *spec.CanaryTrafficPercent > 100 {
		return fmt.Errorf(TrafficBoundsExceededError)
	}
	return fmt.Errorf("Canary rollout is no longer supported on v1alpha2, please convert to v1beta1 to use the feature.")
}

func validateInferenceServiceName(isvc *InferenceService) error {
	if !IsvcRegexp.MatchString(isvc.Name) {
		return fmt.Errorf(InvalidISVCNameFormatError, isvc.Name, IsvcNameFmt)
	}
	return nil
}
