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
	"slices"
	"strings"

	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// rewriteRulesForGroup replaces backendRefs on rules that have a controller-managed
// backend with the full set of weighted group member backends. Rules with only
// user-managed custom backendRefs are left untouched.
func rewriteRulesForGroup(route *gwapiv1.HTTPRoute, llmSvc *v1alpha2.LLMInferenceService, members []resolvedMember) {
	allBackendRefs := make([]gwapiv1.HTTPBackendRef, 0, len(members))
	for _, m := range members {
		// Omit stopped members entirely - a weight:0 backendRef still causes
		// some gateways (e.g. Envoy Gateway) to leak traffic to the backend.
		if m.stopped {
			continue
		}
		allBackendRefs = append(allBackendRefs, gwapiv1.HTTPBackendRef{
			BackendRef: gwapiv1.BackendRef{
				BackendObjectReference: m.backendRef,
				Weight:                 ptr.To(m.weight),
			},
		})
	}

	perParticipantPrefix := "/" + llmSvc.Namespace + "/" + llmSvc.Name

	for i := range route.Spec.Rules {
		if isPerParticipantRule(route.Spec.Rules[i], perParticipantPrefix) {
			continue
		}
		if !hasControllerManagedBackendRef(route.Spec.Rules[i], llmSvc) {
			continue
		}
		route.Spec.Rules[i].BackendRefs = slices.Clone(allBackendRefs)
	}
}

// isPerParticipantRule returns true for rules whose path matches the
// per-participant pattern (/{namespace}/{name}/...). These are direct-access
// routes unique to this member and should not get weighted group backendRefs.
func isPerParticipantRule(rule gwapiv1.HTTPRouteRule, perParticipantPrefix string) bool {
	for _, match := range rule.Matches {
		if match.Path == nil || match.Path.Value == nil {
			continue
		}
		path := *match.Path.Value
		if path == perParticipantPrefix ||
			strings.HasPrefix(path, perParticipantPrefix+"/") {
			return true
		}
	}
	return false
}

// hasControllerManagedBackendRef returns true if any backendRef in the rule is
// a controller-managed backend: default InferencePool, custom pool ref, or
// workload Service.
func hasControllerManagedBackendRef(rule gwapiv1.HTTPRouteRule, llmSvc *v1alpha2.LLMInferenceService) bool {
	for _, ref := range rule.BackendRefs {
		if isExpectedBackendRef(llmSvc, ref.BackendRef) {
			return true
		}
	}
	return false
}

// isExpectedBackendRef checks whether a backendRef matches the backend that
// the controller would produce for this LLMISVC: default InferencePool,
// scheduler pool ref, or workload Service.
func isExpectedBackendRef(llmSvc *v1alpha2.LLMInferenceService, ref gwapiv1.BackendRef) bool {
	if isDefaultBackendRef(llmSvc, ref) {
		return true
	}
	if ptr.Deref(ref.Kind, "Service") == "Service" &&
		string(ref.Name) == workloadServiceName(llmSvc) {
		return true
	}
	// User-provided InferencePool via scheduler.pool.ref
	if llmSvc.Spec.Router != nil &&
		llmSvc.Spec.Router.Scheduler != nil &&
		llmSvc.Spec.Router.Scheduler.Pool.HasRef() &&
		ptr.Deref(ref.Kind, "") == "InferencePool" &&
		string(ref.Name) == llmSvc.Spec.Router.Scheduler.Pool.Ref.Name {
		return true
	}
	return false
}
