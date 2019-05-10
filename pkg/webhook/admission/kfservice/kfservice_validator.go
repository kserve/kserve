package kfservice

import (
	"context"
	"net/http"

	kfservingv1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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
	var logger = logf.Log.WithName("kfservice-validator")
	kfsvc := &kfservingv1alpha1.KFService{}

	if err := validator.Decoder.Decode(req, kfsvc); err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	logger.Info("Validating KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)
	if err := ValidateCreate(kfsvc); err != nil {
		logger.Info("Failed to validate KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name, err.Error())
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}
	logger.Info("Successfully validated KFService", "namespace", kfsvc.Namespace, "name", kfsvc.Name)

	return admission.ValidationResponse(true, "allowed")
}
