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
	"context"
	"net/http"

	kfservingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	admissiontypes "sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// Validator that validates KFServices
type Validator struct {
	Client  client.Client
	Decoder admissiontypes.Decoder
}

var _ admission.Handler = &Validator{}

// Handle decodes the incoming KFService and executes Validation logic.
func (validator *Validator) Handle(ctx context.Context, req admissiontypes.Request) admissiontypes.Response {
	kfsvc := &kfservingv1alpha1.KFService{}

	if err := validator.Decoder.Decode(req, kfsvc); err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	if err := kfsvc.ValidateCreate(); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	return admission.ValidationResponse(true, "allowed")
}
