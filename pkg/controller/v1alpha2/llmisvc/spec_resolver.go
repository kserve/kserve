/*
Copyright 2026 The KServe Authors.

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
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
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

// SpecResolver resolves one LLMInferenceService to the spec that actually gets applied.
//
// It only reads. Status writes, condition marks and the dry-run admission check stay in
// reconcileBaseRefs, because a metrics scrape or a watch mapper also calls Resolve and
// must not perform them.
type SpecResolver struct {
	// Client normally reads through the manager cache.
	Client client.Client

	// Clientset reads ConfigMaps the manager cache excludes. Required: the fallback
	// dereferences it on every cache miss for a scheduler Config.Ref.
	Clientset kubernetes.Interface

	// Recorder serves one deprecation warning and nothing else.
	// TODO: a deprecated spec field is standing state, not an event - surface it as an
	// advisory condition and this field goes away.
	Recorder record.EventRecorder
}

func (r *LLMISVCReconciler) specResolver() *SpecResolver {
	return &SpecResolver{
		Client:    r.Client,
		Clientset: r.Clientset,
		Recorder:  r.EventRecorder,
	}
}

// EffectiveSpec is returned only on success: a non-nil error always comes with a nil
// result, so there is no half-resolved spec to mistake for a whole one.
type EffectiveSpec struct {
	Spec v1alpha2.LLMInferenceServiceSpec
	// Applied is ordered, and tagged by where each config came from.
	Applied []v1alpha2.AppliedConfigRef
	// SchedulerConfigMap survives because resolution clears Config.Ref once it inlines
	// it; this is the only record left of which ConfigMap was read.
	SchedulerConfigMap *types.NamespacedName
}

// Resolve applies well-known config overlays to inject default values for various components, when some components are
// enabled. These LLMInferenceServiceConfig resources must exist in either resource namespace (prioritized) or
// SystemNamespace (e.g. `kserve`).
// It determines which deployment pattern is being used (single node, multi-node, disaggregated) and applies appropriate defaults.
func (s *SpecResolver) Resolve(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, reconcilerConfig *Config) (*EffectiveSpec, error) {
	logger := log.FromContext(ctx).WithName("specResolver")

	// Resolution rewrites the spec it merges, and a caller that is not reconciling hands
	// in an object it does not own.
	llmSvc = llmSvc.DeepCopy()

	// Reads the pins; writing them is Attach, which is a status write and so stays with
	// the reconciler. Unpinned resolves to the current name, which is what an
	// unreconciled service uses anyway.
	wr := &WellKnownConfigResolver{}

	baseRefSpecs := make([]v1alpha2.LLMInferenceServiceSpec, 0, len(llmSvc.Spec.BaseRefs))
	for _, ref := range llmSvc.Spec.BaseRefs {
		cfg, err := s.getConfig(ctx, llmSvc, ref.Name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			baseRefSpecs = append(baseRefSpecs, cfg.Spec)
		}
	}

	// Applied with baseRefs last, so a config that supplies a field replaces the
	// service's own. That order is what the model name copy-back below relies on:
	// webhook defaulting always leaves one set on the service, so a config that
	// supplies one could not otherwise take effect. The spec that gets applied
	// merges the other way round, with the service last.
	resolvedSpec, err := MergeSpecs(ctx, append([]v1alpha2.LLMInferenceServiceSpec{*llmSvc.Spec.DeepCopy()}, baseRefSpecs...)...)
	if err != nil {
		return nil, fmt.Errorf("failed to merge specs: %w", err)
	}

	if resolvedSpec.Model.Name != nil {
		// If original model name was defaulted check if it was not substituted by baseRef
		llmSvc.Spec.Model.Name = resolvedSpec.Model.Name
	}

	logger.V(2).Info("Resolved spec", "spec", resolvedSpec)

	refs := make([]configRef, 0, len(llmSvc.Spec.BaseRefs))

	// Check if user provided a customized config in llmisvc then inject EPPConfig accordingly
	// we only mutate configs we generated.
	injectDefaultSchedulerConfig := resolvedSpec.Router != nil &&
		resolvedSpec.Router.Scheduler != nil &&
		!hasSchedulerEPPConfig(resolvedSpec)

	if resolvedSpec.Router != nil && resolvedSpec.Router.Scheduler != nil && !resolvedSpec.Router.Scheduler.Pool.HasRef() {
		// Set router pod deployment
		schedulerConfigName := wr.Resolve(llmSvc, configRouterSchedulerName)
		refs = append(refs, presetRef(schedulerConfigName))

		if injectDefaultSchedulerConfig {
			schedulerCfg, err := s.getConfig(ctx, llmSvc, schedulerConfigName)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve scheduler config %q: %w", schedulerConfigName, err)
			}

			// resolvedSpec covers .spec plus the user's baseRefs only - the
			// well-known scheduler config is merged further down, so an EPPConfig
			// supplied there (by an admin customizing the preset) is invisible to
			// the check above. It is still user-owned: injecting on top of it would
			// make preserveSchedulerConfig append a second --config-text and
			// silently shadow it.
			if hasSchedulerEPPConfig(schedulerCfg.Spec) {
				injectDefaultSchedulerConfig = false
			}

			// Select the default EndpointPickerConfig from the llmisvcconfig presets.
			// The presets require llm-d-router image version >= routerPresetMinVersion.
			// Older images fall back to the hardcoded schedulerConfigText().
			if injectDefaultSchedulerConfig && routerVersionSupportsPreset(ctx, schedulerCfg) {
				if resolvedSpec.Prefill != nil { // P/D disagg.
					refs = append(refs, presetRef(wr.Resolve(llmSvc, configRouterSchedulerDefaultPDEPPConfigName)))
				} else {
					refs = append(refs, presetRef(wr.Resolve(llmSvc, configRouterSchedulerDefaultEPPConfigName)))
				}
			}
		}
	}

	if resolvedSpec.Router != nil && resolvedSpec.Router.Scheduler != nil && isTokenizerEnabled(resolvedSpec) {
		refs = append(refs, presetRef(wr.Resolve(llmSvc, configTokenizerName)))
	}
	if resolvedSpec.Router != nil && resolvedSpec.Router.Route != nil && !resolvedSpec.Router.Route.HTTP.HasRefs() {
		// For the HTTP route configuration we don't use versioned defaults since this configuration depends on the
		// GW API provider version.
		refs = append(refs, presetRef(configRouterRouteName))
	}
	// Inject tracing default configs when tracing is enabled (field is non-nil)
	if resolvedSpec.Tracing != nil {
		refs = append(refs, presetRef(wr.Resolve(llmSvc, configTracingName)))
	}

	if resolvedSpec.Prefill != nil { // P/D
		// Prefill
		switch {
		case resolvedSpec.Prefill.Worker == nil:
			// single-node prefill
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configPrefillTemplateName)))
		case resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsDataParallel():
			// multi-node Data Parallel prefill
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configPrefillWorkerDataParallelName)))
		case resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel prefill
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configPrefillWorkerPipelineParallelName)))
		}
		// Decode
		switch {
		case resolvedSpec.Worker == nil:
			// single-node decode
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configDecodeTemplateName)))
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsDataParallel():
			// multi-node Data Parallel decode
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configDecodeWorkerDataParallelName)))
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel decode
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configDecodeWorkerPipelineParallelName)))
		}
	} else { // Non P/D
		switch {
		case resolvedSpec.Worker == nil:
			// single-node -- select template based on runtime
			refs = append(refs, presetRef(wr.Resolve(llmSvc, selectSingleNodeTemplateName(resolvedSpec.Runtime))))
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsDataParallel():
			// multi-node Data Parallel
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configWorkerDataParallelName)))
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel
			refs = append(refs, presetRef(wr.Resolve(llmSvc, configWorkerPipelineParallelName)))
		}
	}

	// Append explicit base refs to override well know configs.
	for _, ref := range llmSvc.Spec.BaseRefs {
		refs = append(refs, userRef(ref))
	}

	specs := make([]v1alpha2.LLMInferenceServiceSpec, 0, len(refs)+1)
	appliedRefs := make([]v1alpha2.AppliedConfigRef, 0, len(refs)+1)

	// Prepend the ServingRuntime/ClusterServingRuntime container spec (if spec.runtime
	// resolves) as the lowest-priority merge layer. This gives operators one place —
	// the runtime resource — to pin the engine image while leaving every downstream
	// layer (well-known configs, user baseRefs, service spec.template) free to
	// override it.
	runtimeSpec, err := s.resolveRuntimeSpec(ctx, llmSvc)
	if err != nil {
		return nil, err
	}
	if runtimeSpec != nil {
		specs = append(specs, *runtimeSpec)
		appliedRefs = append(appliedRefs, v1alpha2.AppliedConfigRef{
			Name:   gwapiv1.ObjectName(*llmSvc.Spec.Runtime),
			Source: v1alpha2.AppliedConfigSourceServingRuntime,
		})
	}

	shmSizing := kvCacheShmSizing{}
	for _, ref := range refs {
		cfg, err := s.getConfig(ctx, llmSvc, ref.Name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			specs = append(specs, cfg.Spec)
			// getConfig prefers the service's namespace, so a copy a user put there
			// answers to a well-known name. Being asked for by KServe is not enough;
			// the object must also be one KServe ships.
			source := ref.source
			if cfg.Namespace != constants.KServeNamespace {
				source = v1alpha2.AppliedConfigSourceUserRef
			}
			policy := kvCacheShmPolicy{userDeclared: source == v1alpha2.AppliedConfigSourceUserRef, declaredBy: ref.Name}
			var reserved map[string]string
			utils.PropagatePrefixedMap(cfg.Annotations, &reserved, presetAnnotationPrefix)
			switch {
			case len(reserved) == 0:
			case source == v1alpha2.AppliedConfigSourceUserRef:
				logger.Info("Ignoring reserved annotations on a user-supplied config",
					"config", ref.Name, "annotations", slices.Sorted(maps.Keys(reserved)))
			default:
				raw := reserved[shmPercentOfCPUAnnotation]
				if percent, ok := parseShmPercentOfCPU(raw); ok {
					policy.percentOfCPU = percent
				} else if raw != "" {
					logger.Info("Ignoring unusable KV cache shared-memory headroom percentage; leaving the declared size alone",
						"config", ref.Name, "annotation", shmPercentOfCPUAnnotation, "value", raw)
				}
			}
			shmSizing.record(cfg, policy)
			appliedRefs = append(appliedRefs, v1alpha2.AppliedConfigRef{
				Name:      gwapiv1.ObjectName(ref.Name),
				Namespace: gwapiv1.Namespace(cfg.Namespace),
				Source:    source,
			})
		}
	}
	shmSizing.record(&v1alpha2.LLMInferenceServiceConfig{Spec: llmSvc.Spec},
		kvCacheShmPolicy{userDeclared: true, declaredBy: "the service spec"})
	spec, err := MergeSpecs(ctx, append(specs, llmSvc.Spec)...)
	if err != nil {
		return nil, fmt.Errorf("failed to merge specs: %w", err)
	}

	llmSvcCfg := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: *llmSvc.ObjectMeta.DeepCopy(),
		Spec:       spec,
	}
	var resolvedSchedulerConfigMap *types.NamespacedName

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

	// Render against the merged spec so templates see the values that get applied.
	// Shallow copies suffice: ReplaceVariables only reads the service.
	effectiveSvc := *llmSvc
	effectiveSvc.Spec = llmSvcCfg.Spec

	// Assign only on success: ReplaceVariables returns nil on failure, and reassigning
	// llmSvcCfg from it would leave every error return below dereferencing nil. The
	// pre-render spec is not worth handing back either - it is still full of unexpanded
	// template literals - so a render failure reports how far resolution got and no spec.
	rendered, err := ReplaceVariables(&effectiveSvc, llmSvcCfg, reconcilerConfig)
	if err != nil {
		return nil, err
	}
	llmSvcCfg = rendered

	// Add the lora-affinity-scorer to the scheduler config only when
	// .spec.model.lora.adapters is set and the user did not supply their own
	// scheduler config (via .spec.router.scheduler.config or a config flag in
	// the scheduler template args).
	if injectDefaultSchedulerConfig && resolvedSpec.Model.LoRA != nil && len(resolvedSpec.Model.LoRA.Adapters) > 0 {
		if err := injectLoRAAffinityScorer(llmSvcCfg); err != nil {
			return nil, err
		}
	}

	if declared := applyKVCacheShmSizing(llmSvcCfg, shmSizing); len(declared) > 0 {
		logger.Info("Shared-memory size is declared in the spec; leaving it as written and not adding the KV cache tier",
			"workloads", declared, "mountPath", sharedMemoryMountPath)
	}

	if err := applyKVCacheCarriers(llmSvcCfg); err != nil {
		return nil, err
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
		cm, err := Get(ctx, s.Client, client.ObjectKey{Namespace: llmSvc.GetNamespace(), Name: cmName}, &corev1.ConfigMap{}, WithGetFallbackAPIServerConfigMap(s.Clientset))
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w", llmSvc.GetNamespace(), cmName, err)
			}

			if strings.HasPrefix(cmName, "config-scheduler-") {
				cm, err = Get(ctx, s.Client, client.ObjectKey{Namespace: constants.KServeNamespace, Name: cmName}, &corev1.ConfigMap{}, WithGetFallbackAPIServerConfigMap(s.Clientset))
				if err != nil {
					return nil, fmt.Errorf("failed to get scheduler config %q from namespaces [%q, %q]: %w", cmName, llmSvc.Namespace, constants.KServeNamespace, err)
				}
			}
		}
		if llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key == "" {
			llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key = "epp"
		}
		cfg, ok := cm.Data[llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key]
		if !ok {
			return nil, fmt.Errorf("ConfigMap %s/%s doesn't have key %q in data",
				cm.GetNamespace(),
				cm.GetName(),
				llmSvcCfg.Spec.Router.Scheduler.Config.Ref.Key,
			)
		}
		resolvedSchedulerConfigMap = &types.NamespacedName{
			Namespace: cm.GetNamespace(),
			Name:      cm.GetName(),
		}
		llmSvcCfg.Spec.Router.Scheduler.Config.Inline = &runtime.RawExtension{Raw: []byte(cfg)}
		// Clear the Ref since it has been resolved to Inline; the two fields are
		// mutually exclusive in a valid LLMInferenceService.
		llmSvcCfg.Spec.Router.Scheduler.Config.Ref = nil

		// If predicted-latency-producer plugin is still in use, emit Event on Warning
		if hasPluginInSpec(llmSvcCfg.Spec, "predicted-latency-producer") {
			if s.Recorder != nil {
				s.Recorder.Eventf(llmSvc, corev1.EventTypeWarning, "LatencyPredictorConfigRef",
					"predicted-latency-producer plugin is deprecated, should be removed to avoid disruptions in the future")
			}
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

	// Validate the LoRA adapters the merged spec declares. An unusable adapter - an oci://
	// URI, an unsupported scheme, a mount-path collision - has to surface here so it
	// reports as PresetsCombined rather than as a workload failure several steps later.
	// The list itself is not carried: it is a pure function of the merged spec, so the
	// workload builders derive it from the same spec rather than from a cached copy.
	if _, err := enumerateLoRAAdapters(llmSvcCfg.Spec); err != nil {
		return nil, fmt.Errorf("failed to enumerate LoRA adapters: %w", err)
	}

	return &EffectiveSpec{
		Spec:               llmSvcCfg.Spec,
		Applied:            appliedRefs,
		SchedulerConfigMap: resolvedSchedulerConfigMap,
	}, nil
}

// getConfig retrieves kserveapis.LLMInferenceServiceConfig with the given name from either the kserveapis.LLMInferenceService
// namespace or from the SystemNamespace (e.g. 'kserve'), prioritizing the former.
// This allows for both global default configs and service-specific overrides.
func (s *SpecResolver) getConfig(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, name string) (*v1alpha2.LLMInferenceServiceConfig, error) {
	cfg := &v1alpha2.LLMInferenceServiceConfig{}
	if err := s.Client.Get(ctx, client.ObjectKey{Name: name, Namespace: llmSvc.Namespace}, cfg); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get LLMInferenceServiceConfig %s/%s: %w", llmSvc.Namespace, name, err)
		}
		cfg = &v1alpha2.LLMInferenceServiceConfig{}
		if err := s.Client.Get(ctx, client.ObjectKey{Name: name, Namespace: constants.KServeNamespace}, cfg); err != nil {
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
