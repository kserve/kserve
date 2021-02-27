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
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"regexp"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"strings"
)

// regular expressions for validation of isvc name
const (
	CommaSpaceSeparator                 = ", "
	TmNameFmt                    string = "[a-z]([-a-z0-9]*[a-z0-9])?"
	InvalidTmNameFormatError            = "the Trained Model \"%s\" is invalid: a Trained Model name must consist of lower case alphanumeric characters or '-', and must start with alphabetical character. (e.g. \"my-name\" or \"abc-123\", regex used for validation is '%s')"
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

// +kubebuilder:webhook:verbs=create;update,path=/validate-trainedmodel,mutating=false,failurePolicy=fail,groups=serving.kubeflow.org,resources=trainedmodels,versions=v1alpha1,name=trainedmodel.kfserving-webhook-server.validator

var _ webhook.Validator = &TrainedModel{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (tm *TrainedModel) ValidateCreate() error {
	tmLogger.Info("validate create", "name", tm.Name)
	return utils.FirstNonNilError([]error{
		tm.validateTrainedModel(),
	})
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (tm *TrainedModel) ValidateUpdate(old runtime.Object) error {
	tmLogger.Info("validate update", "name", tm.Name)
	oldTm := convertToTrainedModel(old)

	return utils.FirstNonNilError([]error{
		tm.validateTrainedModel(),
		tm.validateMemorySpecNotModified(oldTm),
	})
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (tm *TrainedModel) ValidateDelete() error {
	tmLogger.Info("validate delete", "name", tm.Name)
	return nil
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

// Convert runtime.Object into TrainedModel
func convertToTrainedModel(old runtime.Object) *TrainedModel {
	tm := old.(*TrainedModel)
	return tm
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
	if !v1beta1.IsPrefixStorageURISupported(tm.Spec.Model.StorageURI, storage.GetAllProtocol()) {
		return fmt.Errorf(InvalidStorageUriFormatError, tm.Name, StorageUriProtocols, tm.Spec.Model.StorageURI)
	}
	return nil
}
