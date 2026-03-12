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

package llmisvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// Configuration template name suffixes for different LLM deployment patterns
// These configs are automatically applied based on the service configuration
const (
	// Single node deployment template
	configTemplateNameSuffix = "config-llm-template"
	// Disaggregated prefill/decode templates
	configDecodeTemplateNameSuffix  = "config-llm-decode-template"
	configPrefillTemplateNameSuffix = "config-llm-prefill-template"
	// Pipeline parallel worker configurations
	configDecodeWorkerPipelineParallelNameSuffix  = "config-llm-decode-worker-pipeline-parallel"
	configWorkerPipelineParallelNameSuffix        = "config-llm-worker-pipeline-parallel"
	configPrefillWorkerPipelineParallelNameSuffix = "config-llm-prefill-worker-pipeline-parallel"
	// Data parallel worker configurations
	configWorkerDataParallelNameSuffix        = "config-llm-worker-data-parallel"
	configDecodeWorkerDataParallelNameSuffix  = "config-llm-decode-worker-data-parallel"
	configPrefillWorkerDataParallelNameSuffix = "config-llm-prefill-worker-data-parallel"
	// Router and scheduler configurations
	configRouterSchedulerNameSuffix           = "config-llm-scheduler"
	configRouterRouteNameSuffix               = "config-llm-router-route"
	configSchedulerLatencyPredictorNameSuffix = "config-llm-scheduler-latency-predictor"
	// Tracing configurations
	configTracingNameSuffix = "config-llm-tracing"
)

var (
	configPrefix                            = constants.GetEnvOrDefault("LLM_INFERENCE_SERVICE_CONFIG_PREFIX", "kserve-")
	configTemplateName                      = configPrefix + configTemplateNameSuffix
	configDecodeTemplateName                = configPrefix + configDecodeTemplateNameSuffix
	configDecodeWorkerPipelineParallelName  = configPrefix + configDecodeWorkerPipelineParallelNameSuffix
	configWorkerPipelineParallelName        = configPrefix + configWorkerPipelineParallelNameSuffix
	configWorkerDataParallelName            = configPrefix + configWorkerDataParallelNameSuffix
	configDecodeWorkerDataParallelName      = configPrefix + configDecodeWorkerDataParallelNameSuffix
	configPrefillTemplateName               = configPrefix + configPrefillTemplateNameSuffix
	configPrefillWorkerPipelineParallelName = configPrefix + configPrefillWorkerPipelineParallelNameSuffix
	configPrefillWorkerDataParallelName     = configPrefix + configPrefillWorkerDataParallelNameSuffix
	configRouterSchedulerName               = configPrefix + configRouterSchedulerNameSuffix
	configRouterRouteName                   = configPrefix + configRouterRouteNameSuffix
	configSchedulerLatencyPredictorName     = configPrefix + configSchedulerLatencyPredictorNameSuffix
	configTracingName                       = configPrefix + configTracingNameSuffix
)

// FIXME move those presets to well-known when they're finally known :)
var _ = sets.New[string](
	configPrefillWorkerPipelineParallelName,
	configDecodeWorkerPipelineParallelName,
	configWorkerPipelineParallelName,
)

// WellKnownDefaultConfigs contains the set of default configuration templates
// that are automatically applied based on the LLM service deployment pattern
var WellKnownDefaultConfigs = sets.New[string](
	configTemplateName,
	configDecodeTemplateName,
	configWorkerDataParallelName,
	configDecodeWorkerDataParallelName,
	configPrefillTemplateName,
	configPrefillWorkerDataParallelName,
	configRouterSchedulerName,
	configRouterRouteName,
	configSchedulerLatencyPredictorName,
	configTracingName,
)

const (
	precisePrefixCacheScorerName = "precise-prefix-cache-scorer"
)

var useVersionedConfig, _ = strconv.ParseBool(constants.GetEnvOrDefault("LLM_INFERENCE_SERVICE_VERSIONED_CONFIG", "true"))

// CombineOption is a functional option for combineBaseRefsConfig
type CombineOption func(*combineOptions)

type combineOptions struct {
	skipClearSchedulerConfigRef bool
}

// CombinedConfig holds the output of combineBaseRefsConfig.
type CombinedConfig struct {
	// Config is the merged LLMInferenceServiceConfig.
	Config *v1alpha2.LLMInferenceServiceConfig
	// AppliedConfigRefs is the ordered list of configs that were applied, tagged by source.
	// May be incomplete when returned alongside a non-nil error.
	AppliedConfigRefs []v1alpha2.AppliedConfigRef
}

// WithSkipClearSchedulerConfigRef prevents clearing the scheduler config ref after resolving.
// This is useful when the caller needs to check which ConfigMap was referenced.
func WithSkipClearSchedulerConfigRef() CombineOption {
	return func(o *combineOptions) {
		o.skipClearSchedulerConfigRef = true
	}
}

func (r *LLMISVCReconciler) reconcileBaseRefs(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) (*v1alpha2.LLMInferenceServiceConfig, error) {
	// Combine base configurations with service-specific overrides
	// This includes default configs based on deployment pattern (single node, multi-node, etc.)
	result, err := r.combineBaseRefsConfig(ctx, llmSvc, config)
	if err != nil {
		if utils.GetForceStopRuntime(llmSvc) {
			llmSvc.MarkPresetsCombinedNotReady("Stopped", "Service is stopped with warning: %v", err.Error())

			return &v1alpha2.LLMInferenceServiceConfig{
				Spec: *llmSvc.Spec.DeepCopy(),
			}, nil
		}

		llmSvc.Status.AppliedConfigRefs = nil

		var cfgNotFound *configNotFoundError
		if errors.As(err, &cfgNotFound) {
			llmSvc.MarkPresetsCombinedNotReady("ConfigNotFound", cfgNotFound.Error())
			return nil, nil // watch on LLMInferenceServiceConfig re-triggers when the config is recreated
		}

		llmSvc.MarkPresetsCombinedNotReady("CombineBaseError", err.Error())
		return nil, fmt.Errorf("failed to combine base-configurations: %w", err)
	}

	// Persist only the applied configs from successful reconciliation.
	llmSvc.Status.AppliedConfigRefs = result.AppliedConfigRefs
	llmSvc.MarkPresetsCombinedReady()

	return result.Config, nil
}

// combineBaseRefsConfig applies well-known config overlays to inject default values for various components, when some components are
// enabled. These LLMInferenceServiceConfig resources must exist in either resource namespace (prioritized) or
// SystemNamespace (e.g. `kserve`).
// It determines which deployment pattern is being used (single node, multi-node, disaggregated) and applies appropriate defaults.
func (r *LLMISVCReconciler) combineBaseRefsConfig(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, reconcilerConfig *Config, opts ...CombineOption) (*CombinedConfig, error) {
	options := &combineOptions{}
	for _, opt := range opts {
		opt(options)
	}
	logger := log.FromContext(ctx).WithName("combineBaseRefsConfig")

	wr := &WellKnownConfigResolver{}
	wr.Attach(llmSvc)

	// Creates the initial spec with the merged BaseRefs, so that we know what's "Enabled".
	resolvedSpec := *llmSvc.Spec.DeepCopy()
	for _, ref := range llmSvc.Spec.BaseRefs {
		cfg, err := r.getConfig(ctx, llmSvc, ref.Name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			var resolvedErr error
			resolvedSpec, resolvedErr = mergeSpecs(ctx, resolvedSpec, cfg.Spec)
			if resolvedErr != nil {
				return nil, fmt.Errorf("failed to merge specs: %w", resolvedErr)
			}
		}
	}

	if resolvedSpec.Model.Name != nil {
		// If original model name was defaulted check if it was not substituted by baseRef
		llmSvc.Spec.Model.Name = resolvedSpec.Model.Name
	}

	logger.V(2).Info("Resolved spec", "spec", resolvedSpec)

	refs := make([]corev1.LocalObjectReference, 0, len(llmSvc.Spec.BaseRefs))
	if resolvedSpec.Router != nil && resolvedSpec.Router.Scheduler != nil && !resolvedSpec.Router.Scheduler.Pool.HasRef() {
		refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configRouterSchedulerName)})
	}
	if hasLatencyProducerInSpec(resolvedSpec) {
		refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configSchedulerLatencyPredictorName)})
	}
	if resolvedSpec.Router != nil && resolvedSpec.Router.Route != nil && !resolvedSpec.Router.Route.HTTP.HasRefs() {
		// For the HTTP route configuration we don't use versioned defaults since this configuration depends on the
		// GW API provider version.
		refs = append(refs, corev1.LocalObjectReference{Name: configRouterRouteName})
	}
	// Inject tracing default configs when tracing is enabled (field is non-nil)
	if resolvedSpec.Tracing != nil {
		refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configTracingName)})
	}

	if resolvedSpec.Prefill != nil { // P/D
		// Prefill
		switch {
		case resolvedSpec.Prefill.Worker == nil:
			// single-node prefill
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configPrefillTemplateName)})
		case resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsDataParallel():
			// multi-node Data Parallel prefill
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configPrefillWorkerDataParallelName)})
		case resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel prefill
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configPrefillWorkerPipelineParallelName)})
		}
		// Decode
		switch {
		case resolvedSpec.Worker == nil:
			// single-node decode
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configDecodeTemplateName)})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsDataParallel():
			// multi-node Data Parallel decode
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configDecodeWorkerDataParallelName)})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel decode
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configDecodeWorkerPipelineParallelName)})
		}
	} else { // Non P/D
		switch {
		case resolvedSpec.Worker == nil:
			// single-node
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configTemplateName)})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsDataParallel():
			// multi-node Data Parallel
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configWorkerDataParallelName)})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel
			refs = append(refs, corev1.LocalObjectReference{Name: wr.Resolve(llmSvc, configWorkerPipelineParallelName)})
		}
	}

	// Append explicit base refs to override well know configs.
	wellKnownCount := len(refs)
	refs = append(refs, llmSvc.Spec.BaseRefs...)

	specs := make([]v1alpha2.LLMInferenceServiceSpec, 0, len(refs))
	appliedRefs := make([]v1alpha2.AppliedConfigRef, 0, len(refs))
	for i, ref := range refs {
		cfg, err := r.getConfig(ctx, llmSvc, ref.Name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			specs = append(specs, cfg.Spec)
			source := v1alpha2.AppliedConfigSourcePreset
			if i >= wellKnownCount {
				source = v1alpha2.AppliedConfigSourceUserRef
			}
			appliedRefs = append(appliedRefs, v1alpha2.AppliedConfigRef{
				Name:      gwapiv1.ObjectName(ref.Name),
				Namespace: gwapiv1.Namespace(cfg.Namespace),
				Source:    source,
			})
		}
	}
	spec, err := MergeSpecs(ctx, append(specs, llmSvc.Spec)...)
	if err != nil {
		return nil, fmt.Errorf("failed to merge specs: %w", err)
	}

	llmSvcCfg := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: *llmSvc.ObjectMeta.DeepCopy(),
		Spec:       spec,
	}

	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Scheduler != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Pool != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Pool.Spec != nil &&
		len(llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.Selector.MatchLabels) == 0 {
		selector := GetWorkloadLabelSelector(llmSvc.ObjectMeta, &llmSvcCfg.Spec)

		gieSelector := make(map[igwapi.LabelKey]igwapi.LabelValue, len(selector))
		for k, v := range selector {
			gieSelector[igwapi.LabelKey(k)] = igwapi.LabelValue(v)
		}
		llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.Selector.MatchLabels = gieSelector
	}

	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Scheduler != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Template != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Template.ServiceAccountName == "" {
		llmSvcCfg.Spec.Router.Scheduler.Template.ServiceAccountName = kmeta.ChildName(llmSvc.GetName(), "-epp-sa")
	}

	llmSvcCfg, err = ReplaceVariables(llmSvc, llmSvcCfg, reconcilerConfig)
	if err != nil {
		return &CombinedConfig{Config: llmSvcCfg, AppliedConfigRefs: appliedRefs}, err
	}

	injectManagedDRAIntoConfig(llmSvc, llmSvcCfg)

	// Update HTTPRoute parentRefs to point to the custom gateway if Gateway.Refs is specified.
	// This ensures the managed HTTPRoute references the correct gateway instead of the default one from presets.
	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Route != nil &&
		llmSvcCfg.Spec.Router.Route.HTTP.HasSpec() &&
		llmSvcCfg.Spec.Router.Gateway.HasRefs() {
		llmSvcCfg.Spec.Router.Route.HTTP.Spec.ParentRefs = ToParentRefs(llmSvcCfg.Spec.Router.Gateway.Refs)
	}

	// Point HTTPRoute to a Service if there is no Scheduler or InferencePool, and the HTTPRoute uses the default
	// InferencePool (to handle cases where the HTTPRoute Spec uses a custom BackendRef).
	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Route != nil &&
		llmSvcCfg.Spec.Router.Route.HTTP.HasSpec() &&
		llmSvcCfg.Spec.Router.Scheduler == nil {
		for i := range llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules {
			for j := range llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs {
				if isDefaultBackendRef(llmSvc, llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].BackendRef) {
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Group = ptr.To[gwapiv1.Group]("")
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Kind = ptr.To[gwapiv1.Kind]("Service")
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Name = gwapiv1.ObjectName(workloadServiceName(llmSvc))
				}
			}
		}
	}

	// Point HTTPRoute to InferencePool reference if specified.
	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Route != nil &&
		llmSvcCfg.Spec.Router.Route.HTTP.HasSpec() &&
		llmSvcCfg.Spec.Router.Scheduler != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Pool.HasRef() {
		for i := range llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules {
			for j := range llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs {
				if isDefaultBackendRef(llmSvc, llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].BackendRef) {
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Name = gwapiv1.ObjectName(llmSvcCfg.Spec.Router.Scheduler.Pool.Ref.Name)
				}
			}
		}
	}

	// Resolve the external Scheduler configuration.
	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Scheduler != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Config != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Config.Ref != nil {
		cmName := llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Name
		cm, err := Get(ctx, r.Client, client.ObjectKey{Namespace: llmSvc.GetNamespace(), Name: cmName}, &corev1.ConfigMap{}, WithGetFallbackAPIServerConfigMap(r.Clientset))
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return &CombinedConfig{Config: llmSvcCfg, AppliedConfigRefs: appliedRefs}, fmt.Errorf("failed to get ConfigMap %s/%s: %w", llmSvc.GetNamespace(), cmName, err)
			}

			if strings.HasPrefix(cmName, "config-scheduler-") {
				cm, err = Get(ctx, r.Client, client.ObjectKey{Namespace: constants.KServeNamespace, Name: cmName}, &corev1.ConfigMap{}, WithGetFallbackAPIServerConfigMap(r.Clientset))
				if err != nil {
					return &CombinedConfig{Config: llmSvcCfg, AppliedConfigRefs: appliedRefs}, fmt.Errorf("failed to get scheduler config %q from namespaces [%q, %q]: %w", cmName, llmSvc.Namespace, constants.KServeNamespace, err)
				}
			}
		}
		if llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key == "" {
			llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key = "epp"
		}
		cfg, ok := cm.Data[llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key]
		if !ok {
			return &CombinedConfig{Config: llmSvcCfg, AppliedConfigRefs: appliedRefs}, fmt.Errorf("ConfigMap %s/%s doesn't have key %q in data",
				cm.GetNamespace(),
				cm.GetName(),
				llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key,
			)
		}
		llmSvcCfg.Spec.Router.Scheduler.Config.Inline = &runtime.RawExtension{Raw: []byte(cfg)}
		// Clear the Ref since we've resolved it to Inline - the validator rejects having both set.
		// Skip clearing if the caller needs to check which ConfigMap was referenced.
		if !options.skipClearSchedulerConfigRef {
			llmSvcCfg.Spec.Router.Scheduler.Config.Ref = nil
		}

		// Warn if the resolved ConfigMap contains predicted-latency-producer but the
		// well-known config was not injected (because detection runs before Ref resolution).
		if hasLatencyProducerInSpec(llmSvcCfg.Spec) {
			r.Eventf(llmSvc, corev1.EventTypeWarning, "LatencyPredictorConfigRef",
				"predicted-latency-producer plugin detected in Config.Ref ConfigMap %q; "+
					"latency predictor sidecar injection requires Config.Inline instead of Config.Ref", cmName)
		}
	}

	// The v1 InferencePool CRD requires port when endpointPickerRef.kind is "Service" (or
	// unspecified, which defaults to "Service"). Configs created before GIE v1.2.0
	// omit the port field entirely. Without this default the controller's
	// dry-run update fails the CEL rule: "port is required when kind is 'Service' or
	// unspecified (defaults to 'Service')". We check both Kind=="Service" and Kind==""
	// because the kubebuilder default is only applied server-side during admission, not
	// during in-process deserialization.
	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Scheduler != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Pool != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Pool.Spec != nil &&
		(llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.EndpointPickerRef.Port == nil || llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.EndpointPickerRef.Port.Number == 0) &&
		(llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.EndpointPickerRef.Kind == "Service" || llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.EndpointPickerRef.Kind == "") {
		llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.EndpointPickerRef.Port = ptr.To(igwapi.Port{Number: 9002})
	}

	// Skip validation when we're only using the result for matching (not for reconciliation).
	// When skipClearSchedulerConfigRef is true, both Inline and Ref may be set, which would fail validation.
	if !options.skipClearSchedulerConfigRef {
		err = r.Validator(ctx, &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      llmSvc.Name,
				Namespace: llmSvc.GetNamespace(),
			},
			Spec: llmSvcCfg.Spec,
		})
		if err != nil {
			return &CombinedConfig{Config: llmSvcCfg, AppliedConfigRefs: appliedRefs}, err
		}
	}

	// Resolve LoRA adapters from the final merged spec and embed the result in reconcilerConfig.
	// Doing this here ties resolution to the config-merge step so all downstream workload
	// functions share a single, consistent resolution rather than each re-parsing the spec.
	loraAdapters, err := enumerateLoRAAdapters(llmSvcCfg.Spec)
	if err != nil {
		return &CombinedConfig{Config: llmSvcCfg, AppliedConfigRefs: appliedRefs}, fmt.Errorf("failed to enumerate LoRA adapters: %w", err)
	}
	reconcilerConfig.ResolvedLoRAAdapters = loraAdapters

	return &CombinedConfig{Config: llmSvcCfg, AppliedConfigRefs: appliedRefs}, nil
}

func isUsingTokenizerSidecar(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil || spec.Router.Scheduler.Template == nil {
		return false
	}
	return utils.GetContainerWithName(spec.Router.Scheduler.Template, tokenizerContainerName) != nil
}

func (r *LLMISVCReconciler) isModelBasedRoutingEnabled(
	ctx context.Context,
	llmSvc *v1alpha2.LLMInferenceService,
	cfg *Config,
) bool {
	if cfg.ModelBasedRoutingHeaderName == "" {
		return false
	}

	// Ensure the workload has been deployed with the alternative served model name for model-based routing.
	// Older presets associated with the previous version will have this unset.
	if v, ok := llmSvc.Spec.Annotations[AnnotationModelBasedRoutingEnabled]; !ok || v != "true" {
		return false
	}

	switch cfg.ModelBasedRoutingMode {
	case ModelBasedRoutingDisabled:
		return false
	case ModelBasedRoutingForced:
		return true
	default:
		gateways, err := r.CollectReferencedGateways(ctx, llmSvc)
		if err != nil {
			log.FromContext(ctx).Error(err, "failed to collect reference gateways to establish model-based routing enabled, defaulting to ModelBasedRoutingMode", "ModelBasedRoutingMode", cfg.ModelBasedRoutingMode)
			return cfg.ModelBasedRoutingMode != ModelBasedRoutingDisabled
		}
		for _, gw := range gateways {
			if gw.Annotations[AnnotationModelBasedRoutingEnabled] == "false" {
				return false
			}
		}
		return true
	}
}

func stripModelBasedRoutingRules(rules []gwapiv1.HTTPRouteRule, headerName string) []gwapiv1.HTTPRouteRule {
	if headerName == "" {
		return rules
	}
	var filtered []gwapiv1.HTTPRouteRule
	for i := range rules {
		var kept []gwapiv1.HTTPRouteMatch
		for _, match := range rules[i].Matches {
			if !isModelBasedRoutingMatch(match, headerName) {
				kept = append(kept, match)
			}
		}
		if len(kept) > 0 {
			rules[i].Matches = kept
			filtered = append(filtered, rules[i])
		}
	}
	return filtered
}

// expandLoRAAdapterMatches duplicates model-routing header matches for each LoRA
// adapter so that adapter requests are routed through the same backend as the base
// model. Matches within a Gateway API rule are OR'd, so a rule ends up matching
// "base model OR adapter-1 OR adapter-2 …" — all targeting the same InferencePool.
//
// Only matches whose header name equals headerName are duplicated; path-only rules
// and rules with unrelated headers are left untouched.
func expandLoRAAdapterMatches(rules []gwapiv1.HTTPRouteRule, namespace string, adapters []v1alpha2.LLMModelSpec, headerName string) {
	if headerName == "" || len(adapters) == 0 {
		return
	}
	for i := range rules {
		var adapterMatches []gwapiv1.HTTPRouteMatch
		for _, match := range rules[i].Matches {
			if !isModelBasedRoutingMatch(match, headerName) {
				continue
			}
			for _, adapter := range adapters {
				if adapter.Name == nil {
					continue
				}
				am := *match.DeepCopy()
				for h := range am.Headers {
					if string(am.Headers[h].Name) == headerName {
						am.Headers[h].Value = fmt.Sprintf("publishers/%s/models/%s", namespace, *adapter.Name)
					}
				}
				adapterMatches = append(adapterMatches, am)
			}
		}
		rules[i].Matches = append(rules[i].Matches, adapterMatches...)
	}
}

// ToParentRefs converts a slice of UntypedObjectReference (gateway refs) to a slice
// of gwapiv1.ParentReference suitable for setting on an HTTPRoute's CommonRouteSpec.
// When a ref includes SectionName, the generated ParentReference targets that
// specific Gateway listener; otherwise the route attaches to all listeners.
func ToParentRefs(gatewayRefs []v1alpha2.GatewayObjectReference) []gwapiv1.ParentReference {
	parentRefs := make([]gwapiv1.ParentReference, 0, len(gatewayRefs))
	for _, ref := range gatewayRefs {
		parentRef := gwapiv1.ParentReference{
			Name:        ref.Name,
			Group:       ptr.To(gwapiv1.Group("gateway.networking.k8s.io")),
			Kind:        ptr.To(gwapiv1.Kind("Gateway")),
			SectionName: ref.SectionName,
		}
		// Keep Namespace nil when the ref omits it so Gateway API defaults to
		// the route namespace, matching validation and watch matching behavior.
		if ref.Namespace != "" {
			namespace := ref.Namespace
			parentRef.Namespace = &namespace
		}
		parentRefs = append(parentRefs, parentRef)
	}
	return parentRefs
}

// templateGlobalConfig exposes only the non-sensitive fields of Config to templates.
// StorageConfig and CredentialConfig are intentionally excluded to prevent template
// injection from accessing internal controller configuration.
type templateGlobalConfig struct {
	SystemNamespace         string
	IngressGatewayName      string
	IngressGatewayNamespace string
	EnableTLS               bool

	// ModelBasedRoutingHeaderName is the HTTP header used to select a model in
	// shared-gateway deployments (e.g. "X-Gateway-Model-Name"). Exposed here so
	// that HTTPRoute templates can reference it via {{ .GlobalConfig.ModelBasedRoutingHeaderName }}.
	ModelBasedRoutingHeaderName string

	// InferencePoolNamespacedName represents the inference pool namespaced reference in the format "<namespace>/<name>",
	// or simply `<name>`.
	InferencePoolNamespacedName string
}

// removeEmptyStringsFromArrays recursively walks a JSON-unmarshaled object and removes
// empty string elements from arrays. This is used to clean up template output where
// conditional templates (e.g., {{- if ... -}}...{{- end }}) evaluate to empty strings.
func removeEmptyStringsFromArrays(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, v := range val {
			val[k] = removeEmptyStringsFromArrays(v)
		}
		return val
	case []any:
		filtered := make([]any, 0, len(val))
		for _, item := range val {
			cleaned := removeEmptyStringsFromArrays(item)
			// Skip empty strings
			if s, ok := cleaned.(string); ok && s == "" {
				continue
			}
			filtered = append(filtered, cleaned)
		}
		return filtered
	default:
		return val
	}
}

// ReplaceVariables processes the configuration as a Go template to substitute
// variables with values from the LLM service and global configuration.
func ReplaceVariables(llmSvc *v1alpha2.LLMInferenceService, llmSvcCfg *v1alpha2.LLMInferenceServiceConfig, reconcilerConfig *Config) (*v1alpha2.LLMInferenceServiceConfig, error) {
	templateBytes, err := json.Marshal(llmSvcCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config for template processing: %w", err)
	}
	buf := bytes.NewBuffer(nil)
	var gc templateGlobalConfig
	if reconcilerConfig != nil {
		gc = templateGlobalConfig{
			SystemNamespace:             reconcilerConfig.SystemNamespace,
			IngressGatewayName:          reconcilerConfig.IngressGatewayName,
			IngressGatewayNamespace:     reconcilerConfig.IngressGatewayNamespace,
			EnableTLS:                   reconcilerConfig.EnableTLS,
			ModelBasedRoutingHeaderName: reconcilerConfig.ModelBasedRoutingHeaderName,
		}
		infPoolNamespacedName := types.NamespacedName{
			Name:      (&v1alpha2.SchedulerSpec{}).InferencePoolName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
		}
		if llmSvcCfg.Spec.Router != nil {
			infPoolNamespacedName.Name = llmSvcCfg.Spec.Router.Scheduler.InferencePoolName(llmSvc)
		}
		gc.InferencePoolNamespacedName = infPoolNamespacedName.String()
	}
	config := struct {
		*v1alpha2.LLMInferenceService
		GlobalConfig templateGlobalConfig
	}{
		LLMInferenceService: llmSvc,
		GlobalConfig:        gc,
	}
	t, err := template.New("config").
		Funcs(map[string]any{
			"ChildName": kmeta.ChildName,
			"kvTransferConfig": func(spec any) string {
				if spec == nil {
					return ""
				}
				kv, ok := spec.(*v1alpha2.KVCacheOffloadingSpec)
				if !ok || kv == nil {
					return ""
				}
				extraConfig := map[string]any{
					"spec_name":        "TieringOffloadingSpec",
					"cpu_bytes_to_use": kv.CPU.Value(),
				}
				if kv.EvictionPolicy != "" {
					extraConfig["eviction_policy"] = kv.EvictionPolicy
				}
				kvConfig := map[string]any{
					"kv_connector":              "OffloadingConnector",
					"kv_role":                   "kv_both",
					"kv_connector_extra_config": extraConfig,
				}
				b, err := json.Marshal(kvConfig)
				if err != nil {
					return ""
				}
				// Escape " as \" so the value embeds safely in a bash double-quoted
				// assignment and in the JSON template string that ReplaceVariables renders.
				return "--kv-transfer-config '" + strings.ReplaceAll(string(b), `"`, `\"`) + "'"
			},
			// shutdownTimeout computes the vLLM --shutdown-timeout value from a *corev1.PodSpec
			// (or nil): max(0, tgps - preStop - min(5, tgps)), defaulting tgps to 60 when unset.
			// The 5-second buffer reserves time for signal propagation and final process cleanup
			// before Kubernetes sends SIGKILL.
			"shutdownTimeout": func(spec any, preStop int64) int64 {
				const defaultTGPS = int64(60)
				var tgpsVal int64
				if spec != nil {
					if ps, ok := spec.(*corev1.PodSpec); ok && ps != nil && ps.TerminationGracePeriodSeconds != nil {
						tgpsVal = *ps.TerminationGracePeriodSeconds
					} else {
						tgpsVal = defaultTGPS
					}
				} else {
					tgpsVal = defaultTGPS
				}
				buf := min(int64(5), tgpsVal)
				result := tgpsVal - preStop - buf
				if result < 0 {
					return 0
				}
				return result
			},
		}).
		Option("missingkey=error").
		Parse(string(templateBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template config: %w", err)
	}
	if err := t.Execute(buf, config); err != nil {
		return nil, fmt.Errorf("failed to merge config: %w", err)
	}

	// First unmarshal into a generic map to clean up empty strings from arrays
	var intermediate map[string]any
	if err := json.Unmarshal(buf.Bytes(), &intermediate); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from template: %w", err)
	}

	// Remove empty strings that resulted from conditional templates evaluating to ""
	cleaned := removeEmptyStringsFromArrays(intermediate)

	// Marshal back to JSON and unmarshal into the final typed struct
	cleanedBytes, err := json.Marshal(cleaned)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cleaned config: %w", err)
	}

	out := &v1alpha2.LLMInferenceServiceConfig{}
	if err := json.Unmarshal(cleanedBytes, out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cleaned config: %w", err)
	}
	return out, nil
}

// configNotFoundError is returned by getConfig when an LLMInferenceServiceConfig
// cannot be found in either the service namespace or the system namespace.
// It carries the config name and the ordered list of namespaces that were searched.
// TODO: extend Error() to list the LLMInferenceServiceConfig resources that do exist
// in the searched namespaces, so the operator can see available alternatives at a glance.
type configNotFoundError struct {
	Name       string
	Namespaces []string
}

func (e *configNotFoundError) Error() string {
	return fmt.Sprintf("LLMInferenceServiceConfig %q not found in namespaces %v", e.Name, e.Namespaces)
}

// getConfig retrieves kserveapis.LLMInferenceServiceConfig with the given name from either the kserveapis.LLMInferenceService
// namespace or from the SystemNamespace (e.g. 'kserve'), prioritizing the former.
// This allows for both global default configs and service-specific overrides.
func (r *LLMISVCReconciler) getConfig(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, name string) (*v1alpha2.LLMInferenceServiceConfig, error) {
	cfg := &v1alpha2.LLMInferenceServiceConfig{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: llmSvc.Namespace}, cfg); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get LLMInferenceServiceConfig %s/%s: %w", llmSvc.Namespace, name, err)
		}
		cfg = &v1alpha2.LLMInferenceServiceConfig{}
		if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: constants.KServeNamespace}, cfg); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, &configNotFoundError{Name: name, Namespaces: []string{llmSvc.Namespace, constants.KServeNamespace}}
			}
			return nil, fmt.Errorf("failed to get LLMInferenceServiceConfig %q from namespaces [%q, %q]: %w",
				name, llmSvc.Namespace, constants.KServeNamespace, err)
		}
		return cfg, nil
	}
	return cfg, nil
}

func MergeSpecs(ctx context.Context, cfgs ...v1alpha2.LLMInferenceServiceSpec) (v1alpha2.LLMInferenceServiceSpec, error) {
	if len(cfgs) == 0 {
		return v1alpha2.LLMInferenceServiceSpec{}, nil
	}

	out := cfgs[0]
	for i := 1; i < len(cfgs); i++ {
		cfg := cfgs[i]
		var err error
		out, err = mergeSpecs(ctx, out, cfg)
		if err != nil {
			return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("failed to merge specs: %w", err)
		}
	}
	return out, nil
}

// mergeSpecs performs a strategic merge by creating a clean patch from the override
// object and applying it to the base object.
func mergeSpecs(ctx context.Context, base, override v1alpha2.LLMInferenceServiceSpec) (v1alpha2.LLMInferenceServiceSpec, error) {
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("could not marshal base spec: %w", err)
	}

	// To create a patch containing only the fields specified in the override,
	// we create a patch between a zero-valued ("empty") object and the override object.
	// This prevents zero-valued fields in the override struct (e.g., an empty string for an
	// unspecified image) from incorrectly wiping out values from the base.
	zero := v1alpha2.LLMInferenceServiceSpec{}
	zeroJSON, err := json.Marshal(zero)
	if err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("could not marshal zero spec: %w", err)
	}

	// This ensures that only explicitly set fields in the override are applied, preventing
	// zero-valued fields from overwriting meaningful base values.
	override.SetDefaults(ctx)

	overrideJSON, err := json.Marshal(override)
	if err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("could not marshal override spec: %w", err)
	}

	logger := log.FromContext(ctx)

	// Create the patch. It will only contain the non-default fields from the override.
	patch, err := strategicpatch.CreateTwoWayMergePatch(zeroJSON, overrideJSON, v1alpha2.LLMInferenceServiceSpec{})
	if err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("could not create merge patch from override: %w", err)
	}

	logger.V(2).Info("merging specs (patch)", "patch", string(patch), "base", string(baseJSON), "override", string(overrideJSON), "zero", string(zeroJSON))

	// Apply this "clean" patch to the base JSON. The strategic merge logic will correctly
	// merge lists and objects based on their Kubernetes patch strategy annotations.
	mergedJSON, err := strategicpatch.StrategicMergePatch(baseJSON, patch, v1alpha2.LLMInferenceServiceSpec{})
	if err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("could not apply merge patch: %w", err)
	}

	// Unmarshal the merged JSON back into a Go struct.
	var finalSpec v1alpha2.LLMInferenceServiceSpec
	if err := json.Unmarshal(mergedJSON, &finalSpec); err != nil {
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("could not unmarshal merged spec: %w", err)
	}
	return finalSpec, nil
}

func isDefaultBackendRef(llmSvc *v1alpha2.LLMInferenceService, ref gwapiv1.BackendRef) bool {
	defaultInfPoolName := (&v1alpha2.SchedulerSpec{}).InferencePoolName(llmSvc)
	// Check Kind and Name only - Group can be either v1 or v1alpha2
	return ptr.Deref[gwapiv1.Kind](ref.Kind, "") == "InferencePool" &&
		string(ref.Name) == defaultInfPoolName
}

type ModelBasedRoutingMode string

const (
	ModelBasedRoutingEnabled  ModelBasedRoutingMode = "enabled"
	ModelBasedRoutingForced   ModelBasedRoutingMode = "forced"
	ModelBasedRoutingDisabled ModelBasedRoutingMode = "disabled"
)

func parseModelBasedRoutingMode(s string) ModelBasedRoutingMode {
	switch strings.ToLower(s) {
	case "forced":
		return ModelBasedRoutingForced
	case "disabled":
		return ModelBasedRoutingDisabled
	default:
		return ModelBasedRoutingEnabled
	}
}

const (
	StaticWellKnownConfigResolverPrefix = "serving.kserve.io/"
)

// WellKnownConfigResolver snapshots well-known config name mappings into Status.Annotations
// and resolves pinned names during reconciliation. This ensures that future prefix changes
// don't affect existing services.
type WellKnownConfigResolver struct{}

// Attach pins the current well-known config name mappings into the LLMInferenceService's
// Status.Annotations at first reconciliation. Already-pinned entries are preserved.
// NOTE: This mutates llmSvc.Status.Annotations in-place; the caller is responsible for
// persisting the status update.
func (w *WellKnownConfigResolver) Attach(llmSvc *v1alpha2.LLMInferenceService) {
	if !useVersionedConfig {
		return
	}
	for _, t := range WellKnownDefaultConfigs.UnsortedList() {
		suffix, _ := strings.CutPrefix(t, configPrefix)
		key := StaticWellKnownConfigResolverPrefix + suffix

		if v, ok := llmSvc.Status.Annotations[key]; ok && v != "" {
			continue
		}

		if llmSvc.Status.Annotations == nil {
			llmSvc.Status.Annotations = map[string]string{}
		}
		llmSvc.Status.Annotations[key] = t
	}
}

// Resolve returns the pinned config name from Status.Annotations if versioned config
// resolution is enabled, otherwise returns the name as-is.
func (w *WellKnownConfigResolver) Resolve(llmSvc *v1alpha2.LLMInferenceService, name string) string {
	if !useVersionedConfig || llmSvc.Status.Annotations == nil {
		return name
	}

	suffix, _ := strings.CutPrefix(name, configPrefix)
	key := StaticWellKnownConfigResolverPrefix + suffix
	if v, ok := llmSvc.Status.Annotations[key]; ok {
		return v
	}
	return name
}
