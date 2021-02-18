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

package v1alpha1

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"regexp"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// regular expressions for validation of isvc name
const (
	TmNameFmt                string = "[a-z]([-a-z0-9]*[a-z0-9])?"
	InvalidTmNameFormatError        = "the Trained Model \"%s\" is invalid: a Trained Model name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
)

var (
	// log is for logging in this package.
	trainedmodellog = logf.Log.WithName("trainedmodel-alpha1-resource")
	// regular expressions for validation of isvc name
	TmRegexp = regexp.MustCompile("^" + TmNameFmt + "$")
)

// +kubebuilder:webhook:verbs=create;update,path=/validate-trainedmodel,mutating=false,failurePolicy=fail,groups=serving.kubeflow.org,resources=trainedmodels,versions=v1alpha1,name=trainedmodel.kfserving-webhook-server.validator

var _ webhook.Validator = &TrainedModel{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (tm *TrainedModel) ValidateCreate() error {
	trainedmodellog.Info("validate create", "name", tm.Name)
	// TODO: Validate storageURI
	return tm.validateTrainedModel()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (tm *TrainedModel) ValidateUpdate(old runtime.Object) error {
	trainedmodellog.Info("validate update", "name", tm.Name)

	oldTm := convertToTrainedModel(old)
	if !tm.Spec.Model.Memory.Equal(oldTm.Spec.Model.Memory) {
		return fmt.Errorf("failed to update trained model \"%s\" in validator because memory was changed", tm.Name)
	}

	return tm.validateTrainedModel()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (tm *TrainedModel) ValidateDelete() error {
	trainedmodellog.Info("validate delete", "name", tm.Name)
	return nil
}

func (tm *TrainedModel) validateTrainedModel() error {
	return tm.validateTrainedModelName()
}

// Convert runtime.Object into TrainedModel
func convertToTrainedModel(old runtime.Object) *TrainedModel {
	tm := old.(*TrainedModel)
	return tm
}

func (tm *TrainedModel) validateTrainedModelName() error {
	if !TmRegexp.MatchString(tm.Name) {
		return fmt.Errorf(InvalidTmNameFormatError, tm.Name, TmRegexp)
	}
	return nil
}
