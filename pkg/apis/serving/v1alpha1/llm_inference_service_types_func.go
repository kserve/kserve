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
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
)

func (r *RouterSpec) HasSchedulerTemplate() bool {
	return r != nil && r.Scheduler != nil && r.Scheduler.Template != nil
}

func InferenceModelName(llmSvc *LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-inference-model")
}

func (s *SchedulerSpec) InferencePoolName(llmSvc *LLMInferenceService) string {
	if s == nil || s.Pool == nil || !s.Pool.HasRef() {
		// This default MUST match the default value set in the well-known presets.
		return kmeta.ChildName(llmSvc.GetName(), "-inference-pool")
	}
	return s.Pool.Ref.Name
}

func (r *RouterSpec) EPPServiceName(llmSvc *LLMInferenceService) string {
	// If Scheduler/Pool/inline Spec aren’t provided, fall back to our managed EPP Service name.
	if r == nil || r.Scheduler == nil || r.Scheduler.Pool == nil || r.Scheduler.Pool.Spec == nil {
		return kmeta.ChildName(llmSvc.GetName(), "-epp-service")
	}

	// In v1, EndpointPickerRef is a value (not a pointer). Its Name is a typed string alias.
	name := string(r.Scheduler.Pool.Spec.EndpointPickerRef.Name)
	if name == "" {
		return kmeta.ChildName(llmSvc.GetName(), "-epp-service")
	}
	return name
}

func (in *GatewayRoutesSpec) IsManaged() bool {
	if in == nil || in.HTTP == nil {
		return false
	}
	// “Managed” means: user gave an inline HTTPRoute spec and did NOT provide refs.
	return in.HTTP.Spec != nil && len(in.HTTP.Refs) == 0
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
		return ptr.To(max(ptr.Deref(p.Data, 1), 1) / max(ptr.Deref(p.DataLocal, 1), 1))
	}
	if p.IsPipelineParallel() {
		return p.Pipeline
	}
	return nil
}
