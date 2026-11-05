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

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestManagedInferencePoolAPIBeforeGatewayAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ports []igwapi.Port
		group string
	}{
		{"single port retains legacy routing", []igwapi.Port{{Number: 8000}}, "inference.networking.x-k8s.io"},
		{"multiple ports require v1 routing", []igwapi.Port{{Number: 8000}, {Number: 8001}}, constants.InferencePoolV1APIGroupName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, gwapiv1.Install(scheme))
			require.NoError(t, igwapi.Install(scheme))
			r := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
			svc := invalidRouteService()
			svc.Status.Router = &v1alpha2.RouterStatus{}
			svc.Spec.Router.Scheduler = &v1alpha2.SchedulerSpec{Pool: &v1alpha2.InferencePoolSpec{Spec: &igwapi.InferencePoolSpec{TargetPorts: tc.ports}}}
			svc.Spec.Router.Route.HTTP.Spec.Rules[0].BackendRefs = []gwapiv1.HTTPBackendRef{{
				BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{
					Group: ptr.To(gwapiv1.Group(constants.InferencePoolV1APIGroupName)), Kind: ptr.To(gwapiv1.Kind("InferencePool")), Name: "test-llm-inference-pool",
				}},
			}}
			route, err := r.expectedHTTPRoute(t.Context(), svc, &Config{})
			require.NoError(t, err)
			require.Equal(t, gwapiv1.Group(tc.group), *route.Spec.Rules[0].BackendRefs[0].Group)
			if len(tc.ports) > 1 {
				// v1-only discovery must work for multi-port pools, before and after pool creation.
				for _, exists := range []bool{false, true} {
					if exists {
						require.NoError(t, r.Create(t.Context(), &igwapi.InferencePool{ObjectMeta: metav1.ObjectMeta{Name: "test-llm-inference-pool", Namespace: svc.Namespace}}))
					}
					require.NoError(t, r.EvaluateInferencePoolConditions(t.Context(), svc, nil))
					require.Equal(t, gwapiv1.Group(constants.InferencePoolV1APIGroupName), svc.Status.Router.Scheduler.InferencePool.Group)
				}
			}
		})
	}
}

func TestMultiPortPoolCannotUseLegacyReadiness(t *testing.T) {
	for _, multiPort := range []bool{false, true} {
		t.Run(map[bool]string{false: "single port", true: "multiple ports"}[multiPort], func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, igwapi.Install(scheme))
			require.NoError(t, igwapiv1alpha2.Install(scheme))
			svc := invalidRouteService()
			svc.Status.Router = &v1alpha2.RouterStatus{}
			ports := []igwapi.Port{{Number: 8000}}
			if multiPort {
				ports = append(ports, igwapi.Port{Number: 8001})
			}
			svc.Spec.Router.Scheduler = &v1alpha2.SchedulerSpec{Pool: &v1alpha2.InferencePoolSpec{Spec: &igwapi.InferencePoolSpec{TargetPorts: ports}}}
			meta := metav1.ObjectMeta{Name: "test-llm-inference-pool", Namespace: svc.Namespace, Generation: 1}
			legacy := &igwapiv1alpha2.InferencePool{ObjectMeta: meta, Status: igwapiv1alpha2.InferencePoolStatus{
				Parents: []igwapiv1alpha2.PoolStatus{{
					GatewayRef: igwapiv1alpha2.ParentGatewayReference{Name: "gateway"},
					Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue, ObservedGeneration: 1}},
				}},
			}}
			current := &igwapi.InferencePool{ObjectMeta: meta, Status: igwapi.InferencePoolStatus{
				Parents: []igwapi.ParentStatus{{
					ParentRef:  igwapi.ParentReference{Name: "gateway"},
					Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionFalse, Reason: "NotSupportedByGateway", ObservedGeneration: 1}},
				}},
			}}
			r := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(current, legacy).Build()}
			gateways := []ResolvedGateway{{Gateway: &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: svc.Namespace}}, ParentRef: gwapiv1.ParentReference{Name: "gateway"}}}
			require.NoError(t, r.EvaluateInferencePoolConditions(t.Context(), svc, gateways))
			condition := svc.Status.GetCondition(v1alpha2.InferencePoolReady)
			require.NotNil(t, condition)
			require.Equal(t, !multiPort, condition.IsTrue(), "a legacy gateway can only make the single-port service ready")
			group := constants.InferencePoolV1Alpha2APIGroupName
			if multiPort {
				group = constants.InferencePoolV1APIGroupName
			}
			require.Equal(t, gwapiv1.Group(group), svc.Status.Router.Scheduler.InferencePool.Group)
		})
	}
}
