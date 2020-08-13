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
	"strings"

	"github.com/kubeflow/kfserving/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	// logger for the validation webhook.
	validatorLogger = logf.Log.WithName("inferenceservice-v1beta1-validation-webhook")
)

// +kubebuilder:webhook:verbs=create;update,path=/validate-inferenceservices,mutating=false,failurePolicy=fail,groups=serving.kubeflow.org,resources=inferenceservices,versions=v1beta1,name=inferenceservice.kfserving-webhook-server.validator
var _ webhook.Validator = &InferenceService{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (isvc *InferenceService) ValidateCreate() error {
	validatorLogger.Info("validate create", "name", isvc.Name)
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

func validateExactlyOneImplementation(component Component) error {
	implementations := NonNilComponents(component.GetImplementations())
	count := len(implementations)
	if count == 2 { // If two implementations, allow if one of them is custom overrides
		for _, implementation := range implementations {
			switch reflect.ValueOf(implementation).Type().Elem().Name() {
			case
				reflect.ValueOf(CustomPredictor{}).Type().Name(),
				reflect.ValueOf(CustomExplainer{}).Type().Name(),
				reflect.ValueOf(CustomTransformer{}).Type().Name():
				return nil
			}
		}
	} else if count == 1 {
		return nil
	}
	return ExactlyOneErrorFor(component)
}

// ExactlyOneErrorFor creates an error for the component's one-of semantic.
func ExactlyOneErrorFor(component Component) error {
	componentType := reflect.ValueOf(component).Type().Elem()
	implementationTypes := []string{}
	for i := 0; i < componentType.NumField()-1; i++ {
		implementationTypes = append(implementationTypes, componentType.Field(i).Type.Elem().Name())
	}
	return fmt.Errorf(
		"Exactly one of [%s] must be specified in %s",
		strings.Join(implementationTypes, ", "),
		componentType.Name(),
	)
}
