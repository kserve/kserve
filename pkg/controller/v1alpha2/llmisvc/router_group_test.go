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
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	v1alpha2 "github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestRouterSpecHasGroup(t *testing.T) {
	tests := []struct {
		name   string
		llmSvc *v1alpha2.LLMInferenceService
		want   bool
	}{
		{
			name:   "nil router",
			llmSvc: &v1alpha2.LLMInferenceService{},
			want:   false,
		},
		{
			name: "nil route",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{},
				},
			},
			want: false,
		},
		{
			name: "no group",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{
							Weight: ptr.To(int32(1)),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "group set",
			llmSvc: &v1alpha2.LLMInferenceService{
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Route: &v1alpha2.GatewayRoutesSpec{
							Group:  ptr.To("llama-70b"),
							Weight: ptr.To(int32(9)),
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.llmSvc.Spec.Router.HasGroup())
		})
	}
}

func TestFilterEligibleMembers(t *testing.T) {
	now := metav1.Now()

	t.Run("excludes terminating members", func(t *testing.T) {
		m := routableMember("v1", "llama-70b", 9, now)
		m.DeletionTimestamp = &now
		members := []v1alpha2.LLMInferenceService{
			m,
			routableMember("v2", "llama-70b", 1, now),
		}
		eligible := filterEligibleMembers(members, "self")
		require.Len(t, eligible, 1)
		assert.Equal(t, "v2", eligible[0].Name)
	})

	t.Run("excludes peer without RouterReady", func(t *testing.T) {
		peer := memberSvc("v2", "llama-70b", 1, false, now)
		members := []v1alpha2.LLMInferenceService{
			routableMember("v1", "llama-70b", 9, now),
			peer,
		}
		eligible := filterEligibleMembers(members, "self")
		require.Len(t, eligible, 1)
		assert.Equal(t, "v1", eligible[0].Name)
	})

	t.Run("excludes peer with zero available replicas", func(t *testing.T) {
		peer := routableMember("v2", "llama-70b", 1, now)
		peer.Status.Workloads.Primary.ReadyReplicas = ptr.To(int32(0))
		members := []v1alpha2.LLMInferenceService{
			routableMember("v1", "llama-70b", 9, now),
			peer,
		}
		eligible := filterEligibleMembers(members, "self")
		require.Len(t, eligible, 1)
		assert.Equal(t, "v1", eligible[0].Name)
	})

	t.Run("self always included regardless of status", func(t *testing.T) {
		self := memberSvc("self", "llama-70b", 9, false, now)
		members := []v1alpha2.LLMInferenceService{self}
		eligible := filterEligibleMembers(members, "self")
		require.Len(t, eligible, 1)
		assert.Equal(t, "self", eligible[0].Name)
	})

	t.Run("excludes peer with zero prefill replicas", func(t *testing.T) {
		peer := routableMember("v2", "llama-70b", 1, now)
		peer.Status.Workloads.Prefill = &v1alpha2.ObservedWorkloadStatus{
			TypedLocalObjectReference: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To("apps"), Kind: "Deployment", Name: "v2-prefill",
			},
			ReadyReplicas: ptr.To(int32(0)),
		}
		members := []v1alpha2.LLMInferenceService{
			routableMember("v1", "llama-70b", 9, now),
			peer,
		}
		eligible := filterEligibleMembers(members, "self")
		require.Len(t, eligible, 1)
		assert.Equal(t, "v1", eligible[0].Name)
	})

	t.Run("excludes peer with nil scheduler replicas", func(t *testing.T) {
		peer := routableMember("v2", "llama-70b", 1, now)
		peer.Status.Workloads.Scheduler = &v1alpha2.ObservedWorkloadStatus{
			TypedLocalObjectReference: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To("apps"), Kind: "Deployment", Name: "v2-scheduler",
			},
		}
		members := []v1alpha2.LLMInferenceService{
			routableMember("v1", "llama-70b", 9, now),
			peer,
		}
		eligible := filterEligibleMembers(members, "self")
		require.Len(t, eligible, 1)
		assert.Equal(t, "v1", eligible[0].Name)
	})

	t.Run("includes force-stopped peer for group status visibility", func(t *testing.T) {
		peer := memberSvc("v2", "llama-70b", 1, true, now)
		peer.Status.Workloads = nil
		members := []v1alpha2.LLMInferenceService{
			routableMember("v1", "llama-70b", 9, now),
			peer,
		}
		eligible := filterEligibleMembers(members, "self")
		require.Len(t, eligible, 2)
	})

	t.Run("includes routable peers", func(t *testing.T) {
		members := []v1alpha2.LLMInferenceService{
			routableMember("v1", "llama-70b", 9, now),
			routableMember("v2", "llama-70b", 1, now),
		}
		eligible := filterEligibleMembers(members, "v1")
		assert.Len(t, eligible, 2)
	})
}

func TestIsGroupRoute(t *testing.T) {
	tests := []struct {
		name  string
		route *gwapiv1.HTTPRoute
		want  bool
	}{
		{
			name:  "nil route",
			route: nil,
			want:  false,
		},
		{
			name:  "no labels",
			route: &gwapiv1.HTTPRoute{},
			want:  false,
		},
		{
			name: "no group label",
			route: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"other": "label"},
				},
			},
			want: false,
		},
		{
			name: "has group label",
			route: &gwapiv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.LLMRoutingGroupLabelKey: "llama-70b",
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isGroupRoute(tt.route))
		})
	}
}

func TestUpdateGroupStatus(t *testing.T) {
	t.Run("populates group status from resolved members", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Route: &v1alpha2.GatewayRoutesSpec{
						Group: ptr.To("llama-70b"),
					},
				},
			},
		}

		resolved := []resolvedMember{
			{name: "v1", weight: 9, backendRef: gwapiv1.BackendObjectReference{Name: "v1-pool"}},
			{name: "v2", weight: 1, backendRef: gwapiv1.BackendObjectReference{Name: "v2-pool"}},
		}

		updateGroupStatus(llmSvc, resolved)

		require.NotNil(t, llmSvc.Status.Router)
		require.NotNil(t, llmSvc.Status.Router.Group)
		assert.Equal(t, "llama-70b", llmSvc.Status.Router.Group.Name)
		require.Len(t, llmSvc.Status.Router.Group.Members, 2)
		assert.Equal(t, "v1", llmSvc.Status.Router.Group.Members[0].Name)
		assert.Equal(t, int32(9), llmSvc.Status.Router.Group.Members[0].Weight)
		assert.Equal(t, "v2", llmSvc.Status.Router.Group.Members[1].Name)
		assert.Equal(t, int32(1), llmSvc.Status.Router.Group.Members[1].Weight)
	})

	t.Run("creates RouterStatus if nil", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{
					Route: &v1alpha2.GatewayRoutesSpec{
						Group: ptr.To("g"),
					},
				},
			},
		}

		updateGroupStatus(llmSvc, []resolvedMember{{name: "v1", weight: 1}})

		require.NotNil(t, llmSvc.Status.Router)
		assert.Equal(t, "g", llmSvc.Status.Router.Group.Name)
	})
}

func TestRewriteRulesForGroup(t *testing.T) {
	t.Run("replaces controller-managed backendRefs with group members", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "my-svc"},
		}
		route := &gwapiv1.HTTPRoute{
			Spec: gwapiv1.HTTPRouteSpec{
				Rules: []gwapiv1.HTTPRouteRule{
					{
						BackendRefs: []gwapiv1.HTTPBackendRef{
							{BackendRef: gwapiv1.BackendRef{
								BackendObjectReference: gwapiv1.BackendObjectReference{
									Kind: ptr.To(gwapiv1.Kind("InferencePool")),
									Name: "my-svc-inference-pool",
								},
							}},
						},
					},
				},
			},
		}

		members := []resolvedMember{
			{name: "v1", weight: 9, backendRef: gwapiv1.BackendObjectReference{Name: "v1-pool"}},
			{name: "v2", weight: 1, backendRef: gwapiv1.BackendObjectReference{Name: "v2-pool"}},
		}

		rewriteRulesForGroup(route, llmSvc, members)

		require.Len(t, route.Spec.Rules[0].BackendRefs, 2)
		assert.Equal(t, gwapiv1.ObjectName("v1-pool"), route.Spec.Rules[0].BackendRefs[0].Name)
		assert.Equal(t, int32(9), *route.Spec.Rules[0].BackendRefs[0].Weight)
		assert.Equal(t, gwapiv1.ObjectName("v2-pool"), route.Spec.Rules[0].BackendRefs[1].Name)
		assert.Equal(t, int32(1), *route.Spec.Rules[0].BackendRefs[1].Weight)
	})

	t.Run("skips rules with only custom backendRefs", func(t *testing.T) {
		llmSvc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "my-svc"},
		}
		route := &gwapiv1.HTTPRoute{
			Spec: gwapiv1.HTTPRouteSpec{
				Rules: []gwapiv1.HTTPRouteRule{
					{BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{
						BackendObjectReference: gwapiv1.BackendObjectReference{
							Kind: ptr.To(gwapiv1.Kind("InferencePool")),
							Name: "my-svc-inference-pool",
						},
					}}}},
					{BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{
						BackendObjectReference: gwapiv1.BackendObjectReference{
							Kind: ptr.To(gwapiv1.Kind("Service")),
							Name: "user-custom-svc",
						},
					}}}},
				},
			},
		}

		members := []resolvedMember{
			{name: "v1", weight: 5, backendRef: gwapiv1.BackendObjectReference{Name: "v1-pool"}},
		}

		rewriteRulesForGroup(route, llmSvc, members)

		require.Len(t, route.Spec.Rules[0].BackendRefs, 1, "controller-managed rule should be rewritten")
		assert.Equal(t, gwapiv1.ObjectName("v1-pool"), route.Spec.Rules[0].BackendRefs[0].Name)

		require.Len(t, route.Spec.Rules[1].BackendRefs, 1, "custom rule should be untouched")
		assert.Equal(t, gwapiv1.ObjectName("user-custom-svc"), route.Spec.Rules[1].BackendRefs[0].Name)
	})
}

func TestIsExpectedBackendRef(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-svc"},
	}
	svcWithScheduler := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-svc"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "custom-pool"},
					},
				},
			},
		},
	}

	tests := []struct {
		name string
		svc  *v1alpha2.LLMInferenceService
		ref  gwapiv1.BackendRef
		want bool
	}{
		{
			name: "default InferencePool matches",
			svc:  svc,
			ref:  gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Kind: ptr.To(gwapiv1.Kind("InferencePool")), Name: "my-svc-inference-pool"}},
			want: true,
		},
		{
			name: "workload Service matches",
			svc:  svc,
			ref:  gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Kind: ptr.To(gwapiv1.Kind("Service")), Name: gwapiv1.ObjectName(workloadServiceName(svc))}},
			want: true,
		},
		{
			name: "custom pool ref matches",
			svc:  svcWithScheduler,
			ref:  gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Kind: ptr.To(gwapiv1.Kind("InferencePool")), Name: "custom-pool"}},
			want: true,
		},
		{
			name: "user InferencePool does not match",
			svc:  svc,
			ref:  gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Kind: ptr.To(gwapiv1.Kind("InferencePool")), Name: "user-custom-pool"}},
			want: false,
		},
		{
			name: "user Service does not match",
			svc:  svc,
			ref:  gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Kind: ptr.To(gwapiv1.Kind("Service")), Name: "user-custom-svc"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isExpectedBackendRef(tt.svc, tt.ref))
		})
	}
}

func TestTrafficFieldsChanged(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name string
		old  *v1alpha2.LLMInferenceService
		new  *v1alpha2.LLMInferenceService
		want bool
	}{
		{
			name: "no traffic fields on either",
			old:  &v1alpha2.LLMInferenceService{},
			new:  &v1alpha2.LLMInferenceService{},
			want: false,
		},
		{
			name: "group added",
			old:  &v1alpha2.LLMInferenceService{},
			new:  &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{Group: ptr.To("g")}}}},
			want: true,
		},
		{
			name: "group removed",
			old:  &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{Group: ptr.To("g")}}}},
			new:  &v1alpha2.LLMInferenceService{},
			want: true,
		},
		{
			name: "weight changed",
			old:  &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{Group: ptr.To("g"), Weight: ptr.To(int32(9))}}}},
			new:  &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{Group: ptr.To("g"), Weight: ptr.To(int32(5))}}}},
			want: true,
		},
		{
			name: "force-stop changed",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, true, now); return &s }(),
			want: true,
		},
		{
			name: "model name changed on grouped member",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.Model.Name = ptr.To("different")
				return &s
			}(),
			want: true,
		},
		{
			name: "LoRA adapters changed on grouped member",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.Model.LoRA = &v1alpha2.LoRASpec{
					Adapters: []v1alpha2.LLMModelSpec{{Name: ptr.To("new-adapter")}},
				}
				return &s
			}(),
			want: true,
		},
		{
			name: "baseRefs changed on grouped member",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.BaseRefs = []corev1.LocalObjectReference{{Name: "new-config"}}
				return &s
			}(),
			want: true,
		},
		{
			name: "scheduler added on grouped member",
			old: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.Router.Scheduler = nil
				return &s
			}(),
			new:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			want: true,
		},
		{
			name: "scheduler removed on grouped member",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.Router.Scheduler = nil
				return &s
			}(),
			want: true,
		},
		{
			name: "scheduler pool ref changed on grouped member",
			old: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{
					Ref: &corev1.LocalObjectReference{Name: "old-pool"},
				}
				return &s
			}(),
			new: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{
					Ref: &corev1.LocalObjectReference{Name: "new-pool"},
				}
				return &s
			}(),
			want: true,
		},
		{
			name: "scheduler pool ref added on grouped member",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new: func() *v1alpha2.LLMInferenceService {
				s := memberSvc("v1", "g", 9, false, now)
				s.Spec.Router.Scheduler.Pool = &v1alpha2.InferencePoolSpec{
					Ref: &corev1.LocalObjectReference{Name: "custom-pool"},
				}
				return &s
			}(),
			want: true,
		},
		{
			name: "peer becomes routable",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new:  func() *v1alpha2.LLMInferenceService { s := routableMember("v1", "g", 9, now); return &s }(),
			want: true,
		},
		{
			name: "peer becomes unroutable (replicas drop to 0)",
			old:  func() *v1alpha2.LLMInferenceService { s := routableMember("v1", "g", 9, now); return &s }(),
			new: func() *v1alpha2.LLMInferenceService {
				s := routableMember("v1", "g", 9, now)
				s.Status.Workloads.Primary.ReadyReplicas = ptr.To(int32(0))
				return &s
			}(),
			want: true,
		},
		{
			name: "replica count change without routable transition",
			old:  func() *v1alpha2.LLMInferenceService { s := routableMember("v1", "g", 9, now); return &s }(),
			new: func() *v1alpha2.LLMInferenceService {
				s := routableMember("v1", "g", 9, now)
				s.Status.Workloads.Primary.ReadyReplicas = ptr.To(int32(3))
				return &s
			}(),
			want: false,
		},
		{
			name: "non-traffic change on grouped member",
			old:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			new:  func() *v1alpha2.LLMInferenceService { s := memberSvc("v1", "g", 9, false, now); return &s }(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, trafficFieldsChanged(tt.old, tt.new))
		})
	}
}

func TestResolveMemberBackendRef(t *testing.T) {
	tests := []struct {
		name    string
		member  *v1alpha2.LLMInferenceService
		want    gwapiv1.BackendObjectReference
		wantErr bool
	}{
		{
			name: "custom pool ref from spec uses well-known API group",
			member: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "v1", Namespace: "default"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{
						Scheduler: &v1alpha2.SchedulerSpec{
							Pool: &v1alpha2.InferencePoolSpec{
								Ref: &corev1.LocalObjectReference{Name: "my-custom-pool"},
							},
						},
					},
				},
			},
			want: gwapiv1.BackendObjectReference{
				Group: ptr.To(gwapiv1.Group(constants.InferencePoolV1Alpha2APIGroupName)),
				Kind:  ptr.To(gwapiv1.Kind("InferencePool")),
				Name:  "my-custom-pool",
				Port:  ptr.To(gwapiv1.PortNumber(8000)),
			},
		},
		{
			name: "managed pool from status uses pool's own API group",
			member: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "v1", Namespace: "default"},
				Status: v1alpha2.LLMInferenceServiceStatus{
					Router: &v1alpha2.RouterStatus{
						Scheduler: &v1alpha2.ObservedSchedulerStatus{
							InferencePool: &gwapiv1.ObjectReference{
								Group: gwapiv1.Group(constants.InferencePoolV1APIGroupName),
								Kind:  "InferencePool",
								Name:  "v1-inference-pool",
							},
						},
					},
				},
			},
			want: gwapiv1.BackendObjectReference{
				Group: ptr.To(gwapiv1.Group(constants.InferencePoolV1APIGroupName)),
				Kind:  ptr.To(gwapiv1.Kind("InferencePool")),
				Name:  "v1-inference-pool",
				Port:  ptr.To(gwapiv1.PortNumber(8000)),
			},
		},
		{
			name: "workload service fallback",
			member: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
			},
			want: gwapiv1.BackendObjectReference{
				Kind: ptr.To(gwapiv1.Kind("Service")),
				Name: gwapiv1.ObjectName(workloadServiceName(&v1alpha2.LLMInferenceService{
					ObjectMeta: metav1.ObjectMeta{Name: "my-svc"},
				})),
				Port: ptr.To(gwapiv1.PortNumber(8000)),
			},
		},
		{
			// The error branch in resolveMemberBackendRef is defensive: kmeta.ChildName
			// always returns non-empty for the "-kserve-workload-svc" suffix, so it
			// falls through to the workload service path even with an empty name.
			name: "empty name still resolves via workload service",
			member: &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: "default"},
			},
			want: gwapiv1.BackendObjectReference{
				Kind: ptr.To(gwapiv1.Kind("Service")),
				Name: gwapiv1.ObjectName(workloadServiceName(&v1alpha2.LLMInferenceService{})),
				Port: ptr.To(gwapiv1.PortNumber(8000)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMemberBackendRef(tt.member)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRouteReferencesBackend(t *testing.T) {
	tests := []struct {
		name    string
		route   *gwapiv1.HTTPRoute
		backend string
		want    bool
	}{
		{
			name: "match in first rule",
			route: &gwapiv1.HTTPRoute{Spec: gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{
				{BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Name: "pool-a"}}}}},
			}}},
			backend: "pool-a",
			want:    true,
		},
		{
			name: "no match",
			route: &gwapiv1.HTTPRoute{Spec: gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{
				{BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Name: "pool-a"}}}}},
			}}},
			backend: "pool-b",
			want:    false,
		},
		{
			name:    "empty rules",
			route:   &gwapiv1.HTTPRoute{},
			backend: "pool-a",
			want:    false,
		},
		{
			name: "match in second rule",
			route: &gwapiv1.HTTPRoute{Spec: gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{
				{BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Name: "pool-a"}}}}},
				{BackendRefs: []gwapiv1.HTTPBackendRef{{BackendRef: gwapiv1.BackendRef{BackendObjectReference: gwapiv1.BackendObjectReference{Name: "pool-b"}}}}},
			}}},
			backend: "pool-b",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, routeReferencesBackend(tt.route, tt.backend))
		})
	}
}

// memberSvc creates a minimal LLMInferenceService for group testing.
func routableMember(name, group string, weight int32, ts metav1.Time) v1alpha2.LLMInferenceService {
	svc := memberSvc(name, group, weight, false, ts)
	svc.Status.SetConditions(apis.Conditions{{
		Type:   v1alpha2.RouterReady,
		Status: corev1.ConditionTrue,
	}})
	svc.Status.Workloads = &v1alpha2.WorkloadStatus{
		Primary: &v1alpha2.ObservedWorkloadStatus{
			TypedLocalObjectReference: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To("apps"), Kind: "Deployment", Name: name + "-kserve",
			},
			ReadyReplicas: ptr.To(int32(1)),
		},
	}
	return svc
}

func memberSvc(name, group string, weight int32, stopped bool, ts metav1.Time) v1alpha2.LLMInferenceService {
	svc := v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: ts,
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				Name: ptr.To("llama-70b"),
			},
			Router: &v1alpha2.RouterSpec{
				Route: &v1alpha2.GatewayRoutesSpec{
					Group:  ptr.To(group),
					Weight: ptr.To(weight),
				},
				Scheduler: &v1alpha2.SchedulerSpec{},
			},
		},
	}
	if stopped {
		svc.Annotations = map[string]string{
			constants.StopAnnotationKey: "true",
		}
	}
	return svc
}
