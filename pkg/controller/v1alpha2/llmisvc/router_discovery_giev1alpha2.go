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

// This file contains v1alpha2 InferencePool readiness helpers.
// Delete this file when v1alpha2 InferencePool support is removed.

package llmisvc

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"

	"github.com/kserve/kserve/pkg/constants"
)

// IsInferencePoolV1Alpha2Ready checks if a v1alpha2 InferencePool has been accepted by all parents.
// This mirrors IsInferencePoolReady but for the v1alpha2 InferencePool type, which has a different
// status structure (PoolStatus instead of ParentStatus).
func IsInferencePoolV1Alpha2Ready(pool *igwapiv1alpha2.InferencePool) bool {
	if pool == nil {
		return false
	}

	if len(pool.Status.Parents) == 0 {
		return false
	}

	if cond, missing := nonReadyInferencePoolV1Alpha2TopLevelCondition(pool); cond != nil || missing {
		return false
	}

	return true
}

func nonReadyInferencePoolV1Alpha2TopLevelCondition(pool *igwapiv1alpha2.InferencePool) (*metav1.Condition, bool) {
	if pool == nil {
		return nil, true
	}

	for _, parent := range pool.Status.Parents {
		cond := meta.FindStatusCondition(parent.Conditions, string(igwapiv1alpha2.InferencePoolConditionAccepted))
		if cond == nil {
			return nil, true
		}
		staleCondition := cond.ObservedGeneration > 0 && cond.ObservedGeneration < pool.Generation
		if cond.Status != metav1.ConditionTrue || staleCondition {
			return cond, false
		}
	}

	return nil, false
}

// IsInferencePoolV1Alpha2ReadyForGateways checks if a v1alpha2 InferencePool is ready,
// considering only parents matching the given gateways.
// See IsInferencePoolReadyForGateways for details.
func IsInferencePoolV1Alpha2ReadyForGateways(pool *igwapiv1alpha2.InferencePool, gateways []types.NamespacedName) bool {
	if pool == nil || len(gateways) == 0 {
		return false
	}

	relevant := filterRelevantV1Alpha2Parents(pool.Status.Parents, gateways, pool.Namespace)
	if len(relevant) == 0 {
		return false
	}

	for _, gw := range gateways {
		if !hasMatchingV1Alpha2PoolParent(gw, relevant, pool.Namespace) {
			return false
		}
	}

	for _, parent := range relevant {
		cond := meta.FindStatusCondition(parent.Conditions, string(igwapiv1alpha2.InferencePoolConditionAccepted))
		if cond == nil {
			return false
		}
		staleCondition := cond.ObservedGeneration > 0 && cond.ObservedGeneration < pool.Generation
		if cond.Status != metav1.ConditionTrue || staleCondition {
			return false
		}
	}

	return true
}

func v1alpha2ParentKey(parent igwapiv1alpha2.PoolStatus, defaultNS string) types.NamespacedName {
	key := types.NamespacedName{Name: string(parent.GatewayRef.Name), Namespace: defaultNS}
	if parent.GatewayRef.Namespace != nil {
		key.Namespace = string(*parent.GatewayRef.Namespace)
	}
	return key
}

func filterRelevantV1Alpha2Parents(parents []igwapiv1alpha2.PoolStatus, gateways []types.NamespacedName, defaultNS string) []igwapiv1alpha2.PoolStatus {
	relevant := make([]igwapiv1alpha2.PoolStatus, 0, len(parents))
	for _, parent := range parents {
		if matchesAnyGateway(v1alpha2ParentKey(parent, defaultNS), gateways) {
			relevant = append(relevant, parent)
		}
	}
	return relevant
}

func findNonReadyV1Alpha2Condition(parents []igwapiv1alpha2.PoolStatus, conditionType string, generation int64) *metav1.Condition {
	for _, parent := range parents {
		cond := meta.FindStatusCondition(parent.Conditions, conditionType)
		if cond == nil {
			continue
		}
		stale := cond.ObservedGeneration > 0 && cond.ObservedGeneration < generation
		if cond.Status != metav1.ConditionTrue || stale {
			return cond
		}
	}
	return nil
}

func hasMatchingV1Alpha2PoolParent(gw types.NamespacedName, parents []igwapiv1alpha2.PoolStatus, defaultNS string) bool {
	for _, parent := range parents {
		if gw == v1alpha2ParentKey(parent, defaultNS) {
			return true
		}
	}
	return false
}

// IsInferencePoolV1Alpha2Supported checks if an HTTPRoute has been accepted by the Gateway, and it's using v1alpha2
// InferencePool.
func IsInferencePoolV1Alpha2Supported(route *gwapiv1.HTTPRoute) metav1.ConditionStatus {
	if isHTTPRouteUsingInferencePool(route, constants.InferencePoolV1Alpha2APIGroupName) {
		return isBackendSupported(route)
	}
	return metav1.ConditionUnknown
}
