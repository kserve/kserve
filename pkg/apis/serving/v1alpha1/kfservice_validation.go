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

	runtime "k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var logger = logf.Log.WithName("kfservice-validation")

// ValidateCreate implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Validator
func (kfsvc *KFService) ValidateCreate() error {
	logger.Info("Validating KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	if err := validateKFService(kfsvc); err != nil {
		logger.Info("Failed to validate KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name, err.Error())
		return err
	}
	logger.Info("Successfully validated KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	return nil
}

// ValidateUpdate implements https://godoc.org/sigs.k8s.io/controller-runtime/pkg/webhook/admission#Validator
func (kfsvc *KFService) ValidateUpdate(old runtime.Object) error {
	return kfsvc.ValidateCreate()
}

func validateKFService(kfsvc *KFService) error {
	if kfsvc == nil {
		return fmt.Errorf("Unable to validate, KFService is nil")
	}
	if err := validateReplicas(kfsvc.Spec.MinReplicas, kfsvc.Spec.MaxReplicas); err != nil {
		return err
	}

	if err := validateDefaultSpec(kfsvc.Spec.Default); err != nil {
		return err
	}

	if err := validateCanarySpec(kfsvc.Spec.Canary); err != nil {
		return err
	}
	return nil
}

func validateReplicas(minReplicas int, maxReplicas int) error {
	if minReplicas < 0 {
		return fmt.Errorf("MinReplicas cannot be less than 0")
	}
	if maxReplicas < 0 {
		return fmt.Errorf("MaxReplicas cannot be less than 0")
	}
	if minReplicas > maxReplicas && maxReplicas != 0 {
		return fmt.Errorf("MinReplicas cannot be greater than MaxReplicas")
	}
	return nil
}

func validateDefaultSpec(defaultSpec ModelSpec) error {
	if err := validateOneModelSpec(defaultSpec); err != nil {
		return err
	}
	return nil
}

func validateCanarySpec(canarySpec *CanarySpec) error {
	if canarySpec == nil {
		return nil
	}
	if err := validateOneModelSpec(canarySpec.ModelSpec); err != nil {
		return err
	}
	if canarySpec.TrafficPercent < 0 || canarySpec.TrafficPercent > 100 {
		return fmt.Errorf("TrafficPercent must be between [0, 100]")
	}
	return nil
}

func validateOneModelSpec(modelSpec ModelSpec) error {
	count := 0
	if modelSpec.Custom != nil {
		count++
	}
	if modelSpec.ScikitLearn != nil {
		count++
	}
	if modelSpec.XGBoost != nil {
		count++
	}
	if modelSpec.Tensorflow != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("Exactly one of [Custom, Tensorflow, ScikitLearn, XGBoost] should be specified in ModelSpec")
	}
	return nil
}
