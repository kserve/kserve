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
)

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

	return nil
}

// ConvertFrom converts the Hub version (v1alpha2) to this LLMInferenceServiceConfig (v1alpha1).
func (dst *LLMInferenceServiceConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha2.LLMInferenceServiceConfig)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

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
	for _, adapter := range src.Adapters {
		dst.Adapters = append(dst.Adapters, convertModelSpecToV1Alpha2(&adapter))
	}

	return dst
}

func convertLoRASpecFromV1Alpha2(src *v1alpha2.LoRASpec) *LoRASpec {
	if src == nil {
		return nil
	}

	dst := &LoRASpec{}
	for _, adapter := range src.Adapters {
		dst.Adapters = append(dst.Adapters, convertModelSpecFromV1Alpha2(&adapter))
	}

	return dst
}

func convertWorkloadSpecToV1Alpha2(src *WorkloadSpec) v1alpha2.WorkloadSpec {
	dst := v1alpha2.WorkloadSpec{
		Replicas: src.Replicas,
		Template: src.Template,
		Worker:   src.Worker,
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

	return dst
}

func convertWorkloadSpecFromV1Alpha2(src *v1alpha2.WorkloadSpec) WorkloadSpec {
	dst := WorkloadSpec{
		Replicas: src.Replicas,
		Template: src.Template,
		Worker:   src.Worker,
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
			dst.Gateway.Refs = append(dst.Gateway.Refs, v1alpha2.UntypedObjectReference{
				Name:      ref.Name,
				Namespace: ref.Namespace,
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
			Template: src.Scheduler.Template,
		}
		if src.Scheduler.Pool != nil {
			dst.Scheduler.Pool = &v1alpha2.InferencePoolSpec{
				Ref:  src.Scheduler.Pool.Ref,
				Spec: convertInferencePoolSpecToV1(src.Scheduler.Pool.Spec),
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
		// Fallback: return empty spec on error (should not happen in practice)
		return &igwapiv1.InferencePoolSpec{}
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
			dst.Gateway.Refs = append(dst.Gateway.Refs, UntypedObjectReference{
				Name:      ref.Name,
				Namespace: ref.Namespace,
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
			Template: src.Scheduler.Template,
		}
		if src.Scheduler.Pool != nil {
			dst.Scheduler.Pool = &InferencePoolSpec{
				Ref:  src.Scheduler.Pool.Ref,
				Spec: convertInferencePoolSpecFromV1(src.Scheduler.Pool.Spec),
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
		// Fallback: return empty spec on error (should not happen in practice)
		return &igwapiv1alpha2.InferencePoolSpec{}
	}

	return &dstPool.Spec
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
