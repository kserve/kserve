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
	"reflect"

	"regexp"

	"github.com/kubeflow/kfserving/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// regular expressions for validation of isvc name
const (
	IsvcNameFmt string = "[a-z]([-a-z0-9]*[a-z0-9])?"
)

var (
	// logger for the validation webhook.
	validatorLogger = logf.Log.WithName("inferenceservice-v1beta1-validation-webhook")
	// regular expressions for validation of isvc name
	IsvcRegexp = regexp.MustCompile("^" + IsvcNameFmt + "$")
)

// +kubebuilder:webhook:verbs=create;update,path=/validate-inferenceservices,mutating=false,failurePolicy=fail,groups=serving.kubeflow.org,resources=inferenceservices,versions=v1beta1,name=inferenceservice.kfserving-webhook-server.validator
var _ webhook.Validator = &InferenceService{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateCreate() error {
	validatorLogger.Info("validate create", "name", isvc.Name)

	if err := validateInferenceServiceName(isvc); err != nil {
		return err
	}

	for _, component := range []Component{
		&isvc.Spec.Predictor,
		isvc.Spec.Transformer,
		isvc.Spec.Explainer,
	} {
		if !reflect.ValueOf(component).IsNil() {
			if err := validateExactlyOneImplementation(component); err != nil {
				return err
			}
			if err := utils.FirstNonNilError([]error{
				component.GetImplementation().Validate(),
				component.GetExtensions().Validate(),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateUpdate(old runtime.Object) error {
	validatorLogger.Info("validate update", "name", isvc.Name)

	return isvc.ValidateCreate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateDelete() error {
	validatorLogger.Info("validate delete", "name", isvc.Name)
	return nil
}

// GetIntReference returns the pointer for the integer input
func GetIntReference(number int) *int {
	num := number
	return &num
}

// Validation of isvc name
func validateInferenceServiceName(isvc *InferenceService) error {
	if !IsvcRegexp.MatchString(isvc.Name) {
		return fmt.Errorf(InvalidISVCNameFormatError, isvc.Name, IsvcNameFmt)
	}
	return nil
}
