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

package kfservice

import (
	"fmt"

	kfservingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
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

func ValidateCreate(kfsvc *kfservingv1alpha1.KFService) error {
	if err := validateKFService(kfsvc); err != nil {
		return err
	}
	return nil
}

func validateKFService(kfsvc *kfservingv1alpha1.KFService) error {
	if kfsvc == nil {
		return fmt.Errorf("Unable to validate, KFService is nil")
	}
	if err := validateReplicas(kfsvc.Spec.MinReplicas, kfsvc.Spec.MaxReplicas); err != nil {
		return err
	}

	if err := kfsvc.Spec.Default.Validate(); err != nil {
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

func validateCanarySpec(canarySpec *kfservingv1alpha1.CanarySpec) error {
	if canarySpec == nil {
		return nil
	}
	if err := canarySpec.ModelSpec.Validate(); err != nil {
		return err
	}
	if canarySpec.TrafficPercent < 0 || canarySpec.TrafficPercent > 100 {
		return fmt.Errorf(TrafficBoundsExceededError)
	}
	return nil
}
