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
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/coreos/go-semver/semver"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// Configuration template name suffixes for different LLM deployment patterns
// These configs are automatically applied based on the service configuration
const (
	// Single node deployment template
	configTemplateNameSuffix = "config-llm-template"
	// Single node SGLang deployment template
	configSGLangTemplateNameSuffix = "config-sglang-template"
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
	configRouterSchedulerNameSuffix                   = "config-llm-scheduler"
	configRouterSchedulerDefaultEPPConfigNameSuffix   = "config-llm-scheduler-eppconfig-default"    // default EPPConfig
	configRouterSchedulerDefaultPDEPPConfigNameSuffix = "config-llm-scheduler-eppconfig-default-pd" // default EPPConfig for P/D
	configRouterRouteNameSuffix                       = "config-llm-router-route"
	configTokenizerNameSuffix                         = "config-llm-tokenizer" // #nosec G101
	// Tracing configurations
	configTracingNameSuffix = "config-llm-tracing"
)

var (
	configPrefix                                = constants.GetEnvOrDefault("LLM_INFERENCE_SERVICE_CONFIG_PREFIX", "kserve-")
	configTemplateName                          = configPrefix + configTemplateNameSuffix
	configSGLangTemplateName                    = configPrefix + configSGLangTemplateNameSuffix
	configDecodeTemplateName                    = configPrefix + configDecodeTemplateNameSuffix
	configDecodeWorkerPipelineParallelName      = configPrefix + configDecodeWorkerPipelineParallelNameSuffix
	configWorkerPipelineParallelName            = configPrefix + configWorkerPipelineParallelNameSuffix
	configWorkerDataParallelName                = configPrefix + configWorkerDataParallelNameSuffix
	configDecodeWorkerDataParallelName          = configPrefix + configDecodeWorkerDataParallelNameSuffix
	configPrefillTemplateName                   = configPrefix + configPrefillTemplateNameSuffix
	configPrefillWorkerPipelineParallelName     = configPrefix + configPrefillWorkerPipelineParallelNameSuffix
	configPrefillWorkerDataParallelName         = configPrefix + configPrefillWorkerDataParallelNameSuffix
	configRouterSchedulerName                   = configPrefix + configRouterSchedulerNameSuffix
	configRouterSchedulerDefaultEPPConfigName   = configPrefix + configRouterSchedulerDefaultEPPConfigNameSuffix
	configRouterSchedulerDefaultPDEPPConfigName = configPrefix + configRouterSchedulerDefaultPDEPPConfigNameSuffix
	configRouterRouteName                       = configPrefix + configRouterRouteNameSuffix
	configTokenizerName                         = configPrefix + configTokenizerNameSuffix
	configTracingName                           = configPrefix + configTracingNameSuffix
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
	configSGLangTemplateName,
	configDecodeTemplateName,
	configWorkerDataParallelName,
	configDecodeWorkerDataParallelName,
	configPrefillTemplateName,
	configPrefillWorkerDataParallelName,
	configRouterSchedulerName,
	configRouterSchedulerDefaultEPPConfigName,
	configRouterSchedulerDefaultPDEPPConfigName,
	configRouterRouteName,
	configTokenizerName,
	configTracingName,
)

const (
	precisePrefixCacheScorerName = "precise-prefix-cache-scorer"
)

// routerPresetMinVersion is the minimum llm-d-router version that supports the
// preset-based EPPConfig plugins. Services running older router images fall
// back to the hardcoded schedulerConfigText().
var routerPresetMinVersion = semver.New("0.11.0")

var useVersionedConfig, _ = strconv.ParseBool(constants.GetEnvOrDefault("LLM_INFERENCE_SERVICE_VERSIONED_CONFIG", "true"))

// SGLangServingRuntimeName is the well-known ClusterServingRuntime name shipped
// with KServe that supplies the SGLang container image. When spec.runtime is
// set to this name, the controller selects the SGLang infrastructure template
// (kserve-config-sglang-template).
//
// TODO: Longer term we want to make kserve-config-llm-template engine-agnostic
// (no image, aligned probes / volumes / security context across engines) so
// that the ServingRuntime alone drives engine selection. At that point this
// name-based mapping and kserve-config-sglang-template can be removed.
const SGLangServingRuntimeName = "kserve-llm-sglang"

// selectSingleNodeTemplateName returns the well-known config template name for
// a single-node Non-P/D deployment based on the requested runtime. When runtime
// points at the well-known SGLang ServingRuntime, the SGLang-specific
// infrastructure template is used; otherwise the default vLLM template.
func selectSingleNodeTemplateName(runtime *string) string {
	if runtime != nil && *runtime == SGLangServingRuntimeName {
		return configSGLangTemplateName
	}
	return configTemplateName
}

// presetAnnotationPrefix marks annotations a config contributes to the controller
// rather than to the rendered spec. They are collected while configs are merged
// and never reach a pod.
//
// Only configs KServe ships are read; one on a user's config is logged and
// ignored, so an internal detail does not become an API that cannot be
// withdrawn.
const presetAnnotationPrefix = "internal." + constants.KServeAPIGroupName + "/"

// routerVersionSupportsPreset reports whether the scheduler config's declared
// llm-d-router version (app.kubernetes.io/version annotation) is >=
// routerPresetMinVersion. Returns false when the annotation is absent or
// unparseable, so the caller falls back to the hardcoded schedulerConfigText().
func routerVersionSupportsPreset(ctx context.Context, cfg *v1alpha2.LLMInferenceServiceConfig) bool {
	if cfg.Spec.Router == nil || cfg.Spec.Router.Scheduler == nil {
		return false
	}
	versionStr, ok := cfg.Spec.Router.Scheduler.Annotations["app.kubernetes.io/version"]
	if !ok || versionStr == "" {
		return false
	}
	v, err := semver.NewVersion(versionStr)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to parse llm-d-router version from scheduler config", "version", versionStr)
		return false
	}
	return v.Compare(*routerPresetMinVersion) >= 0
}

// hasSchedulerEPPConfig reports whether a spec already supplies an EPPConfig for
// the scheduler, either inline via .router.scheduler.config or as a
// --config-text/--config-file flag on the "main" container. Such a config is
// user-owned, so no preset is injected on top of it.
func hasSchedulerEPPConfig(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil {
		return false
	}
	return spec.Router.Scheduler.Config != nil || hasMainContainerConfigFlag(spec)
}

// hasMainContainerConfigFlag reports whether the scheduler's "main" container
// already carries a config flag (--config-text/--config-file). If so, the user
// supplies the EPPConfig and we skip injecting a default llmisvcconfig preset.
func hasMainContainerConfigFlag(spec v1alpha2.LLMInferenceServiceSpec) bool {
	if spec.Router == nil || spec.Router.Scheduler == nil || spec.Router.Scheduler.Template == nil {
		return false
	}
	return configFlagFromContainers(spec.Router.Scheduler.Template.Containers) != nil
}

// injectLoRAAffinityScorer adds lora-affinity-scorer to the scheduler config
// so requests go to pods that already have the adapter loaded. Skips if this plugin already present.
func injectLoRAAffinityScorer(cfg *v1alpha2.LLMInferenceServiceConfig) error {
	if cfg.Spec.Router == nil || cfg.Spec.Router.Scheduler == nil ||
		cfg.Spec.Router.Scheduler.Config == nil || cfg.Spec.Router.Scheduler.Config.Inline == nil {
		return nil
	}

	epp := map[string]interface{}{}
	if err := yaml.Unmarshal(cfg.Spec.Router.Scheduler.Config.Inline.Raw, &epp); err != nil {
		return fmt.Errorf("failed to parse scheduler config for LoRA injection: %w", err)
	}

	// Append the scorer to the plugins list, skip if already there.
	plugins, _ := epp["plugins"].([]interface{})
	for _, p := range plugins {
		if m, ok := p.(map[string]interface{}); ok {
			if t, _ := m["type"].(string); t == loraAffinityScorerPlugin {
				return nil
			}
		}
	}
	epp["plugins"] = append(plugins, map[string]interface{}{"type": loraAffinityScorerPlugin})

	// Add lora-affinity-scorer with weight 4 into each scheduling profile's plugins.
	profiles, _ := epp["schedulingProfiles"].([]interface{})
	for _, pr := range profiles {
		profile, ok := pr.(map[string]interface{})
		if !ok {
			continue
		}
		entries, _ := profile["plugins"].([]interface{})
		profile["plugins"] = append(entries,
			map[string]interface{}{"pluginRef": loraAffinityScorerPlugin, "weight": int64(4)})
	}

	raw, err := json.Marshal(epp)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduler config after lora-affinity-scorer injection: %w", err)
	}
	cfg.Spec.Router.Scheduler.Config.Inline = &runtime.RawExtension{Raw: raw}
	return nil
}

// conditionMessage renders err for a status condition, dropping the "terminal
// error: " that controller-runtime prepends. That prefix describes whether the
// controller will requeue, which is not something a user can act on.
func conditionMessage(err error) string {
	// Defensive: nothing on this path returns a terminal error today, since
	// reconcileBaseRefs classifies them. Kept so a future one cannot leak the
	// prefix into a message a user reads.
	//
	// The marker can sit at any depth and unwrapping to reach it would discard the
	// context wrapped around it, so the terminal layer is located and only its own
	// rendering is swapped for its cause's - replacing the first "terminal error: "
	// anywhere in the message would also hit one that came from the spec.
	for e := err; e != nil; e = errors.Unwrap(e) {
		if !errors.Is(e, reconcile.TerminalError(nil)) {
			break
		}
		inner := errors.Unwrap(e)
		if inner == nil {
			break
		}
		if errors.Is(inner, reconcile.TerminalError(nil)) {
			continue
		}
		return strings.Replace(err.Error(), e.Error(), inner.Error(), 1)
	}
	return err.Error()
}

// reconcileBaseRefs resolves and merges the referenced configs, then checks the
// merged spec by dry-running it against the API server.
//
// A missing config, an unservable merged spec, or a spec the API server rejects
// returns reconcile.TerminalError: each is a pure function of the spec and its
// referenced configs, so retrying fixes none of them and the controller waits for a
// watch event on either instead. Any other error is returned as-is and requeued with
// backoff.
func (r *LLMISVCReconciler) reconcileBaseRefs(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) (v1alpha2.LLMInferenceServiceSpec, error) {
	// Pin the well-known config names into Status.Annotations before resolving, so that
	// resolution reads the pins this reconcile will persist. Resolve cannot do this
	// itself: it works on a copy, and a status write is the reconciler's to make.
	(&WellKnownConfigResolver{}).Attach(llmSvc)

	// Combine base configurations with service-specific overrides
	// This includes default configs based on deployment pattern (single node, multi-node, etc.)
	result, err := r.specResolver().Resolve(ctx, llmSvc, config)
	if err != nil {
		if utils.GetForceStopRuntime(llmSvc) {
			llmSvc.MarkPresetsCombinedNotReady("Stopped", "Service is stopped with warning: %v", err.Error())

			return *llmSvc.Spec.DeepCopy(), nil
		}

		llmSvc.Status.AppliedConfigRefs = nil

		// Both are raised inside SpecResolver.Resolve but classified here, because that
		// function also runs in the watch mapping handlers: an unrelated Gateway or
		// ConfigMap change would otherwise mark the condition and fire the event for a
		// service nothing is reconciling.
		var cfgNotFound *configNotFoundError
		if errors.As(err, &cfgNotFound) {
			llmSvc.MarkPresetsCombinedNotReady("ConfigNotFound", "%s", cfgNotFound.Error())
			return v1alpha2.LLMInferenceServiceSpec{}, reconcile.TerminalError(cfgNotFound)
		}

		var collision *loRAMountPathCollisionError
		if errors.As(err, &collision) {
			r.Eventf(llmSvc, corev1.EventTypeWarning, "LoRAMountPathCollision", "%s", collision.Error())
			llmSvc.MarkPresetsCombinedNotReady("LoRAMountPathCollision", "%s", collision.Error())
			return v1alpha2.LLMInferenceServiceSpec{}, reconcile.TerminalError(collision)
		}

		var slotConflict *kvTransferSlotError
		if errors.As(err, &slotConflict) {
			r.Eventf(llmSvc, corev1.EventTypeWarning, "KVTransferSlotConflict", "%s", slotConflict.Error())
			llmSvc.MarkPresetsCombinedNotReady("KVTransferSlotConflict", "%s", slotConflict.Error())
			return v1alpha2.LLMInferenceServiceSpec{}, reconcile.TerminalError(slotConflict)
		}

		llmSvc.MarkPresetsCombinedNotReady("CombineBaseError", "%s", conditionMessage(err))
		return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("failed to combine base-configurations: %w", err)
	}

	// The LLMInferenceServiceConfig CRD's OpenAPI constraints are relaxed
	// to allow Go templating. Validate the rendered spec through the
	// LLMInferenceService CRD's full admission chain: OpenAPI/CEL schema
	// rules and KServe's validating webhook.
	validationSpec := *result.Spec.DeepCopy()
	normalizeRawExtensionsToJSON(&validationSpec)
	if err = r.Create(ctx, &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: kmeta.ChildName(llmSvc.Name, "-validation-"),
			Namespace:    llmSvc.GetNamespace(),
		},
		Spec: validationSpec,
	}, client.DryRunAll); err != nil {
		if utils.GetForceStopRuntime(llmSvc) {
			llmSvc.MarkPresetsCombinedNotReady("Stopped", "Service is stopped with warning: %v", err.Error())

			return *llmSvc.Spec.DeepCopy(), nil
		}

		llmSvc.Status.AppliedConfigRefs = result.Applied

		// Anything other than Invalid means the spec was never checked: the API server
		// timed out, the webhook was unreachable, RBAC was revoked. Nothing about the
		// service changed, so no watch event will fire when it recovers - requeue
		// instead. Unknown, not False: the service may still be serving.
		if !apierrors.IsInvalid(err) {
			llmSvc.MarkPresetsCombinedUnknown("ValidationUnavailable", "%s",
				renderedConfigConditionMessage(err, result.Applied))

			return v1alpha2.LLMInferenceServiceSpec{}, fmt.Errorf("failed to dry-run validate rendered config: %w", err)
		}

		llmSvc.MarkPresetsCombinedNotReady("InvalidRenderedConfig", "%s",
			renderedConfigConditionMessage(err, result.Applied))

		return v1alpha2.LLMInferenceServiceSpec{}, reconcile.TerminalError(fmt.Errorf("rendered config rejected: %w", err))
	}

	llmSvc.Status.AppliedConfigRefs = result.Applied
	llmSvc.MarkPresetsCombinedReady()

	return result.Spec, nil
}

// normalizeRawExtensionsToJSON converts YAML-encoded RawExtension fields to
// JSON so the spec can be sent to the apiserver (which expects JSON in
// RawExtension.Raw). This is needed because scheduler Config.Ref resolution
// stores the ConfigMap value as-is (which may be YAML).
func normalizeRawExtensionsToJSON(spec *v1alpha2.LLMInferenceServiceSpec) {
	if spec.Router == nil || spec.Router.Scheduler == nil ||
		spec.Router.Scheduler.Config == nil || spec.Router.Scheduler.Config.Inline == nil {
		return
	}
	raw := spec.Router.Scheduler.Config.Inline.Raw
	if len(raw) > 0 && raw[0] != '{' {
		if jsonBytes, err := yaml.YAMLToJSON(raw); err == nil {
			spec.Router.Scheduler.Config.Inline.Raw = jsonBytes
		}
	}
}

func renderedConfigConditionMessage(err error, refs []v1alpha2.AppliedConfigRef) string {
	refNames := make([]string, 0, len(refs))
	for _, ref := range refs {
		refNames = append(refNames, fmt.Sprintf("%s:%s/%s", ref.Source, ref.Namespace, ref.Name))
	}

	return fmt.Sprintf("dry-run validation failed after merging configs [%s]: %s",
		strings.Join(refNames, ", "), validationDetail(err))
}

// validationDetail lists the rejected fields from a validation error. The full
// error text is only a fallback: it starts with the name of the throwaway object
// the dry-run creates, which the user never wrote and cannot look up.
func validationDetail(err error) string {
	var statusErr apierrors.APIStatus
	if !errors.As(err, &statusErr) {
		return err.Error()
	}

	details := statusErr.Status().Details
	if details == nil || len(details.Causes) == 0 {
		return err.Error()
	}

	msgs := make([]string, 0, len(details.Causes))
	for _, cause := range details.Causes {
		if cause.Field == "" {
			msgs = append(msgs, cause.Message)
			continue
		}
		msgs = append(msgs, fmt.Sprintf("%s: %s", cause.Field, cause.Message))
	}

	return strings.Join(msgs, "; ")
}

// configRef is one config to merge, carrying who asked for it. Provenance is
// recorded where the ref is created rather than recovered from its position
// afterwards: a ref appended on the wrong side of a boundary would silently
// promote a user's config to a preset, and presets are trusted with reserved
// annotations and with owning the sizes the controller computes.
type configRef struct {
	corev1.LocalObjectReference
	source v1alpha2.AppliedConfigSource
}

// presetRef is a config KServe selected for this service.
func presetRef(name string) configRef {
	return configRef{
		LocalObjectReference: corev1.LocalObjectReference{Name: name},
		source:               v1alpha2.AppliedConfigSourcePreset,
	}
}

// userRef is a config the service asked for through spec.baseRefs.
func userRef(ref corev1.LocalObjectReference) configRef {
	return configRef{LocalObjectReference: ref, source: v1alpha2.AppliedConfigSourceUserRef}
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
// Only matches naming headerName are duplicated; path-only rules and rules with
// unrelated headers are left untouched.
func expandLoRAAdapterMatches(rules []gwapiv1.HTTPRouteRule, namespace string, adapters []v1alpha2.LLMModelSpec, headerName string) {
	if headerName == "" || len(adapters) == 0 {
		return
	}

	sorted := make([]v1alpha2.LLMModelSpec, len(adapters))
	copy(sorted, adapters)
	slices.SortFunc(sorted, func(a, b v1alpha2.LLMModelSpec) int {
		return strings.Compare(ptr.Deref(a.Name, ""), ptr.Deref(b.Name, ""))
	})

	for i := range rules {
		var adapterMatches []gwapiv1.HTTPRouteMatch
		for _, match := range rules[i].Matches {
			if !isModelBasedRoutingMatch(match, headerName) {
				continue
			}
			for _, adapter := range sorted {
				if adapter.Name == nil {
					continue
				}
				am := *match.DeepCopy()
				for h := range am.Headers {
					if isModelRoutingHeader(am.Headers[h].Name, headerName) {
						am.Headers[h].Value = fullyQualifiedModelName(namespace, *adapter.Name)
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

// templateFuncs are the functions a LLMInferenceServiceConfig preset may call.
//
// Every entry is a public contract: a pinned preset is re-rendered by whatever
// controller version is installed, so changing a signature or an output here
// rewrites the pod spec of untouched workloads and restarts them.
//
// Retire an entry by moving it to deprecatedTemplateFuncs, not by editing or
// deleting it in place.
var templateFuncs = map[string]any{
	"ChildName": kmeta.ChildName,
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
}

// deprecatedTemplateFuncs are frozen. No preset in config/llmisvcconfig calls
// them any more, but presets pinned by running services still do, and
// text/template rejects an unknown function at parse time - so removing one does
// not degrade those services, it breaks them outright.
//
// The rules for this map:
//
//   - Do not change a signature, an output, or an escaping rule. Whatever an
//     entry emitted when it was deprecated is the contract now, bugs included.
//   - Do not delete an entry while any supported release could still have a
//     preset pinned against it.
//   - Fix the bug in the replacement, never here.
//
// Each entry records what replaced it.
var deprecatedTemplateFuncs = map[string]any{
	// kvTransferConfig: replaced by the kvTransferArgsEnvVar slot, which the
	// controller fills in after rendering. The unconditional TieringOffloadingSpec
	// below is the bug that slot exists to fix, and it stays - a pinned preset that
	// rendered it must keep rendering it.
	//
	// The body is a copy of what shipped, deliberately not a call into the live
	// path: sharing a helper with the carrier once let a guard added for the new
	// path change what this renders.
	"kvTransferConfig": func(spec any) string {
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
		var secondaryTiers []map[string]any
		for i, s := range kv.Secondary {
			if s.FileSystem == nil {
				continue
			}
			secondaryTiers = append(secondaryTiers, map[string]any{
				"type":     "fs",
				"root_dir": fmt.Sprintf("/mnt/kv-cache-%d", i),
			})
		}
		if len(secondaryTiers) > 0 {
			extraConfig["secondary_tiers"] = secondaryTiers
		}
		b, err := json.Marshal(map[string]any{
			"kv_connector":              "OffloadingConnector",
			"kv_role":                   "kv_both",
			"kv_connector_extra_config": extraConfig,
		})
		if err != nil {
			return ""
		}
		// \\\" decodes to \" after ReplaceVariables re-unmarshals this as JSON, and
		// the \" then survives the KV_TRANSFER_ARGS="..." bash assignment in the
		// template. Plain " would be eaten by the shell and vLLM would get invalid JSON.
		return "--kv-transfer-config '" + strings.ReplaceAll(string(b), `"`, `\\\"`) + "'"
	},
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
		Funcs(templateFuncs).
		Funcs(deprecatedTemplateFuncs).
		Option("missingkey=error").
		Parse(string(templateBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template config: %w", err)
	}
	if err := t.Execute(buf, config); err != nil {
		return nil, fmt.Errorf("failed to merge config: %w", err)
	}

	out := &v1alpha2.LLMInferenceServiceConfig{}
	if err := json.Unmarshal(buf.Bytes(), out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from template: %w", err)
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
