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

package v1alpha2

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"

	"k8s.io/utils/ptr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/utils"
)

// variantCostPattern is compiled once at package init to avoid recompilation on every webhook call.
var variantCostPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

// +kubebuilder:webhook:path=/validate-serving-kserve-io-v1alpha2-llminferenceservice,mutating=false,failurePolicy=fail,sideEffects=None,groups=serving.kserve.io,resources=llminferenceservices,verbs=create;update,versions=v1alpha2,name=llminferenceservice.kserve-webhook-server.v1alpha2.validator,admissionReviewVersions=v1

// LLMInferenceServiceValidator is responsible for validating the LLMInferenceService resource
// when it is created, updated, or deleted.
// +kubebuilder:object:generate=false
type LLMInferenceServiceValidator struct{}

var _ webhook.CustomValidator = &LLMInferenceServiceValidator{}

func (l *LLMInferenceServiceValidator) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&LLMInferenceService{}).
		WithValidator(l).
		Complete()
}

func (l *LLMInferenceServiceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	llmSvc, err := utils.Convert[*LLMInferenceService](obj)
	if err != nil {
		return nil, err
	}

	return l.validate(ctx, nil, llmSvc)
}

func (l *LLMInferenceServiceValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	llmSvc, err := utils.Convert[*LLMInferenceService](newObj)
	if err != nil {
		return nil, err
	}
	prev, err := utils.Convert[*LLMInferenceService](oldObj)
	if err != nil {
		return nil, err
	}

	return l.validate(ctx, prev, llmSvc)
}

func (l *LLMInferenceServiceValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	// No validation needed for deletion
	return admission.Warnings{}, nil
}

func (l *LLMInferenceServiceValidator) validate(ctx context.Context, prev *LLMInferenceService, llmSvc *LLMInferenceService) (admission.Warnings, error) {
	logger := log.FromContext(ctx)
	logger.Info("Validating LLMInferenceService v1alpha2", "name", llmSvc.Name, "namespace", llmSvc.Namespace)

	var allErrs field.ErrorList
	var warnings admission.Warnings

	routerWarnings, routerErrs := l.validateRouterCrossFieldConstraints(llmSvc)
	warnings = append(warnings, routerWarnings...)
	allErrs = append(allErrs, routerErrs...)
	allErrs = append(allErrs, l.validateParallelismConstraints(llmSvc)...)
	allErrs = append(allErrs, l.validateSchedulerConfig(llmSvc)...)

	allErrs = append(allErrs, l.validateScaling(llmSvc)...)

	allErrs = append(allErrs, l.validateImmutable(prev, llmSvc)...)

	if len(allErrs) == 0 {
		logger.V(2).Info("LLMInferenceService v1alpha2 is valid", "llmisvc", llmSvc)
		return warnings, nil
	}

	return warnings, apierrors.NewInvalid(
		LLMInferenceServiceGVK.GroupKind(),
		llmSvc.Name, allErrs)
}

func (l *LLMInferenceServiceValidator) validateRouterCrossFieldConstraints(llmSvc *LLMInferenceService) (admission.Warnings, field.ErrorList) {
	router := llmSvc.Spec.Router
	if router == nil || router.Route == nil {
		return nil, nil
	}

	routerPath := field.NewPath("spec").Child("router")
	gatewayPath := routerPath.Child("gateway")
	gwRefsPath := gatewayPath.Child("refs")
	routePath := routerPath.Child("route")
	httpRoutePath := routePath.Child("http")
	httpRouteRefs := httpRoutePath.Child("refs")
	httpRouteSpec := httpRoutePath.Child("spec")

	httpRoute := router.Route.HTTP
	if httpRoute == nil {
		return nil, nil
	}

	var allErrs field.ErrorList
	var warnings admission.Warnings

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

	// When both parentRefs and gateway refs are set, check for consistency.
	// If they match, it's redundant but valid — the controller derives parentRefs from gateway.refs automatically.
	// If they conflict, reject.
	if httpRoute.Spec != nil && len(httpRoute.Spec.ParentRefs) > 0 &&
		router.Gateway != nil && len(router.Gateway.Refs) > 0 {
		if parentRefsMatchGatewayRefs(httpRoute.Spec.ParentRefs, router.Gateway.Refs) {
			warnings = append(warnings,
				fmt.Sprintf("%s.parentRefs can be omitted when %s is set; parentRefs will be derived automatically from gateway refs",
					httpRouteSpec, gwRefsPath))
		} else {
			allErrs = append(allErrs, field.Invalid(
				httpRoutePath.Child("spec"),
				httpRoute.Spec,
				fmt.Sprintf("unsupported configuration: managed HTTP route spec ('%s') has parentRefs that conflict with custom gateway refs ('%s'); "+
					"either remove '%s' or '%s.parentRefs'",
					httpRouteSpec, gwRefsPath, gwRefsPath, httpRouteSpec,
				),
			))
		}
	}

	return warnings, allErrs
}

// parentRefsMatchGatewayRefs checks whether the parentRefs on an HTTPRoute are
// consistent with the gateway refs. A parentRef matches a gateway ref if the
// name and namespace are equal (Group and Kind are always Gateway).
// Both slices are sorted before comparison so order does not matter.
func parentRefsMatchGatewayRefs(parentRefs []gwapiv1.ParentReference, gatewayRefs []UntypedObjectReference) bool {
	if len(parentRefs) != len(gatewayRefs) {
		return false
	}

	// Build comparable keys and sort so that order-independent matching works.
	type key struct{ ns, name string }
	toKey := func(name gwapiv1.ObjectName, ns gwapiv1.Namespace) key {
		return key{ns: string(ns), name: string(name)}
	}
	cmpKey := func(a, b key) int {
		if c := cmp.Compare(a.ns, b.ns); c != 0 {
			return c
		}
		return cmp.Compare(a.name, b.name)
	}

	pKeys := make([]key, len(parentRefs))
	for i, ref := range parentRefs {
		pKeys[i] = toKey(ref.Name, ptr.Deref(ref.Namespace, ""))
	}
	gKeys := make([]key, len(gatewayRefs))
	for i, ref := range gatewayRefs {
		gKeys[i] = toKey(ref.Name, ref.Namespace)
	}

	slices.SortFunc(pKeys, cmpKey)
	slices.SortFunc(gKeys, cmpKey)

	return slices.Equal(pKeys, gKeys)
}

func (l *LLMInferenceServiceValidator) validateParallelismConstraints(llmSvc *LLMInferenceService) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, l.validateWorkloadParallelism(field.NewPath("spec"), &llmSvc.Spec.WorkloadSpec)...)

	if llmSvc.Spec.Prefill != nil {
		allErrs = append(allErrs, l.validateWorkloadParallelism(field.NewPath("spec").Child("prefill"), llmSvc.Spec.Prefill)...)
	}

	return allErrs
}

func (l *LLMInferenceServiceValidator) validateWorkloadParallelism(basePath *field.Path, workload *WorkloadSpec) field.ErrorList {
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

func (l *LLMInferenceServiceValidator) validateImmutable(prev *LLMInferenceService, curr *LLMInferenceService) field.ErrorList {
	var allErrs field.ErrorList
	if prev == nil {
		return allErrs
	}

	specPath := field.NewPath("spec")

	allErrs = append(allErrs, l.validateImmutableParallelism(specPath, prev.Spec.Parallelism, curr.Spec.Parallelism)...)
	if curr.Spec.Prefill != nil && prev.Spec.Prefill != nil {
		allErrs = append(allErrs, l.validateImmutableParallelism(specPath.Child("prefill"), prev.Spec.Prefill.Parallelism, curr.Spec.Prefill.Parallelism)...)
	}

	return allErrs
}

func (l *LLMInferenceServiceValidator) validateImmutableParallelism(basePath *field.Path, prev *ParallelismSpec, curr *ParallelismSpec) field.ErrorList {
	var allErrs field.ErrorList
	if pSize, cSize := ptr.Deref(prev.GetSize(), 1), ptr.Deref(curr.GetSize(), 1); cSize != pSize {
		allErrs = append(allErrs, immutableField(
			basePath.Child("parallelism"),
			cSize,
			fmt.Sprintf("total parallelism size is immutable, previous size %d, curr size %d", pSize, cSize),
		))
	}
	return allErrs
}

func (l *LLMInferenceServiceValidator) validateSchedulerConfig(svc *LLMInferenceService) field.ErrorList {
	var allErrs field.ErrorList

	if svc.Spec.Router == nil || svc.Spec.Router.Scheduler == nil {
		return allErrs
	}

	schedulerPath := field.NewPath("spec", "router", "scheduler")

	if svc.Spec.Router.Scheduler.Replicas != nil && *svc.Spec.Router.Scheduler.Replicas <= 0 {
		allErrs = append(allErrs, field.Invalid(schedulerPath.Child("replicas"), *svc.Spec.Router.Scheduler.Replicas, "scheduler replicas must be greater than zero"))
	}

	if svc.Spec.Router.Scheduler.Config == nil {
		return allErrs
	}

	configPath := schedulerPath.Child("config")

	if svc.Spec.Router.Scheduler.Config.Ref == nil && svc.Spec.Router.Scheduler.Config.Inline == nil {
		allErrs = append(allErrs, field.Invalid(
			configPath,
			svc.Spec.Router.Scheduler.Config,
			"either inline or ref is required",
		))
	}

	if svc.Spec.Router.Scheduler.Config.Inline != nil && svc.Spec.Router.Scheduler.Config.Ref != nil {
		allErrs = append(allErrs, field.Invalid(
			configPath,
			svc.Spec.Router.Scheduler.Config,
			"both inline and ref are set, either specify inline or ref",
		))
	}

	if svc.Spec.Router.Scheduler.Config.Inline != nil && len(svc.Spec.Router.Scheduler.Config.Inline.Raw) < 3 /* we expect at least a few characters '{...}' */ {
		allErrs = append(allErrs, field.Invalid(
			configPath.Child("inline"),
			svc.Spec.Router.Scheduler.Config.Inline,
			"inline configuration is invalid",
		))
	}

	if svc.Spec.Router.Scheduler.Config.Ref != nil {
		if svc.Spec.Router.Scheduler.Config.Ref.Name == "" {
			allErrs = append(allErrs, field.Invalid(
				configPath.Child("ref", "name"),
				svc.Spec.Router.Scheduler.Config.Ref,
				"name is empty",
			))
		}
	}

	return allErrs
}

func (l *LLMInferenceServiceValidator) validateScaling(llmSvc *LLMInferenceService) field.ErrorList {
	var allErrs field.ErrorList

	// Validate scaling on the main (decode) workload
	allErrs = append(allErrs, ValidateWorkloadScaling(field.NewPath("spec"), &llmSvc.Spec.WorkloadSpec)...)

	// Validate scaling on the prefill workload, if present
	if llmSvc.Spec.Prefill != nil {
		allErrs = append(allErrs, ValidateWorkloadScaling(field.NewPath("spec").Child("prefill"), llmSvc.Spec.Prefill)...)
	}

	allErrs = append(allErrs, l.validateActuatorConsistency(llmSvc)...)

	return allErrs
}

func (l *LLMInferenceServiceValidator) validateActuatorConsistency(llmSvc *LLMInferenceService) field.ErrorList {
	return ValidateActuatorConsistency(&llmSvc.Spec.WorkloadSpec, llmSvc.Spec.Prefill)
}

// ValidateActuatorConsistency ensures that when both decode and prefill workloads
// have autoscaling configured, they use the same actuator backend (both HPA or both KEDA).
// Mixing backends is not supported because:
//   - HPA requires a Prometheus Adapter to expose metrics to the Kubernetes Metrics API
//   - KEDA queries Prometheus directly without an adapter
//
// Using different backends forces operators to maintain two different metric pipelines
// and results in independent, unsynchronised scaling decisions across the two sides
// of a disaggregated deployment.
//
// It is exported so that v1alpha1 can reuse it via conversion.
func ValidateActuatorConsistency(decode *WorkloadSpec, prefill *WorkloadSpec) field.ErrorList {
	if prefill == nil {
		return nil
	}

	// Both sides must have scaling.wva configured for a mismatch to be possible.
	decodeScaling := decode.Scaling
	prefillScaling := prefill.Scaling
	if decodeScaling == nil || decodeScaling.WVA == nil || prefillScaling == nil || prefillScaling.WVA == nil {
		return nil
	}

	decodeUsesHPA := decodeScaling.WVA.HPA != nil
	prefillUsesHPA := prefillScaling.WVA.HPA != nil

	if decodeUsesHPA == prefillUsesHPA {
		return nil
	}

	decodeBackend := "keda"
	prefillBackend := "hpa"
	if decodeUsesHPA {
		decodeBackend = "hpa"
		prefillBackend = "keda"
	}

	return field.ErrorList{
		field.Invalid(
			field.NewPath("spec").Child("prefill", "scaling", "wva"),
			prefillScaling.WVA,
			fmt.Sprintf(
				"decode and prefill must use the same actuator backend; "+
					"decode uses %s but prefill uses %s — "+
					"mixing backends requires two separate metric pipelines and leads to independent, unsynchronised scaling decisions",
				decodeBackend, prefillBackend,
			),
		),
	}
}

// ValidateWorkloadScaling validates the scaling configuration of a single workload
// (decode or prefill). It is exported so that v1alpha1 can reuse it via conversion.
func ValidateWorkloadScaling(basePath *field.Path, workload *WorkloadSpec) field.ErrorList {
	var allErrs field.ErrorList

	scaling := workload.Scaling
	if scaling == nil {
		return allErrs
	}

	scalingPath := basePath.Child("scaling")

	// Replicas and scaling are mutually exclusive
	if workload.Replicas != nil {
		allErrs = append(allErrs, field.Invalid(
			scalingPath,
			scaling,
			"scaling and replicas are mutually exclusive; use scaling for autoscaled deployments or replicas for static deployments",
		))
	}

	// Validate replica bounds
	if scaling.MinReplicas != nil && *scaling.MinReplicas > scaling.MaxReplicas {
		allErrs = append(allErrs, field.Invalid(
			scalingPath.Child("minReplicas"),
			*scaling.MinReplicas,
			fmt.Sprintf("minReplicas (%d) cannot exceed maxReplicas (%d)", *scaling.MinReplicas, scaling.MaxReplicas),
		))
	}

	// WVA is required when scaling is configured — it provides the scaling mechanism
	if scaling.WVA == nil {
		allErrs = append(allErrs, field.Required(
			scalingPath.Child("wva"),
			"wva is required when scaling is configured; it provides the autoscaling mechanism",
		))
		return allErrs
	}

	// Validate WVA configuration
	wvaPath := scalingPath.Child("wva")

	// HPA and KEDA are mutually exclusive
	if scaling.WVA.HPA != nil && scaling.WVA.KEDA != nil {
		allErrs = append(allErrs, field.Invalid(
			wvaPath,
			scaling.WVA,
			"hpa and keda are mutually exclusive; choose one actuator backend",
		))
	}

	// Must specify at least one actuator
	if scaling.WVA.HPA == nil && scaling.WVA.KEDA == nil {
		allErrs = append(allErrs, field.Required(
			wvaPath,
			"either hpa or keda must be specified as the actuator backend",
		))
	}

	// Validate variantCost format (must be a non-negative numeric string, e.g., "10", "10.0", "0.5")
	if scaling.WVA.VariantCost != "" {
		if !variantCostPattern.MatchString(scaling.WVA.VariantCost) {
			allErrs = append(allErrs, field.Invalid(
				wvaPath.Child("variantCost"),
				scaling.WVA.VariantCost,
				"variantCost must be a non-negative numeric string (e.g., \"10\", \"10.0\", \"0.5\")",
			))
		}
	}

	// Validate KEDA advanced fields that are controller-owned and must not be set by users
	if scaling.WVA.KEDA != nil && scaling.WVA.KEDA.Advanced != nil {
		kedaPath := wvaPath.Child("keda")
		sm := scaling.WVA.KEDA.Advanced.ScalingModifiers
		if sm.Formula != "" || sm.Target != "" || sm.ActivationTarget != "" || string(sm.MetricType) != "" {
			allErrs = append(allErrs, field.Forbidden(
				kedaPath.Child("advanced", "scalingModifiers"),
				"scalingModifiers must not be set; WVA controls the scaling metric formula and logic",
			))
		}
		if scaling.WVA.KEDA.Advanced.HorizontalPodAutoscalerConfig != nil &&
			scaling.WVA.KEDA.Advanced.HorizontalPodAutoscalerConfig.Name != "" {
			allErrs = append(allErrs, field.Forbidden(
				kedaPath.Child("advanced", "horizontalPodAutoscalerConfig", "name"),
				"horizontalPodAutoscalerConfig.name must not be set; the controller manages the HPA name",
			))
		}
	}

	// Validate KEDA idleReplicaCount requires minReplicas and must be less than it
	if scaling.WVA.KEDA != nil && scaling.WVA.KEDA.IdleReplicaCount != nil {
		if scaling.MinReplicas == nil {
			allErrs = append(allErrs, field.Required(
				scalingPath.Child("minReplicas"),
				fmt.Sprintf("minReplicas is required when idleReplicaCount is set; "+
					"idleReplicaCount (%d) must be less than minReplicas",
					*scaling.WVA.KEDA.IdleReplicaCount),
			))
		} else if *scaling.WVA.KEDA.IdleReplicaCount >= *scaling.MinReplicas {
			allErrs = append(allErrs, field.Invalid(
				wvaPath.Child("keda").Child("idleReplicaCount"),
				*scaling.WVA.KEDA.IdleReplicaCount,
				fmt.Sprintf("idleReplicaCount (%d) must be less than minReplicas (%d); "+
					"idleReplicaCount defines the replica floor when no triggers are active",
					*scaling.WVA.KEDA.IdleReplicaCount, *scaling.MinReplicas),
			))
		}
	}

	return allErrs
}

// immutableField returns a *Error indicating "unsupported mutation".
// This is used to report unsupported mutation of values.
func immutableField(path *field.Path, value interface{}, detail string) *field.Error {
	return &field.Error{Type: field.ErrorTypeNotSupported, Field: path.String(), BadValue: value, Detail: detail}
}
