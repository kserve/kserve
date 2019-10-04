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
func (kfsvc *KFService) ValidateCreate() error {
	return kfsvc.validate()
}

// ValidateUpdate implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Validator
func (kfsvc *KFService) ValidateUpdate(old runtime.Object) error {
	return kfsvc.validate()
}

func (kfsvc *KFService) validate() error {
	logger.Info("Validating KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	if err := validateKFService(kfsvc); err != nil {
		logger.Info("Failed to validate KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name,
			"error", err.Error())
		return err
	}
	logger.Info("Successfully validated KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	return nil
}

func validateKFService(kfsvc *KFService) error {
	if kfsvc == nil {
		return fmt.Errorf("Unable to validate, KFService is nil")
	}
	endpoints := []*EndpointSpec{
		&kfsvc.Spec.Default,
		kfsvc.Spec.Canary,
	}

	for _, endpoint := range endpoints {
		if err := validateEndpoint(endpoint); err != nil {
			return err
		}
	}

	if err := validateCanaryTrafficPercent(kfsvc.Spec); err != nil {
		return err
	}
	return nil
}

func validateEndpoint(endpoint *EndpointSpec) error {
	if endpoint == nil {
		return nil
	}
	if err := endpoint.Predictor.Validate(); err != nil {
		return err
	}
	if endpoint.Transformer != nil {
		if err := endpoint.Transformer.Validate(); err != nil {
			return err
		}
	}
	if endpoint.Explainer != nil {
		if err := endpoint.Explainer.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func validateCanaryTrafficPercent(spec KFServiceSpec) error {
	if spec.Canary == nil && spec.CanaryTrafficPercent != 0 {
		return fmt.Errorf(TrafficProvidedWithoutCanaryError)
	}

	if spec.CanaryTrafficPercent < 0 || spec.CanaryTrafficPercent > 100 {
		return fmt.Errorf(TrafficBoundsExceededError)
	}
	return nil
}
