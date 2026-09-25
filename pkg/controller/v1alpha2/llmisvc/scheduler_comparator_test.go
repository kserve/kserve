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
	"testing"

	"github.com/stretchr/testify/assert"
	rbacv1 "k8s.io/api/rbac/v1"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"
)

// The selector is a map, and a map with an extra key on the cluster is a subset
// match. Dropping a key from the spec therefore leaves the pool selecting a wider
// set of pods than the service asks for.
func TestSemanticInferencePoolIsEqual(t *testing.T) {
	pool := func(selector map[igwapi.LabelKey]igwapi.LabelValue, ports ...int32) *igwapi.InferencePool {
		targetPorts := make([]igwapi.Port, 0, len(ports))
		for _, p := range ports {
			targetPorts = append(targetPorts, igwapi.Port{Number: igwapi.PortNumber(p)})
		}
		return &igwapi.InferencePool{
			Spec: igwapi.InferencePoolSpec{
				Selector:    igwapi.LabelSelector{MatchLabels: selector},
				TargetPorts: targetPorts,
			},
		}
	}

	both := map[igwapi.LabelKey]igwapi.LabelValue{"app": "vllm", "tier": "gpu"}
	one := map[igwapi.LabelKey]igwapi.LabelValue{"app": "vllm"}

	tests := []struct {
		name     string
		expected *igwapi.InferencePool
		current  *igwapi.InferencePool
		wantEq   bool
	}{
		{
			name:     "identical - no update",
			expected: pool(both, 8000),
			current:  pool(both, 8000),
			wantEq:   true,
		},
		{
			name:     "selector key removed from spec - update",
			expected: pool(one, 8000),
			current:  pool(both, 8000),
			wantEq:   false,
		},
		{
			name:     "selector key added to spec - update",
			expected: pool(both, 8000),
			current:  pool(one, 8000),
			wantEq:   false,
		},
		{
			name:     "selector value changed - update",
			expected: pool(map[igwapi.LabelKey]igwapi.LabelValue{"app": "sglang"}, 8000),
			current:  pool(one, 8000),
			wantEq:   false,
		},
		{
			name:     "target port removed from spec - update",
			expected: pool(one, 8000),
			current:  pool(one, 8000, 8001),
			wantEq:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEq, semanticInferencePoolIsEqual(tt.expected, tt.current))
		})
	}
}

func TestSemanticInferencePoolV1Alpha2IsEqual(t *testing.T) {
	pool := func(selector map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue) *igwapiv1alpha2.InferencePool {
		return &igwapiv1alpha2.InferencePool{
			Spec: igwapiv1alpha2.InferencePoolSpec{
				Selector:         selector,
				TargetPortNumber: 8000,
			},
		}
	}

	both := map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm", "tier": "gpu"}
	one := map[igwapiv1alpha2.LabelKey]igwapiv1alpha2.LabelValue{"app": "vllm"}

	tests := []struct {
		name     string
		expected *igwapiv1alpha2.InferencePool
		current  *igwapiv1alpha2.InferencePool
		wantEq   bool
	}{
		{
			name:     "identical - no update",
			expected: pool(both),
			current:  pool(both),
			wantEq:   true,
		},
		{
			name:     "selector key removed from spec - update",
			expected: pool(one),
			current:  pool(both),
			wantEq:   false,
		},
		{
			name:     "selector key added to spec - update",
			expected: pool(both),
			current:  pool(one),
			wantEq:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEq, semanticInferencePoolV1Alpha2IsEqual(tt.expected, tt.current))
		})
	}
}

// Role rules are hardcoded in the controller, so they shrink when a release
// narrows them. Until the comparison is exact, that narrowing never reaches
// services that already exist - the wider grant stays in place.
func TestSemanticRoleIsEqual(t *testing.T) {
	role := func(rules ...rbacv1.PolicyRule) *rbacv1.Role {
		return &rbacv1.Role{Rules: rules}
	}

	pods := rbacv1.PolicyRule{
		APIGroups: []string{""},
		Resources: []string{"pods"},
		Verbs:     []string{"get", "list", "watch"},
	}
	podsReadOnly := rbacv1.PolicyRule{
		APIGroups: []string{""},
		Resources: []string{"pods"},
		Verbs:     []string{"get", "list"},
	}
	leases := rbacv1.PolicyRule{
		APIGroups: []string{"coordination.k8s.io"},
		Resources: []string{"leases"},
		Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
	}

	tests := []struct {
		name     string
		expected *rbacv1.Role
		current  *rbacv1.Role
		wantEq   bool
	}{
		{
			name:     "identical - no update",
			expected: role(pods, leases),
			current:  role(pods, leases),
			wantEq:   true,
		},
		{
			name:     "rule dropped from the desired state - update",
			expected: role(pods),
			current:  role(pods, leases),
			wantEq:   false,
		},
		{
			name:     "verb trimmed within a rule - update",
			expected: role(podsReadOnly),
			current:  role(pods),
			wantEq:   false,
		},
		{
			name:     "rule added - update",
			expected: role(pods, leases),
			current:  role(pods),
			wantEq:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantEq, semanticRoleIsEqual(tt.expected, tt.current))
		})
	}
}
