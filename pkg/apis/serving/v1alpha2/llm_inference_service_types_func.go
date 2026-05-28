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
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
)

func (s *SchedulerSpec) InferencePoolName(llmSvc *LLMInferenceService) string {
	if s == nil || s.Pool == nil || !s.Pool.HasRef() {
		// This default MUST match the default value set in the well-known presets.
		return kmeta.ChildName(llmSvc.GetName(), "-inference-pool")
	}
	return s.Pool.Ref.Name
}

func (r *RouterSpec) EPPServiceName(llmSvc *LLMInferenceService) string {
	if r != nil && r.Scheduler != nil && r.Scheduler.Pool != nil &&
		!r.Scheduler.Pool.HasRef() &&
		r.Scheduler.Pool.Spec != nil && r.Scheduler.Pool.Spec.EndpointPickerRef.Name != "" {
		return string(r.Scheduler.Pool.Spec.EndpointPickerRef.Name)
	}
	return kmeta.ChildName(llmSvc.GetName(), "-epp-service")
}

func (in *GatewaySpec) HasRefs() bool {
	return in != nil && len(in.Refs) > 0
}

func (r *HTTPRouteSpec) HasRefs() bool {
	return r != nil && len(r.Refs) > 0
}

func (r *HTTPRouteSpec) HasSpec() bool {
	return r != nil && r.Spec != nil
}

func (p *InferencePoolSpec) HasRef() bool {
	return p != nil && p.Ref != nil && p.Ref.Name != ""
}

func (p *ParallelismSpec) IsPipelineParallel() bool {
	if p == nil {
		return false
	}
	return ptr.Deref(p.Pipeline, 0) > 0
}

func (p *ParallelismSpec) IsDataParallel() bool {
	if p == nil {
		return false
	}
	return ptr.Deref(p.Data, 0) > 0 || ptr.Deref(p.DataLocal, 0) > 0
}

func (p *ParallelismSpec) IsTensorParallel() bool {
	if p == nil {
		return false
	}
	return ptr.Deref(p.Tensor, 0) > 0
}

func (p *ParallelismSpec) GetSize() *int32 {
	if p == nil {
		return nil
	}
	if p.IsDataParallel() {
		return ptr.To(max(
			// p.Data / p.DataLocal
			max(ptr.Deref(p.Data, 1), 1)/max(ptr.Deref(p.DataLocal, 1), 1),
			1,
		))
	}
	if p.IsPipelineParallel() {
		return p.Pipeline
	}
	return nil
}

// IsUsingLLMInferenceServiceConfig returns true if the given config name is referenced by this service.
// This is a name-only helper and should be preferred only when namespace context is unavailable.
func (s *LLMInferenceService) IsUsingLLMInferenceServiceConfig(name string) bool {
	return s.IsUsingLLMInferenceServiceConfigInNamespace(name, "")
}

// IsUsingLLMInferenceServiceConfigInNamespace returns true if the given config is referenced by this service.
// When status.appliedConfigs is present, it is treated as authoritative.
// Annotation/baseRefs fallback is used when appliedConfigs is empty (new service, or stopped service
// whose applied configs were cleared).
func (s *LLMInferenceService) IsUsingLLMInferenceServiceConfigInNamespace(name, namespace string) bool {
	// Use applied configs from the last successful reconciliation when available.
	if len(s.Status.AppliedConfigRefs) > 0 {
		for i := range s.Status.AppliedConfigRefs {
			if string(s.Status.AppliedConfigRefs[i].Name) != name {
				continue
			}

			if namespace == "" || string(s.Status.AppliedConfigRefs[i].Namespace) == namespace {
				return true
			}
		}
		return false
	}

	// Fallback: appliedConfigs is empty (not yet reconciled, or cleared on stop).
	for _, value := range s.Status.Annotations {
		if value == name {
			return true
		}
	}

	for _, ref := range s.Spec.BaseRefs {
		if ref.Name == name {
			return true
		}
	}

	return false
}
