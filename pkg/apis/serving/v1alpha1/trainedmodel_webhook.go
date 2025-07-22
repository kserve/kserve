/*
Copyright 2021 The KServe Authors.

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
	"context"
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/kserve/kserve/pkg/utils"
)

// regular expressions for validation of isvc name
const (
	CommaSpaceSeparator                 = ", "
	TmNameFmt                    string = "[a-zA-Z0-9_-]+"
	InvalidTmNameFormatError            = "the Trained Model \"%s\" is invalid: a Trained Model name must consist of alphanumeric characters, '_', or '-'. (e.g. \"my-Name\" or \"abc_123\", regex used for validation is '%s')"
	InvalidStorageUriFormatError        = "the Trained Model \"%s\" storageUri field is invalid. The storage uri must start with one of the prefixes: %s. (the storage uri given is \"%s\")"
	InvalidTmMemoryModification         = "the Trained Model \"%s\" memory field is immutable. The memory was \"%s\" but it is updated to \"%s\""
)

var (
	// log is for logging in this package.
	tmLogger = logf.Log.WithName("trainedmodel-alpha1-validator")
	// regular expressions for validation of tm name
	TmRegexp = regexp.MustCompile("^" + TmNameFmt + "$")
	// protocols that are accepted by storage uri
	StorageUriProtocols = strings.Join(storage.GetAllProtocol(), CommaSpaceSeparator)
)

// +kubebuilder:object:generate=false
// +k8s:openapi-gen=false
// TrainedModelValidator is responsible for setting default values on the TrainedModel resources
// when created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type TrainedModelValidator struct{}

// +kubebuilder:webhook:verbs=create;update,path=/validate-trainedmodel,mutating=false,failurePolicy=fail,groups=serving.kserve.io,resources=trainedmodels,versions=v1alpha1,name=trainedmodel.kserve-webhook-server.validator

var _ webhook.CustomValidator = &TrainedModelValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *TrainedModelValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	tm, err := utils.Convert[*TrainedModel](obj)
	if err != nil {
		tmLogger.Error(err, "Unable to convert object to TrainedModel")
		return nil, err
	}
	tmLogger.Info("validate create", "name", tm.Name)
	return nil, utils.FirstNonNilError([]error{
		tm.validateTrainedModel(),
	})
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *TrainedModelValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newTm, err := utils.Convert[*TrainedModel](newObj)
	if err != nil {
		tmLogger.Error(err, "Unable to convert object to TrainedModel")
		return nil, err
	}
	oldTm, err := utils.Convert[*TrainedModel](oldObj)
	if err != nil {
		tmLogger.Error(err, "Unable to convert object to TrainedModel")
		return nil, err
	}
	tmLogger.Info("validate update", "name", newTm.Name)

	return nil, utils.FirstNonNilError([]error{
		newTm.validateTrainedModel(),
		newTm.validateMemorySpecNotModified(oldTm),
	})
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *TrainedModelValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	tm, err := utils.Convert[*TrainedModel](obj)
	if err != nil {
		tmLogger.Error(err, "Unable to convert object to TrainedModel")
		return nil, err
	}
	tmLogger.Info("validate delete", "name", tm.Name)
	return nil, nil
}

// Validates ModelSpec memory is not modified from previous TrainedModel state
func (tm *TrainedModel) validateMemorySpecNotModified(oldTm *TrainedModel) error {
	newTmMemory := tm.Spec.Model.Memory
	oldTmMemory := oldTm.Spec.Model.Memory
	if !newTmMemory.Equal(oldTmMemory) {
		return fmt.Errorf(InvalidTmMemoryModification, tm.Name, oldTmMemory.String(), newTmMemory.String())
	}
	return nil
}

// Validates format of TrainedModel's fields
func (tm *TrainedModel) validateTrainedModel() error {
	return utils.FirstNonNilError([]error{
		tm.validateTrainedModelName(),
		tm.validateStorageURI(),
	})
}

// Validates format for TrainedModel's name
func (tm *TrainedModel) validateTrainedModelName() error {
	if !TmRegexp.MatchString(tm.Name) {
		return fmt.Errorf(InvalidTmNameFormatError, tm.Name, TmRegexp)
	}
	return nil
}

// Validates TrainModel's storageURI
func (tm *TrainedModel) validateStorageURI() error {
	if !utils.IsPrefixSupported(tm.Spec.Model.StorageURI, storage.GetAllProtocol()) {
		return fmt.Errorf(InvalidStorageUriFormatError, tm.Name, StorageUriProtocols, tm.Spec.Model.StorageURI)
	}
	return nil
}
