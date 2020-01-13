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

package inferenceservice

import (
	"context"
	"fmt"
	"net/http"

	kfserving "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:webhook:path=/validate-inferenceservices,mutating=false,failurePolicy=fail,groups="serving.kubeflow.org",resources=inferenceservices,verbs=create;update,versions=v1alpha2,name=inferenceservice.kfserving-webhook-server.validator

// Validator that validates InferenceServices
type Validator struct {
	Client  client.Client
	Decoder admission.Decoder
}

var _ admission.Handler = &Validator{}

// Handle decodes the incoming InferenceService and executes Validation logic.
func (validator *Validator) Handle(ctx context.Context, req admission.Request) admission.Response {
	isvc := &kfserving.InferenceService{}

	if err := validator.Decoder.Decode(req, isvc); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if constants.IsEnableWebhookNamespaceSelector {
		if err := validator.validateNamespace(isvc, req.AdmissionRequest.Namespace); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	if err := isvc.ValidateCreate(validator.Client); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}

func (validator *Validator) validateNamespace(isvc *kfserving.InferenceService, namespace string) error {
	ns := &v1.Namespace{}
	if err := validator.Client.Get(context.TODO(), ktypes.NamespacedName{Name: namespace}, ns); err != nil {
		return err
	}
	validNS := true
	if ns.Labels == nil {
		validNS = false
	} else {
		if v, ok := ns.Labels[constants.InferenceServicePodLabelKey]; !ok || v != constants.EnableKFServingMutatingWebhook {
			validNS = false
		}
	}
	if !validNS {
		return fmt.Errorf("Cannot create the Inferenceservice %q in namespace %q: the namespace lacks label \"%s: %s\"",
			isvc.Name, namespace, constants.InferenceServicePodLabelKey, constants.EnableKFServingMutatingWebhook)
	} else {
		return nil
	}
}
