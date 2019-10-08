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
	"encoding/json"
	"net/http"

	kfserving "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/webhook/third_party"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	admissiontypes "sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// Defaulter that sets default fields in InferenceServices
type Defaulter struct {
	Client  client.Client
	Decoder admissiontypes.Decoder
}

var _ admission.Handler = &Defaulter{}

// Handle decodes the incoming InferenceService and executes Validation logic.
func (defaulter *Defaulter) Handle(ctx context.Context, req admissiontypes.Request) admissiontypes.Response {
	isvc := &kfserving.InferenceService{}

	if err := defaulter.Decoder.Decode(req, isvc); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	isvc.Default()

	patch, err := json.Marshal(isvc)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	return third_party.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
}
