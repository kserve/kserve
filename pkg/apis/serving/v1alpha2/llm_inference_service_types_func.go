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
	"fmt"
	"strconv"
	"strings"

	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"

	"github.com/kserve/kserve/pkg/constants"
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

func (r *RouterSpec) HasGroup() bool {
	return r != nil && r.Route != nil && r.Route.Group != nil
}

func (r *RouterSpec) Group() *string {
	if r == nil || r.Route == nil {
		return nil
	}
	return r.Route.Group
}

func (r *RouterSpec) Weight() *int32 {
	if r == nil || r.Route == nil {
		return nil
	}
	return r.Route.Weight
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

// HasManagedDRA reports whether managed DRA is enabled via annotations.
func (s *LLMInferenceService) HasManagedDRA() bool {
	if s == nil {
		return false
	}
	_, ok := s.Annotations[constants.ManagedDRADeviceClassAnnotationKey]
	return ok
}

// ManagedDRADeviceClass returns the trimmed device-class annotation value and
// whether it is set.
func (s *LLMInferenceService) ManagedDRADeviceClass() (string, bool) {
	if s == nil {
		return "", false
	}
	raw, ok := s.Annotations[constants.ManagedDRADeviceClassAnnotationKey]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(raw), true
}

// ManagedDRADeviceCount returns the requested device count, defaulting to 1.
func (s *LLMInferenceService) ManagedDRADeviceCount() (int, error) {
	if s == nil {
		return 1, nil
	}
	raw, ok := s.Annotations[constants.ManagedDRADeviceCountAnnotationKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return 1, nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", constants.ManagedDRADeviceCountAnnotationKey, raw, err)
	}
	if count < 1 {
		return 0, fmt.Errorf("invalid %s value %q: must be >= 1", constants.ManagedDRADeviceCountAnnotationKey, raw)
	}
	return count, nil
}

// ManagedDRACelSelectors returns the newline-separated CEL expressions, with
// empty lines and surrounding whitespace stripped.
func (s *LLMInferenceService) ManagedDRACelSelectors() []string {
	if s == nil {
		return nil
	}
	raw, ok := s.Annotations[constants.ManagedDRACelSelectorAnnotationKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	selectors := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			selectors = append(selectors, v)
		}
	}
	return selectors
}

// ManagedDRAContainerName returns the trimmed container-name annotation value
// and whether it is set.
func (s *LLMInferenceService) ManagedDRAContainerName() (string, bool) {
	if s == nil {
		return "", false
	}
	raw, ok := s.Annotations[constants.ManagedDRAContainerNameAnnotationKey]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(raw), true
}
