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
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	igwapiv1 "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

const (
	// ModelCriticalityAnnotationKey stores the model criticality when converting to v1alpha2.
	ModelCriticalityAnnotationKey = "internal.serving.kserve.io/model-criticality"
	// LoRACriticalitiesAnnotationKey stores LoRA adapter criticalities as JSON when converting to v1alpha2.
	LoRACriticalitiesAnnotationKey = "internal.serving.kserve.io/lora-criticalities"
	// LoRAExtrasAnnotationKey stores v1alpha2-only LoRA fields (LoadMode, Priority, Disabled, Ref,
	// MaxAdapters, MaxRank) as JSON so they survive a v1alpha2→v1alpha1→v1alpha2 round-trip.
	LoRAExtrasAnnotationKey = "internal.serving.kserve.io/lora-extras"
)

// loraExtras is the JSON payload stored in LoRAExtrasAnnotationKey.
type loraExtras struct {
	MaxAdapters *int32                    `json:"maxAdapters,omitempty"`
	MaxRank     *int32                    `json:"maxRank,omitempty"`
	Adapters    map[int]loraAdapterExtras `json:"adapters,omitempty"`
}

// loraAdapterExtras holds v1alpha2-only per-adapter fields that have no v1alpha1 equivalent.
type loraAdapterExtras struct {
	LoadMode v1alpha2.LoRALoadMode `json:"loadMode,omitempty"`
	Priority *int32                `json:"priority,omitempty"`
	Disabled bool                  `json:"disabled,omitempty"`
	// RefName is the Name of the corev1.LocalObjectReference stored in LoRAAdapterSpec.Ref.
	RefName string `json:"refName,omitempty"`
}

// Compile-time interface compliance checks.
var (
	_ conversion.Convertible = &LLMInferenceService{}
	_ conversion.Convertible = &LLMInferenceServiceConfig{}
)

// ConvertTo converts this LLMInferenceService (v1alpha1) to the Hub version (v1alpha2).
func (src *LLMInferenceService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.LLMInferenceService)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Preserve criticality values in annotations
	saveCriticalityToAnnotations(&dst.ObjectMeta, &src.Spec.Model)

	// Spec conversion
	dst.Spec = convertSpecToV1Alpha2(&src.Spec)

	// Restore v1alpha2-only LoRA fields from annotation (present when this object
	// was previously downgraded v1alpha2 → v1alpha1 and is now being promoted back).
	restoreLoRAExtrasFromAnnotations(&dst.ObjectMeta, dst.Spec.Model.LoRA)

	// Status conversion
	dst.Status = v1alpha2.LLMInferenceServiceStatus{
		URL:           src.Status.URL,
		Status:        src.Status.Status,
		AddressStatus: src.Status.AddressStatus,
	}

	return nil
}

// ConvertFrom converts the Hub version (v1alpha2) to this LLMInferenceService (v1alpha1).
func (dst *LLMInferenceService) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.LLMInferenceService)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Save v1alpha2-only LoRA fields to annotation before losing them in the downgrade.
	saveLoRAExtrasToAnnotations(&dst.ObjectMeta, src.Spec.Model.LoRA)

	// Spec conversion
	dst.Spec = convertSpecFromV1Alpha2(&src.Spec)

	// Restore criticality values from annotations
	restoreCriticalityFromAnnotations(&dst.ObjectMeta, &dst.Spec.Model)

	// Status conversion
	dst.Status = LLMInferenceServiceStatus{
		URL:           src.Status.URL,
		Status:        src.Status.Status,
		AddressStatus: src.Status.AddressStatus,
	}

	return nil
}

// ConvertTo converts this LLMInferenceServiceConfig (v1alpha1) to the Hub version (v1alpha2).
func (src *LLMInferenceServiceConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.LLMInferenceServiceConfig)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Preserve criticality values in annotations
	saveCriticalityToAnnotations(&dst.ObjectMeta, &src.Spec.Model)

	// Spec conversion
	dst.Spec = convertSpecToV1Alpha2(&src.Spec)

	// Restore v1alpha2-only LoRA fields from annotation.
	restoreLoRAExtrasFromAnnotations(&dst.ObjectMeta, dst.Spec.Model.LoRA)

	return nil
}

// ConvertFrom converts the Hub version (v1alpha2) to this LLMInferenceServiceConfig (v1alpha1).
func (dst *LLMInferenceServiceConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.LLMInferenceServiceConfig)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Save v1alpha2-only LoRA fields to annotation before losing them in the downgrade.
	saveLoRAExtrasToAnnotations(&dst.ObjectMeta, src.Spec.Model.LoRA)

	// Spec conversion
	dst.Spec = convertSpecFromV1Alpha2(&src.Spec)

	// Restore criticality values from annotations
	restoreCriticalityFromAnnotations(&dst.ObjectMeta, &dst.Spec.Model)

	return nil
}

func convertSpecToV1Alpha2(src *LLMInferenceServiceSpec) v1alpha2.LLMInferenceServiceSpec {
	dst := v1alpha2.LLMInferenceServiceSpec{
		Model:    convertModelSpecToV1Alpha2(&src.Model),
		BaseRefs: src.BaseRefs,
	}

	// StorageInitializer - direct copy since structs are identical
	if src.StorageInitializer != nil {
		dst.StorageInitializer = &v1alpha2.StorageInitializerSpec{
			Enabled: src.StorageInitializer.Enabled,
		}
	}

	// WorkloadSpec (inline)
	dst.WorkloadSpec = convertWorkloadSpecToV1Alpha2(&src.WorkloadSpec)

	// Router
	if src.Router != nil {
		dst.Router = convertRouterSpecToV1Alpha2(src.Router)
	}

	// Prefill
	if src.Prefill != nil {
		prefill := convertWorkloadSpecToV1Alpha2(src.Prefill)
		dst.Prefill = &prefill
	}

	return dst
}

func convertSpecFromV1Alpha2(src *v1alpha2.LLMInferenceServiceSpec) LLMInferenceServiceSpec {
	dst := LLMInferenceServiceSpec{
		Model:    convertModelSpecFromV1Alpha2(&src.Model),
		BaseRefs: src.BaseRefs,
	}

	// StorageInitializer - direct copy since structs are identical
	if src.StorageInitializer != nil {
		dst.StorageInitializer = &StorageInitializerSpec{
			Enabled: src.StorageInitializer.Enabled,
		}
	}

	// WorkloadSpec (inline)
	dst.WorkloadSpec = convertWorkloadSpecFromV1Alpha2(&src.WorkloadSpec)

	// Router
	if src.Router != nil {
		dst.Router = convertRouterSpecFromV1Alpha2(src.Router)
	}

	// Prefill
	if src.Prefill != nil {
		prefill := convertWorkloadSpecFromV1Alpha2(src.Prefill)
		dst.Prefill = &prefill
	}

	return dst
}

func convertScalingSpecToV1Alpha2(src *ScalingSpec) *v1alpha2.ScalingSpec {
	if src == nil {
		return nil
	}

	dst := &v1alpha2.ScalingSpec{
		MinReplicas: src.MinReplicas,
		MaxReplicas: src.MaxReplicas,
	}

	if src.WVA != nil {
		dst.WVA = &v1alpha2.WVASpec{
			VariantCost: src.WVA.VariantCost,
		}
		if src.WVA.HPA != nil {
			dst.WVA.HPA = &v1alpha2.HPAScalingSpec{
				Behavior: src.WVA.HPA.Behavior,
			}
		}
		if src.WVA.KEDA != nil {
			dst.WVA.KEDA = &v1alpha2.KEDAScalingSpec{
				PollingInterval:       src.WVA.KEDA.PollingInterval,
				CooldownPeriod:        src.WVA.KEDA.CooldownPeriod,
				InitialCooldownPeriod: src.WVA.KEDA.InitialCooldownPeriod,
				IdleReplicaCount:      src.WVA.KEDA.IdleReplicaCount,
				Fallback:              src.WVA.KEDA.Fallback,
				Advanced:              src.WVA.KEDA.Advanced,
			}
		}
	}

	return dst
}

func convertScalingSpecFromV1Alpha2(src *v1alpha2.ScalingSpec) *ScalingSpec {
	if src == nil {
		return nil
	}

	dst := &ScalingSpec{
		MinReplicas: src.MinReplicas,
		MaxReplicas: src.MaxReplicas,
	}

	if src.WVA != nil {
		dst.WVA = &WVASpec{
			VariantCost: src.WVA.VariantCost,
		}
		if src.WVA.HPA != nil {
			dst.WVA.HPA = &HPAScalingSpec{
				Behavior: src.WVA.HPA.Behavior,
			}
		}
		if src.WVA.KEDA != nil {
			dst.WVA.KEDA = &KEDAScalingSpec{
				PollingInterval:       src.WVA.KEDA.PollingInterval,
				CooldownPeriod:        src.WVA.KEDA.CooldownPeriod,
				InitialCooldownPeriod: src.WVA.KEDA.InitialCooldownPeriod,
				IdleReplicaCount:      src.WVA.KEDA.IdleReplicaCount,
				Fallback:              src.WVA.KEDA.Fallback,
				Advanced:              src.WVA.KEDA.Advanced,
			}
		}
	}

	return dst
}

func convertModelSpecToV1Alpha2(src *LLMModelSpec) v1alpha2.LLMModelSpec {
	dst := v1alpha2.LLMModelSpec{
		URI:  src.URI,
		Name: src.Name,
		// Note: Criticality is preserved via annotations in ConvertTo/ConvertFrom
	}

	if src.LoRA != nil {
		dst.LoRA = convertLoRASpecToV1Alpha2(src.LoRA)
	}

	return dst
}

func convertModelSpecFromV1Alpha2(src *v1alpha2.LLMModelSpec) LLMModelSpec {
	dst := LLMModelSpec{
		URI:  src.URI,
		Name: src.Name,
		// Note: Criticality is restored from annotations in ConvertTo/ConvertFrom
	}

	if src.LoRA != nil {
		dst.LoRA = convertLoRASpecFromV1Alpha2(src.LoRA)
	}

	return dst
}

func convertLoRASpecToV1Alpha2(src *LoRASpec) *v1alpha2.LoRASpec {
	if src == nil {
		return nil
	}

	dst := &v1alpha2.LoRASpec{}
	for i := range src.Adapters {
		a := &src.Adapters[i]
		uri := a.URI // copy value so we can safely take its address
		adapted := v1alpha2.LoRAAdapterSpec{
			URI: &uri,
		}
		if a.Name != nil {
			adapted.Name = *a.Name
		}
		dst.Adapters = append(dst.Adapters, adapted)
	}
	// MaxAdapters and MaxRank are v1alpha2-only; they are restored from the
	// LoRAExtrasAnnotationKey annotation in restoreLoRAExtrasFromAnnotations.
	return dst
}

func convertLoRASpecFromV1Alpha2(src *v1alpha2.LoRASpec) *LoRASpec {
	if src == nil {
		return nil
	}

	dst := &LoRASpec{}
	for i := range src.Adapters {
		a := &src.Adapters[i]
		name := a.Name
		adapted := LLMModelSpec{
			Name: &name,
		}
		if a.URI != nil {
			adapted.URI = *a.URI
		}
		// LoadMode, Priority, Disabled, Ref, MaxAdapters, MaxRank are saved to
		// LoRAExtrasAnnotationKey in saveLoRAExtrasToAnnotations before this runs.
		dst.Adapters = append(dst.Adapters, adapted)
	}
	return dst
}

func convertWorkloadSpecToV1Alpha2(src *WorkloadSpec) v1alpha2.WorkloadSpec {
	dst := v1alpha2.WorkloadSpec{
		Replicas:    src.Replicas,
		Labels:      src.Labels,
		Annotations: src.Annotations,
		Template:    src.Template,
		Worker:      src.Worker,
	}

	if src.Parallelism != nil {
		dst.Parallelism = &v1alpha2.ParallelismSpec{
			Tensor:      src.Parallelism.Tensor,
			Pipeline:    src.Parallelism.Pipeline,
			Data:        src.Parallelism.Data,
			DataLocal:   src.Parallelism.DataLocal,
			DataRPCPort: src.Parallelism.DataRPCPort,
			Expert:      src.Parallelism.Expert,
		}
	}

	if src.Scaling != nil {
		dst.Scaling = convertScalingSpecToV1Alpha2(src.Scaling)
	}

	return dst
}

func convertWorkloadSpecFromV1Alpha2(src *v1alpha2.WorkloadSpec) WorkloadSpec {
	dst := WorkloadSpec{
		Replicas:    src.Replicas,
		Labels:      src.Labels,
		Annotations: src.Annotations,
		Template:    src.Template,
		Worker:      src.Worker,
	}

	if src.Parallelism != nil {
		dst.Parallelism = &ParallelismSpec{
			Tensor:      src.Parallelism.Tensor,
			Pipeline:    src.Parallelism.Pipeline,
			Data:        src.Parallelism.Data,
			DataLocal:   src.Parallelism.DataLocal,
			DataRPCPort: src.Parallelism.DataRPCPort,
			Expert:      src.Parallelism.Expert,
		}
	}

	if src.Scaling != nil {
		dst.Scaling = convertScalingSpecFromV1Alpha2(src.Scaling)
	}

	return dst
}

func convertRouterSpecToV1Alpha2(src *RouterSpec) *v1alpha2.RouterSpec {
	if src == nil {
		return nil
	}

	dst := &v1alpha2.RouterSpec{}

	if src.Route != nil {
		dst.Route = &v1alpha2.GatewayRoutesSpec{}
		if src.Route.HTTP != nil {
			dst.Route.HTTP = &v1alpha2.HTTPRouteSpec{
				Refs: src.Route.HTTP.Refs,
				Spec: src.Route.HTTP.Spec,
			}
		}
	}

	if src.Gateway != nil {
		dst.Gateway = &v1alpha2.GatewaySpec{}
		for _, ref := range src.Gateway.Refs {
			dst.Gateway.Refs = append(dst.Gateway.Refs, v1alpha2.GatewayObjectReference{
				UntypedObjectReference: v1alpha2.UntypedObjectReference{
					Name:      ref.Name,
					Namespace: ref.Namespace,
				},
				SectionName: ref.SectionName,
			})
		}
	}

	if src.Ingress != nil {
		dst.Ingress = &v1alpha2.IngressSpec{}
		for _, ref := range src.Ingress.Refs {
			dst.Ingress.Refs = append(dst.Ingress.Refs, v1alpha2.UntypedObjectReference{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			})
		}
	}

	if src.Scheduler != nil {
		dst.Scheduler = &v1alpha2.SchedulerSpec{
			Labels:      src.Scheduler.Labels,
			Annotations: src.Scheduler.Annotations,
			Template:    src.Scheduler.Template,
			Replicas:    src.Scheduler.Replicas,
		}
		if src.Scheduler.Pool != nil {
			dst.Scheduler.Pool = &v1alpha2.InferencePoolSpec{
				Ref:  src.Scheduler.Pool.Ref,
				Spec: convertInferencePoolSpecToV1(src.Scheduler.Pool.Spec),
			}
		}
		if src.Scheduler.Config != nil {
			dst.Scheduler.Config = &v1alpha2.SchedulerConfigSpec{
				Inline: src.Scheduler.Config.Inline,
				Ref:    src.Scheduler.Config.Ref,
			}
		}
	}

	return dst
}

// convertInferencePoolSpecToV1 converts igwapi v1alpha2 InferencePoolSpec to igwapi v1 InferencePoolSpec
// using the built-in conversion from the gateway-api-inference-extension library.
func convertInferencePoolSpecToV1(src *igwapiv1alpha2.InferencePoolSpec) *igwapiv1.InferencePoolSpec {
	if src == nil {
		return nil
	}

	// Use the built-in conversion by wrapping in temporary InferencePool objects
	srcPool := &igwapiv1alpha2.InferencePool{Spec: *src}
	dstPool := &igwapiv1.InferencePool{}

	if err := srcPool.ConvertTo(dstPool); err != nil {
		// Return nil rather than an empty spec — callers nil-check Pool.Spec, and an
		// empty spec would bypass those guards with invalid zero-value fields.
		return nil
	}

	return &dstPool.Spec
}

func convertRouterSpecFromV1Alpha2(src *v1alpha2.RouterSpec) *RouterSpec {
	if src == nil {
		return nil
	}

	dst := &RouterSpec{}

	if src.Route != nil {
		dst.Route = &GatewayRoutesSpec{}
		if src.Route.HTTP != nil {
			dst.Route.HTTP = &HTTPRouteSpec{
				Refs: src.Route.HTTP.Refs,
				Spec: src.Route.HTTP.Spec,
			}
		}
	}

	if src.Gateway != nil {
		dst.Gateway = &GatewaySpec{}
		for _, ref := range src.Gateway.Refs {
			dst.Gateway.Refs = append(dst.Gateway.Refs, GatewayObjectReference{
				UntypedObjectReference: UntypedObjectReference{
					Name:      ref.Name,
					Namespace: ref.Namespace,
				},
				SectionName: ref.SectionName,
			})
		}
	}

	if src.Ingress != nil {
		dst.Ingress = &IngressSpec{}
		for _, ref := range src.Ingress.Refs {
			dst.Ingress.Refs = append(dst.Ingress.Refs, UntypedObjectReference{
				Name:      ref.Name,
				Namespace: ref.Namespace,
			})
		}
	}

	if src.Scheduler != nil {
		dst.Scheduler = &SchedulerSpec{
			Labels:      src.Scheduler.Labels,
			Annotations: src.Scheduler.Annotations,
			Template:    src.Scheduler.Template,
			Replicas:    src.Scheduler.Replicas,
		}
		if src.Scheduler.Pool != nil {
			dst.Scheduler.Pool = &InferencePoolSpec{
				Ref:  src.Scheduler.Pool.Ref,
				Spec: convertInferencePoolSpecFromV1(src.Scheduler.Pool.Spec),
			}
		}
		if src.Scheduler.Config != nil {
			dst.Scheduler.Config = &SchedulerConfigSpec{
				Inline: src.Scheduler.Config.Inline,
				Ref:    src.Scheduler.Config.Ref,
			}
		}
	}

	return dst
}

// convertInferencePoolSpecFromV1 converts igwapi v1 InferencePoolSpec to igwapi v1alpha2 InferencePoolSpec
// using the built-in conversion from the gateway-api-inference-extension library.
func convertInferencePoolSpecFromV1(src *igwapiv1.InferencePoolSpec) *igwapiv1alpha2.InferencePoolSpec {
	if src == nil {
		return nil
	}

	// Use the built-in conversion by wrapping in temporary InferencePool objects
	srcPool := &igwapiv1.InferencePool{Spec: *src}
	dstPool := &igwapiv1alpha2.InferencePool{}

	if err := dstPool.ConvertFrom(srcPool); err != nil {
		// Return nil rather than an empty spec — callers nil-check Pool.Spec, and an
		// empty spec would bypass those guards with invalid zero-value fields.
		return nil
	}

	return &dstPool.Spec
}

// saveLoRAExtrasToAnnotations serialises v1alpha2-only LoRA fields into LoRAExtrasAnnotationKey
// so they survive a v1alpha2→v1alpha1 downgrade. Called during ConvertFrom.
func saveLoRAExtrasToAnnotations(meta *metav1.ObjectMeta, lora *v1alpha2.LoRASpec) {
	if lora == nil {
		return
	}
	extras := loraExtras{
		MaxAdapters: lora.MaxAdapters,
		MaxRank:     lora.MaxRank,
	}
	hasExtras := extras.MaxAdapters != nil || extras.MaxRank != nil
	for i, adapter := range lora.Adapters {
		e := loraAdapterExtras{
			LoadMode: adapter.LoadMode,
			Priority: adapter.Priority,
			Disabled: adapter.Disabled,
		}
		if adapter.Ref != nil {
			e.RefName = adapter.Ref.Name
		}
		if e.LoadMode != "" || e.Priority != nil || e.Disabled || e.RefName != "" {
			if extras.Adapters == nil {
				extras.Adapters = make(map[int]loraAdapterExtras)
			}
			extras.Adapters[i] = e
			hasExtras = true
		}
	}
	if !hasExtras {
		return
	}
	data, err := json.Marshal(extras)
	if err != nil {
		return
	}
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	meta.Annotations[LoRAExtrasAnnotationKey] = string(data)
}

// restoreLoRAExtrasFromAnnotations reads LoRAExtrasAnnotationKey and applies the stored
// v1alpha2-only fields back onto the LoRASpec after conversion. Called during ConvertTo.
func restoreLoRAExtrasFromAnnotations(meta *metav1.ObjectMeta, lora *v1alpha2.LoRASpec) {
	if meta.Annotations == nil || lora == nil {
		return
	}
	extrasData, ok := meta.Annotations[LoRAExtrasAnnotationKey]
	if !ok || extrasData == "" {
		return
	}
	var extras loraExtras
	if err := json.Unmarshal([]byte(extrasData), &extras); err != nil {
		return
	}
	lora.MaxAdapters = extras.MaxAdapters
	lora.MaxRank = extras.MaxRank
	for i := range lora.Adapters {
		if e, ok := extras.Adapters[i]; ok {
			lora.Adapters[i].LoadMode = e.LoadMode
			lora.Adapters[i].Priority = e.Priority
			lora.Adapters[i].Disabled = e.Disabled
			if e.RefName != "" {
				lora.Adapters[i].Ref = &corev1.LocalObjectReference{Name: e.RefName}
				// Ref and URI are mutually exclusive; clear URI when restoring a Ref.
				lora.Adapters[i].URI = nil
			}
		}
	}
	delete(meta.Annotations, LoRAExtrasAnnotationKey)
	if len(meta.Annotations) == 0 {
		meta.Annotations = nil
	}
}

// saveCriticalityToAnnotations stores criticality values in annotations to prevent data loss
// when converting from v1alpha1 to v1alpha2.
func saveCriticalityToAnnotations(meta *metav1.ObjectMeta, model *LLMModelSpec) {
	// Save model criticality
	if model.Criticality != nil && *model.Criticality != "" {
		if meta.Annotations == nil {
			meta.Annotations = make(map[string]string)
		}
		meta.Annotations[ModelCriticalityAnnotationKey] = string(*model.Criticality)
	}

	// Save LoRA adapter criticalities
	if model.LoRA != nil && len(model.LoRA.Adapters) > 0 {
		loraCriticalities := make(map[int]string)
		hasCriticality := false
		for i, adapter := range model.LoRA.Adapters {
			if adapter.Criticality != nil && *adapter.Criticality != "" {
				loraCriticalities[i] = string(*adapter.Criticality)
				hasCriticality = true
			}
		}
		if hasCriticality {
			if meta.Annotations == nil {
				meta.Annotations = make(map[string]string)
			}
			if data, err := json.Marshal(loraCriticalities); err == nil {
				meta.Annotations[LoRACriticalitiesAnnotationKey] = string(data)
			}
		}
	}
}

// restoreCriticalityFromAnnotations restores criticality values from annotations
// when converting from v1alpha2 to v1alpha1.
func restoreCriticalityFromAnnotations(meta *metav1.ObjectMeta, model *LLMModelSpec) {
	if meta.Annotations == nil {
		return
	}

	// Restore model criticality
	if criticality, ok := meta.Annotations[ModelCriticalityAnnotationKey]; ok && criticality != "" {
		c := Criticality(criticality)
		model.Criticality = &c
		delete(meta.Annotations, ModelCriticalityAnnotationKey)
	}

	// Restore LoRA adapter criticalities
	if loraData, ok := meta.Annotations[LoRACriticalitiesAnnotationKey]; ok && loraData != "" {
		var loraCriticalities map[int]string
		if err := json.Unmarshal([]byte(loraData), &loraCriticalities); err == nil {
			if model.LoRA != nil {
				for i := range model.LoRA.Adapters {
					if criticality, exists := loraCriticalities[i]; exists {
						c := Criticality(criticality)
						model.LoRA.Adapters[i].Criticality = &c
					}
				}
			}
		}
		delete(meta.Annotations, LoRACriticalitiesAnnotationKey)
	}

	// Clean up empty annotations map
	if len(meta.Annotations) == 0 {
		meta.Annotations = nil
	}
}
