package kfservice

import (
	"context"
	"net/http"

	kfservingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"

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

	patch := kfsvc.DeepCopy()
	patch.Default()
	return admission.PatchResponse(kfsvc, patch)
}
