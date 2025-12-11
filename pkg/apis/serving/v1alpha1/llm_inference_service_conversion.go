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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// ConvertTo converts this LLMInferenceService (v1alpha1) to the Hub version (v1alpha2).
func (src *LLMInferenceService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha2.LLMInferenceService)

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

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

	return nil
}

func convertSpecToV1Alpha2(src *LLMInferenceServiceSpec) v1alpha2.LLMInferenceServiceSpec {
	dst := v1alpha2.LLMInferenceServiceSpec{
		Model:    convertModelSpecToV1Alpha2(&src.Model),
		BaseRefs: src.BaseRefs,
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
		// Note: Criticality field is dropped in v1alpha2
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
		// Note: Criticality field doesn't exist in v1alpha2, will be nil
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
				Ref: src.Scheduler.Pool.Ref,
				// Note: v1alpha1 InferencePoolSpec doesn't have Spec field
			}
		}
	}

	return dst
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
				Ref: src.Scheduler.Pool.Ref,
				// Note: v1alpha2 InferencePoolSpec.Spec is not converted (lossy)
			}
		}
	}

	return dst
}
