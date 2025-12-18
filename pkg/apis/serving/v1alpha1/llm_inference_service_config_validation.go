/*
Copyright 2025 The KServe Authors.

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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/utils"
)

// +kubebuilder:webhook:path=/validate-serving-kserve-io-v1alpha1-llminferenceserviceconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=llminferenceserviceconfigs,verbs=create;update,versions=v1alpha1,name=llminferenceserviceconfig.kserve-webhook-server.v1alpha1.validator,admissionReviewVersions=v1

// LLMInferenceServiceConfigValidator is responsible for validating the LLMInferenceServiceConfig resource
// when it is created, updated, or deleted.
// +kubebuilder:object:generate=false
type LLMInferenceServiceConfigValidator struct {
	// ConfigValidationFunc is an optional function for additional validation logic.
	// This can be set by the controller to inject validation that depends on controller packages.
	ConfigValidationFunc func(ctx context.Context, config *LLMInferenceServiceConfig) error
}

var _ webhook.CustomValidator = &LLMInferenceServiceConfigValidator{}

func (l *LLMInferenceServiceConfigValidator) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&LLMInferenceServiceConfig{}).
		WithValidator(l).
		Complete()
}

func (l *LLMInferenceServiceConfigValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	warnings := admission.Warnings{}
	config, err := utils.Convert[*LLMInferenceServiceConfig](obj)
	if err != nil {
		return warnings, err
	}

	return warnings, l.validate(ctx, config)
}

func (l *LLMInferenceServiceConfigValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	warnings := admission.Warnings{}
	config, err := utils.Convert[*LLMInferenceServiceConfig](newObj)
	if err != nil {
		return warnings, err
	}

	return warnings, l.validate(ctx, config)
}

func (l *LLMInferenceServiceConfigValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	// No validation needed for deletion
	return admission.Warnings{}, nil
}

func (l *LLMInferenceServiceConfigValidator) validate(ctx context.Context, config *LLMInferenceServiceConfig) error {
	logger := log.FromContext(ctx)
	logger.Info("Validating LLMInferenceServiceConfig v1alpha1", "name", config.Name, "namespace", config.Namespace)

	var allErrs field.ErrorList

	// BaseRefs is not permitted in LLMInferenceServiceConfig
	if len(config.Spec.BaseRefs) > 0 {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec").Child("baseRefs"),
			"baseRefs is not a permitted field in LLMInferenceServiceConfig, support for recursive refs has been disabled",
		))
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			LLMInferenceServiceConfigGVK.GroupKind(),
			config.Name, allErrs)
	}

	// Run additional validation if configured
	if l.ConfigValidationFunc != nil {
		if err := l.ConfigValidationFunc(ctx, config); err != nil {
			return err
		}
	}

	logger.V(2).Info("LLMInferenceServiceConfig v1alpha1 is valid", "config", config)
	return nil
}
