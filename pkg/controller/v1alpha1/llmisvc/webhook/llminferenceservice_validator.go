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

	"k8s.io/utils/ptr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/utils"
)

// +kubebuilder:webhook:path=/validate-serving-kserve-io-v1alpha1-llminferenceservice,mutating=false,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=llminferenceservices,verbs=create;update,versions=v1alpha1,name=llminferenceservice.kserve-webhook-server.validator,admissionReviewVersions=v1;v1beta1

// LLMInferenceServiceValidator is responsible for validating the LLMInferenceService resource
// when it is created, updated, or deleted.
// +kubebuilder:object:generate=false
type LLMInferenceServiceValidator struct{}

var _ webhook.CustomValidator = &LLMInferenceServiceValidator{}

func (l *LLMInferenceServiceValidator) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.LLMInferenceService{}).
		WithValidator(l).
		Complete()
}

func (l *LLMInferenceServiceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	warnings := admission.Warnings{}
	llmSvc, err := utils.Convert[*v1alpha1.LLMInferenceService](obj)
	if err != nil {
		return warnings, err
	}

	return warnings, l.validate(ctx, llmSvc)
}

func (l *LLMInferenceServiceValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	warnings := admission.Warnings{}
	llmSvc, err := utils.Convert[*v1alpha1.LLMInferenceService](newObj)
	if err != nil {
		return warnings, err
	}

	return warnings, l.validate(ctx, llmSvc)
}

func (l *LLMInferenceServiceValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	// No validation needed for deletion
	return admission.Warnings{}, nil
}

func (l *LLMInferenceServiceValidator) validate(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	logger.Info("Validating LLMInferenceService", "name", llmSvc.Name, "namespace", llmSvc.Namespace)

	var allErrs field.ErrorList

	allErrs = append(allErrs, l.validateRouterCrossFieldConstraints(llmSvc)...)
	allErrs = append(allErrs, l.validateParallelismConstraints(llmSvc)...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		v1alpha1.LLMInferenceServiceGVK.GroupKind(),
		llmSvc.Name, allErrs)
}

func (l *LLMInferenceServiceValidator) validateRouterCrossFieldConstraints(llmSvc *v1alpha1.LLMInferenceService) field.ErrorList {
	router := llmSvc.Spec.Router
	if router == nil || router.Route == nil {
		return field.ErrorList{}
	}

	routerPath := field.NewPath("spec").Child("router")
	gatewayPath := routerPath.Child("gateway")
	gwRefsPath := gatewayPath.Child("refs")
	routePath := routerPath.Child("route")
	httpRoutePath := routePath.Child("http")
	httpRouteRefs := httpRoutePath.Child("refs")
	httpRouteSpec := httpRoutePath.Child("spec")

	zero := v1alpha1.GatewayRoutesSpec{}
	if ptr.Deref(router.Route, zero) == zero && router.Gateway != nil && router.Gateway.Refs != nil {
		return field.ErrorList{
			field.Invalid(
				gwRefsPath,
				router.Gateway.Refs,
				fmt.Sprintf("unsupported configuration: custom gateway ('%s') cannot be used with managed route ('%s: {}'); "+
					"either provide your own HTTP routes ('%s') or remove '%s' to use the managed gateway",
					gwRefsPath,
					routePath,
					httpRouteRefs,
					gwRefsPath,
				),
			),
		}
	}

	httpRoute := router.Route.HTTP
	if httpRoute == nil {
		return field.ErrorList{}
	}

	var allErrs field.ErrorList

	// Both refs and spec cannot be used together
	if len(httpRoute.Refs) > 0 && httpRoute.Spec != nil {
		allErrs = append(allErrs, field.Invalid(
			httpRoutePath,
			httpRoute,
			fmt.Sprintf("unsupported configuration: cannot use both custom HTTPRoute refs ('%s') and an inline route spec ('%s'); "+
				"choose one",
				httpRouteRefs, httpRouteSpec,
			),
		),
		)
	}

	// User-defined routes (refs) cannot be used with managed gateway (empty gateway config)
	if len(httpRoute.Refs) > 0 && router.Gateway != nil && len(router.Gateway.Refs) == 0 {
		allErrs = append(allErrs, field.Invalid(
			httpRouteRefs,
			httpRoute.Refs,
			fmt.Sprintf("unsupported configuration: custom HTTP routes ('%s') cannot be used with a managed gateway ('%s'); "+
				"either remove '%s' or set '%s'",
				httpRouteRefs, gatewayPath, httpRouteRefs, gwRefsPath,
			),
		))
	}

	// Managed route spec cannot be used with user-defined gateway refs
	if httpRoute.Spec != nil && router.Gateway != nil && len(router.Gateway.Refs) > 0 {
		allErrs = append(allErrs, field.Invalid(
			httpRoutePath.Child("spec"),
			httpRoute.Spec,
			fmt.Sprintf("unsupported configuration: managed HTTP route spec ('%s') cannot be used with custom gateway refs ('%s'); "+
				"either remove '%s' or '%s'",
				httpRouteSpec, gwRefsPath, gwRefsPath, httpRouteSpec,
			),
		))
	}

	return allErrs
}

func (l *LLMInferenceServiceValidator) validateParallelismConstraints(llmSvc *v1alpha1.LLMInferenceService) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, l.validateWorkloadParallelism(field.NewPath("spec"), &llmSvc.Spec.WorkloadSpec)...)

	if llmSvc.Spec.Prefill != nil {
		allErrs = append(allErrs, l.validateWorkloadParallelism(field.NewPath("spec").Child("prefill"), llmSvc.Spec.Prefill)...)
	}

	return allErrs
}

func (l *LLMInferenceServiceValidator) validateWorkloadParallelism(basePath *field.Path, workload *v1alpha1.WorkloadSpec) field.ErrorList {
	var allErrs field.ErrorList

	if workload.Worker != nil && workload.Parallelism == nil {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("worker"),
			workload.Worker,
			"when worker is specified, parallelism must be configured for either data parallelism or pipeline parallelism",
		))
		return allErrs
	}

	if workload.Parallelism == nil {
		return field.ErrorList{}
	}

	parallelismPath := basePath.Child("parallelism")
	parallelism := workload.Parallelism

	if workload.Worker != nil && !parallelism.IsDataParallel() && !parallelism.IsPipelineParallel() {
		allErrs = append(allErrs, field.Invalid(
			basePath.Child("worker"),
			workload.Worker,
			"when worker is specified, parallelism must be configured for either data parallelism or pipeline parallelism",
		))
	}

	if parallelism.IsPipelineParallel() && parallelism.IsDataParallel() {
		allErrs = append(allErrs, field.Invalid(
			parallelismPath,
			parallelism,
			"cannot set both pipeline parallelism and data parallelism (data or dataLocal) simultaneously",
		))
	}

	// Data and DataLocal must always be set together
	if (parallelism.Data != nil) != (parallelism.DataLocal != nil) {
		if parallelism.Data != nil && parallelism.DataLocal == nil {
			allErrs = append(allErrs, field.Invalid(
				parallelismPath.Child("dataLocal"),
				parallelism.DataLocal,
				"dataLocal must be set when data is set",
			))
		}
		if parallelism.DataLocal != nil && parallelism.Data == nil {
			allErrs = append(allErrs, field.Invalid(
				parallelismPath.Child("data"),
				parallelism.Data,
				"data must be set when dataLocal is set",
			))
		}
	}

	if parallelism.Pipeline != nil && *parallelism.Pipeline <= 0 {
		allErrs = append(allErrs, field.Invalid(
			parallelismPath.Child("pipeline"),
			*parallelism.Pipeline,
			"pipeline parallelism must be greater than 0",
		))
	}

	if parallelism.Data != nil && *parallelism.Data <= 0 {
		allErrs = append(allErrs, field.Invalid(
			parallelismPath.Child("data"),
			*parallelism.Data,
			"data parallelism must be greater than 0",
		))
	}

	if parallelism.DataLocal != nil && *parallelism.DataLocal <= 0 {
		allErrs = append(allErrs, field.Invalid(
			parallelismPath.Child("dataLocal"),
			*parallelism.DataLocal,
			"dataLocal parallelism must be greater than 0",
		))
	}

	return allErrs
}
