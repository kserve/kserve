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
	"fmt"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	configPrefix                            = "kserve-"
	configTemplateName                      = configPrefix + "config-llm-template"
	configDecodeTemplateName                = configPrefix + "config-llm-decode-template"
	configDecodeWorkerPipelineParallelName  = configPrefix + "config-llm-decode-worker-pipeline-parallel"
	configWorkerPipelineParallelName        = configPrefix + "config-llm-worker-pipeline-parallel"
	configWorkerDataParallelName            = configPrefix + "config-llm-worker-data-parallel"
	configDecodeWorkerDataParallelName      = configPrefix + "config-llm-decode-worker-data-parallel"
	configPrefillTemplateName               = configPrefix + "config-llm-prefill-template"
	configPrefillWorkerPipelineParallelName = configPrefix + "config-llm-prefill-worker-pipeline-parallel"
	configPrefillWorkerDataParallelName     = configPrefix + "config-llm-prefill-worker-data-parallel"
	configRouterSchedulerName               = configPrefix + "config-llm-scheduler"
	configRouterRouteName                   = configPrefix + "config-llm-router-route"
)

// FIXME move those presets to well-known when they're finally known :)
var _ = sets.New[string](
	configPrefillWorkerPipelineParallelName,
	configDecodeWorkerPipelineParallelName,
	configWorkerPipelineParallelName,
)

var WellKnownDefaultConfigs = sets.New[string](
	configTemplateName,
	configDecodeTemplateName,
	configWorkerDataParallelName,
	configDecodeWorkerDataParallelName,
	configPrefillTemplateName,
	configPrefillWorkerDataParallelName,
	configRouterSchedulerName,
	configRouterRouteName,
)

// combineBaseRefsConfig applies well-known config overlays to inject default values for various components, when some components are
// enabled. These LLMInferenceServiceConfig resources must exist in either resource namespace (prioritized) or
// SystemNamespace (e.g. `kserve`).
func (r *LLMISVCReconciler) combineBaseRefsConfig(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, reconcilerConfig *Config) (*v1alpha1.LLMInferenceServiceConfig, error) {
	logger := log.FromContext(ctx).WithName("combineBaseRefsConfig")

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
		refs = append(refs, corev1.LocalObjectReference{Name: configRouterSchedulerName})
	}
	if resolvedSpec.Router != nil && resolvedSpec.Router.Route != nil && !resolvedSpec.Router.Route.HTTP.HasRefs() {
		refs = append(refs, corev1.LocalObjectReference{Name: configRouterRouteName})
	}

	if resolvedSpec.Prefill != nil { // P/D
		// Prefill
		switch {
		case resolvedSpec.Prefill.Worker == nil:
			// single-node prefill
			refs = append(refs, corev1.LocalObjectReference{Name: configPrefillTemplateName})
		case resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsDataParallel():
			// multi-node Data Parallel prefill
			refs = append(refs, corev1.LocalObjectReference{Name: configPrefillWorkerDataParallelName})
		case resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel prefill
			refs = append(refs, corev1.LocalObjectReference{Name: configPrefillWorkerPipelineParallelName})
		}
		// Decode
		switch {
		case resolvedSpec.Worker == nil:
			// single-node decode
			refs = append(refs, corev1.LocalObjectReference{Name: configDecodeTemplateName})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsDataParallel():
			// multi-node Data Parallel decode
			refs = append(refs, corev1.LocalObjectReference{Name: configDecodeWorkerDataParallelName})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel decode
			refs = append(refs, corev1.LocalObjectReference{Name: configDecodeWorkerPipelineParallelName})
		}
	} else { // Non P/D
		switch {
		case resolvedSpec.Worker == nil:
			// single-node
			refs = append(refs, corev1.LocalObjectReference{Name: configTemplateName})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsDataParallel():
			// multi-node Data Parallel
			refs = append(refs, corev1.LocalObjectReference{Name: configWorkerDataParallelName})
		case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsPipelineParallel():
			// multi-node Pipeline Parallel
			refs = append(refs, corev1.LocalObjectReference{Name: configWorkerPipelineParallelName})
		}
	}

	// Append explicit base refs to override well know configs.
	refs = append(refs, llmSvc.Spec.BaseRefs...)

	specs := make([]v1alpha1.LLMInferenceServiceSpec, 0, len(refs))
	for _, ref := range refs {
		cfg, err := r.getConfig(ctx, llmSvc, ref.Name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			specs = append(specs, cfg.Spec)
		}
	}
	spec, err := MergeSpecs(ctx, append(specs, llmSvc.Spec)...)
	if err != nil {
		return nil, fmt.Errorf("failed to merge specs: %w", err)
	}

	llmSvcCfg := &v1alpha1.LLMInferenceServiceConfig{
		ObjectMeta: *llmSvc.ObjectMeta.DeepCopy(),
		Spec:       spec,
	}

	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Scheduler != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Pool != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Pool.Spec != nil &&
		len(llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.Selector) == 0 {
		selector := getInferencePoolWorkloadLabelSelector(llmSvc.ObjectMeta, &llmSvcCfg.Spec)

		gieSelector := make(map[igwapi.LabelKey]igwapi.LabelValue, len(selector))
		for k, v := range selector {
			gieSelector[igwapi.LabelKey(k)] = igwapi.LabelValue(v)
		}
		llmSvcCfg.Spec.Router.Scheduler.Pool.Spec.Selector = gieSelector
	}

	if llmSvcCfg.Spec.Router != nil &&
		llmSvcCfg.Spec.Router.Scheduler != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Template != nil &&
		llmSvcCfg.Spec.Router.Scheduler.Template.ServiceAccountName == "" {
		llmSvcCfg.Spec.Router.Scheduler.Template.ServiceAccountName = kmeta.ChildName(llmSvc.GetName(), "-epp-sa")
	}

	llmSvcCfg, err = ReplaceVariables(llmSvc, llmSvcCfg, reconcilerConfig)
	if err != nil {
		return llmSvcCfg, err
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
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Group = ptr.To[gatewayapi.Group]("")
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Kind = ptr.To[gatewayapi.Kind]("Service")
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Name = gatewayapi.ObjectName(kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"))
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
					llmSvcCfg.Spec.Router.Route.HTTP.Spec.Rules[i].BackendRefs[j].Name = gatewayapi.ObjectName(llmSvcCfg.Spec.Router.Scheduler.Pool.Ref.Name)
				}
			}
		}
	}

	return llmSvcCfg, nil
}

func ReplaceVariables(llmSvc *v1alpha1.LLMInferenceService, llmSvcCfg *v1alpha1.LLMInferenceServiceConfig, reconcilerConfig *Config) (*v1alpha1.LLMInferenceServiceConfig, error) {
	templateBytes, _ := json.Marshal(llmSvcCfg)
	buf := bytes.NewBuffer(nil)
	config := struct {
		*v1alpha1.LLMInferenceService
		GlobalConfig *Config
	}{
		LLMInferenceService: llmSvc,
		GlobalConfig:        reconcilerConfig,
	}
	t, err := template.New("config").
		Funcs(map[string]any{
			"ChildName": kmeta.ChildName,
		}).
		Option("missingkey=error").
		Parse(string(templateBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template config: %w", err)
	}
	if err := t.Execute(buf, config); err != nil {
		return nil, fmt.Errorf("failed to merge config: %w", err)
	}

	out := &v1alpha1.LLMInferenceServiceConfig{}
	if err := json.Unmarshal(buf.Bytes(), out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from template: %w", err)
	}
	return out, nil
}

// getConfig retrieves kserveapis.LLMInferenceServiceConfig with the given name from either the kserveapis.LLMInferenceService
// namespace or from the SystemNamespace (e.g. 'kserve'), prioritizing the former.
func (r *LLMISVCReconciler) getConfig(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, name string) (*v1alpha1.LLMInferenceServiceConfig, error) {
	cfg := &v1alpha1.LLMInferenceServiceConfig{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: name, Namespace: llmSvc.Namespace}, cfg); err != nil {
		if apierrors.IsNotFound(err) {
			cfg = &v1alpha1.LLMInferenceServiceConfig{}
			if err := r.Client.Get(ctx, client.ObjectKey{Name: name, Namespace: constants.KServeNamespace}, cfg); err != nil {
				// TODO: add available LLMInferenceServiceConfig in system namespace and llmSvc.Namespace namespace if not found

				return nil, fmt.Errorf("failed to get LLMInferenceServiceConfig %q from namespaces [%q, %q]: %w", name, llmSvc.Namespace, constants.KServeNamespace, err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to get LLMInferenceServiceConfig %s/%s: %w", llmSvc.Namespace, name, err)
	}
	return cfg, nil
}

func MergeSpecs(ctx context.Context, cfgs ...v1alpha1.LLMInferenceServiceSpec) (v1alpha1.LLMInferenceServiceSpec, error) {
	if len(cfgs) == 0 {
		return v1alpha1.LLMInferenceServiceSpec{}, nil
	}

	out := cfgs[0]
	for i := 1; i < len(cfgs); i++ {
		cfg := cfgs[i]
		var err error
		out, err = mergeSpecs(ctx, out, cfg)
		if err != nil {
			return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("failed to merge specs: %w", err)
		}
	}
	return out, nil
}

// mergeSpecs performs a strategic merge by creating a clean patch from the override
// object and applying it to the base object.
func mergeSpecs(ctx context.Context, base, override v1alpha1.LLMInferenceServiceSpec) (v1alpha1.LLMInferenceServiceSpec, error) {
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not marshal base spec: %w", err)
	}

	// To create a patch containing only the fields specified in the override,
	// we create a patch between a zero-valued ("empty") object and the override object.
	// This prevents zero-valued fields in the override struct (e.g., an empty string for an
	// unspecified image) from incorrectly wiping out values from the base.
	zero := v1alpha1.LLMInferenceServiceSpec{}
	zeroJSON, err := json.Marshal(zero)
	if err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not marshal zero spec: %w", err)
	}

	override.SetDefaults(ctx)

	overrideJSON, err := json.Marshal(override)
	if err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not marshal override spec: %w", err)
	}

	logger := log.FromContext(ctx)

	// Create the patch. It will only contain the non-default fields from the override.
	patch, err := strategicpatch.CreateTwoWayMergePatch(zeroJSON, overrideJSON, v1alpha1.LLMInferenceServiceSpec{})
	if err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not create merge patch from override: %w", err)
	}

	logger.V(2).Info("merging specs (patch)", "patch", string(patch), "base", string(baseJSON), "override", string(overrideJSON), "zero", string(zeroJSON))

	// Apply this "clean" patch to the base JSON. The strategic merge logic will correctly
	// merge lists and objects based on their Kubernetes patch strategy annotations.
	mergedJSON, err := strategicpatch.StrategicMergePatch(baseJSON, patch, v1alpha1.LLMInferenceServiceSpec{})
	if err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not apply merge patch: %w", err)
	}

	// Unmarshal the merged JSON back into a Go struct.
	var finalSpec v1alpha1.LLMInferenceServiceSpec
	if err := json.Unmarshal(mergedJSON, &finalSpec); err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not unmarshal merged spec: %w", err)
	}
	return finalSpec, nil
}

func isDefaultBackendRef(llmSvc *v1alpha1.LLMInferenceService, ref gatewayapi.BackendRef) bool {
	defaultInfPoolName := (&v1alpha1.SchedulerSpec{}).InferencePoolName(llmSvc)
	return ptr.Deref[gatewayapi.Group](ref.Group, "") == igwapi.GroupName &&
		ptr.Deref[gatewayapi.Kind](ref.Kind, "") == "InferencePool" &&
		string(ref.Name) == defaultInfPoolName
}
