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

	"github.com/kserve/kserve/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
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
	// Creates the initial spec with the merged BaseRefs, so that we know what's "Enabled".
	resolvedSpec := *llmSvc.Spec.DeepCopy()
	for _, ref := range llmSvc.Spec.BaseRefs {
		cfg, err := r.getConfig(ctx, llmSvc, ref.Name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			var resolvedErr error
			resolvedSpec, resolvedErr = mergeSpecs(resolvedSpec, cfg.Spec)
			if resolvedErr != nil {
				return nil, fmt.Errorf("failed to merge specs: %w", resolvedErr)
			}
		}
	}

	if resolvedSpec.Model.Name != nil {
		// If original model name was defaulted check if it was not substituted by baseRef
		llmSvc.Spec.Model.Name = resolvedSpec.Model.Name
	}

	refs := make([]corev1.LocalObjectReference, 0, len(llmSvc.Spec.BaseRefs))
	if resolvedSpec.Router != nil && resolvedSpec.Router.Scheduler != nil && !resolvedSpec.Router.Scheduler.Pool.HasRef() {
		refs = append(refs, corev1.LocalObjectReference{Name: configRouterSchedulerName})
	}
	if resolvedSpec.Router != nil && resolvedSpec.Router.Route != nil && !resolvedSpec.Router.Route.HTTP.HasRefs() {
		refs = append(refs, corev1.LocalObjectReference{Name: configRouterRouteName})
	}
	switch {
	// Disaggregated prefill and decode (P/D) cases.
	case resolvedSpec.Prefill != nil && resolvedSpec.Prefill.Worker == nil:
		refs = append(refs, corev1.LocalObjectReference{Name: configPrefillTemplateName})
		refs = append(refs, corev1.LocalObjectReference{Name: configDecodeTemplateName})
	case resolvedSpec.Prefill != nil && resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsPipelineParallel():
		refs = append(refs, corev1.LocalObjectReference{Name: configDecodeWorkerPipelineParallelName})
		refs = append(refs, corev1.LocalObjectReference{Name: configPrefillWorkerPipelineParallelName})
	case resolvedSpec.Prefill != nil && resolvedSpec.Prefill.Worker != nil && resolvedSpec.Prefill.Parallelism.IsDataParallel():
		refs = append(refs, corev1.LocalObjectReference{Name: configDecodeWorkerDataParallelName})
		refs = append(refs, corev1.LocalObjectReference{Name: configPrefillWorkerDataParallelName})
	// Multi Node without Disaggregated prefill and decode (P/D) cases.
	case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsPipelineParallel():
		refs = append(refs, corev1.LocalObjectReference{Name: configWorkerPipelineParallelName})
	case resolvedSpec.Worker != nil && resolvedSpec.Parallelism.IsDataParallel():
		refs = append(refs, corev1.LocalObjectReference{Name: configWorkerDataParallelName})
	default:
		// Single Node case.
		refs = append(refs, corev1.LocalObjectReference{Name: configTemplateName})
	}
	// Append explicit base refs to override well know configs.
	refs = append(refs, llmSvc.Spec.BaseRefs...)

	specs := make([]v1alpha1.LLMInferenceServiceSpec, 0, len(llmSvc.Spec.BaseRefs)+1)
	for _, ref := range refs {
		cfg, err := r.getConfig(ctx, llmSvc, ref.Name)
		if err != nil {
			return nil, err
		}
		if cfg != nil {
			specs = append(specs, cfg.Spec)
		}
	}
	spec, err := MergeSpecs(append(specs, llmSvc.Spec)...)
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


func MergeSpecs(cfgs ...v1alpha1.LLMInferenceServiceSpec) (v1alpha1.LLMInferenceServiceSpec, error) {
	if len(cfgs) == 0 {
		return v1alpha1.LLMInferenceServiceSpec{}, nil
	}

	out := cfgs[0]
	for i := 1; i < len(cfgs); i++ {
		cfg := cfgs[i]
		var err error
		out, err = mergeSpecs(out, cfg)
		if err != nil {
			return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("failed to merge specs: %w", err)
		}
	}
	return out, nil
}

// mergeSpecs performs a strategic merge by creating a clean patch from the override
// object and applying it to the base object.
func mergeSpecs(base, override v1alpha1.LLMInferenceServiceSpec) (v1alpha1.LLMInferenceServiceSpec, error) {
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

	overrideJSON, err := json.Marshal(override)
	if err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not marshal override spec: %w", err)
	}

	// Create the patch. It will only contain the non-default fields from the override.
	patch, err := strategicpatch.CreateTwoWayMergePatch(zeroJSON, overrideJSON, v1alpha1.LLMInferenceServiceSpec{})
	if err != nil {
		return v1alpha1.LLMInferenceServiceSpec{}, fmt.Errorf("could not create merge patch from override: %w", err)
	}

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
