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

package v1alpha1

import (
	"fmt"

	knserving "github.com/knative/serving/pkg/apis/serving"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// MinReplicasShouldBeLessThanMaxError is an known error message
	MinReplicasShouldBeLessThanMaxError = "MinReplicas cannot be greater than MaxReplicas"
	// MinReplicasLowerBoundExceededError is an known error message
	MinReplicasLowerBoundExceededError = "MinReplicas cannot be less than 0"
	// MaxReplicasLowerBoundExceededError is an known error message
	MaxReplicasLowerBoundExceededError = "MaxReplicas cannot be less than 0"
	// TrafficBoundsExceededError is an known error message
	TrafficBoundsExceededError = "TrafficPercent must be between [0, 100]"
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
		logger.Info("Failed to validate KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name, err.Error())
		return err
	}
	logger.Info("Successfully validated KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	return nil
}

func validateKFService(kfsvc *KFService) error {
	if kfsvc == nil {
		return fmt.Errorf("Unable to validate, KFService is nil")
	}

	if err := validateModelSpec(kfsvc.Spec.Default); err != nil {
		return err
	}

	if err := validateCanarySpec(kfsvc.Spec.Canary); err != nil {
		return err
	}

	return nil
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

func validateModelSpec(spec ModelSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}

	container := spec.CreateModelServingContainer("any")
	knativeErrs := knserving.ValidateContainer(*container, sets.String{})
	if knativeErrs != nil {
		return fmt.Errorf("Custom: " + knativeErrs.Error())
	}
	return nil
}

func validateCanarySpec(canarySpec *CanarySpec) error {
	if canarySpec == nil {
		return nil
	}
	if err := validateModelSpec(canarySpec.ModelSpec); err != nil {
		return err
	}
	if canarySpec.TrafficPercent < 0 || canarySpec.TrafficPercent > 100 {
		return fmt.Errorf(TrafficBoundsExceededError)
	}
	return nil
}
