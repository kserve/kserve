package kfservice

import (
	"context"
	"encoding/json"
	"net/http"

	kfservingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/mattbaird/jsonpatch"
	"k8s.io/api/admission/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	admissiontypes "sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// Defaulter that sets default fields in KFServices
type Defaulter struct {
	Client  client.Client
	Decoder admissiontypes.Decoder
}

var _ admission.Handler = &Defaulter{}

// Handle decodes the incoming KFService and executes Validation logic.
func (defaulter *Defaulter) Handle(ctx context.Context, req admissiontypes.Request) admissiontypes.Response {
	kfsvc := &kfservingv1alpha1.KFService{}

	if err := defaulter.Decoder.Decode(req, kfsvc); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	kfsvc.Default()

	patch, err := json.Marshal(kfsvc)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	return patchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
}

// Use the controller-runtime version of this in 1.13. This is temporarily ported from:
// https://github.com/kubernetes-sigs/controller-runtime/blob/58a08d8098290a173ef143bd28820f4308916948/pkg/webhook/admission/response.go#L81
func patchResponseFromRaw(original, current []byte) admissiontypes.Response {
	patches, err := jsonpatch.CreatePatch(original, current)
	if err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	return admissiontypes.Response{
		Patches: patches,
		Response: &v1beta1.AdmissionResponse{
			Allowed:   true,
			PatchType: func() *v1beta1.PatchType { pt := v1beta1.PatchTypeJSONPatch; return &pt }(),
		},
	}
}
