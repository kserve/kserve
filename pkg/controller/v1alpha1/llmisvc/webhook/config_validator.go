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

package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc"
	"github.com/kserve/kserve/pkg/utils"
)

// +kubebuilder:webhook:path=/validate-serving-kserve-io-v1alpha1-llminferenceserviceconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=llminferenceserviceconfigs,verbs=create;update,versions=v1alpha1,name=llminferenceserviceconfigs.kserve-webhook-server.validator,admissionReviewVersions=v1

// LLMInferenceServiceConfigValidator is responsible for validating the LLMInferenceServiceConfig resource
// when it is created, updated, or deleted.
// +kubebuilder:object:generate=false
type LLMInferenceServiceConfigValidator struct {
	ClientSet kubernetes.Interface
}

var _ webhook.CustomValidator = &LLMInferenceServiceConfigValidator{}

func (l *LLMInferenceServiceConfigValidator) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.LLMInferenceServiceConfig{}).
		WithValidator(l).
		Complete()
}

func (l *LLMInferenceServiceConfigValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	warnings := admission.Warnings{}
	llmSvcConfig, err := utils.Convert[*v1alpha1.LLMInferenceServiceConfig](obj)
	if err != nil {
		return warnings, err
	}

	return warnings, l.validate(ctx, llmSvcConfig)
}

func (l *LLMInferenceServiceConfigValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	logger := log.FromContext(ctx)
	oldConfig, errOld := utils.Convert[*v1alpha1.LLMInferenceServiceConfig](oldObj)
	if errOld != nil {
		return admission.Warnings{}, errOld
	}
	newConfig, errNew := utils.Convert[*v1alpha1.LLMInferenceServiceConfig](newObj)
	if errNew != nil {
		return admission.Warnings{}, errNew
	}

	warnings := admission.Warnings{}
	if llmisvc.WellKnownDefaultConfigs.Has(oldConfig.Name) && !equality.Semantic.DeepDerivative(oldConfig.Spec, newConfig.Spec) {
		warning := fmt.Sprintf("modifying well-known config %s/%s is not recommended. Consider creating a custom config instead", oldConfig.Namespace, oldConfig.Name)
		logger.Info(warning)
		warnings = append(warnings, warning)
	}

	return warnings, l.validate(ctx, newConfig)
}

func (l *LLMInferenceServiceConfigValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logger := log.FromContext(ctx)
	config, err := utils.Convert[*v1alpha1.LLMInferenceServiceConfig](obj)
	if err != nil {
		return admission.Warnings{}, err
	}

	warnings := admission.Warnings{}
	if llmisvc.WellKnownDefaultConfigs.Has(config.Name) {
		warning := fmt.Sprintf("deleting well-known config %s/%s is not recommended", config.Namespace, config.Name)
		logger.Info(warning)
		warnings = append(warnings, warning)
	}

	return warnings, nil
}

func (l *LLMInferenceServiceConfigValidator) validate(ctx context.Context, llmSvcConfig *v1alpha1.LLMInferenceServiceConfig) error {
	logger := log.FromContext(ctx)
	llmSvcConfig = llmSvcConfig.DeepCopy()

	config, err := llmisvc.LoadConfig(ctx, l.ClientSet)
	if err != nil {
		logger.Error(err, "failed to load config")
		return err
	}

	_, err = llmisvc.ReplaceVariables(llmisvc.LLMInferenceServiceSample(), llmSvcConfig, config)
	if err != nil {
		logger.Error(err, "failed to process the template")
	}

	return err
}
