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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	testGroup    = "group"
	testPeerSvc  = "peer-kserve-workload-svc"
	testPeerPool = "peer-inference-pool"
)

func TestRemoveTerminatingGroupBackends(t *testing.T) {
	for _, tt := range []struct {
		name                            string
		group, kind, backend, namespace string
		poolRef                         string
		active, forceStopped, remove    bool
	}{
		{name: "workload Service", backend: testPeerSvc, remove: true},
		{name: "v1 pool", group: constants.InferencePoolV1APIGroupName, kind: "InferencePool", backend: testPeerPool, remove: true},
		{name: "v1alpha2 pool", group: constants.InferencePoolV1Alpha2APIGroupName, kind: "InferencePool", backend: testPeerPool, remove: true},
		{name: "explicit own namespace", backend: testPeerSvc, namespace: "ns", remove: true},
		// A member using scheduler.pool.ref has that pool injected as its
		// backendRef, so cleanup must recognise it by the referenced name.
		{name: "explicitly referenced pool", kind: "InferencePool", backend: "user-pool", poolRef: "user-pool", remove: true},
		// Both names are recognised: a member that switched to a pool ref may still
		// have the controller-created default pool named in a peer's stored route.
		{name: "default pool name while a pool ref is set", kind: "InferencePool", backend: testPeerPool, poolRef: "user-pool", remove: true},
		// isDefaultBackendRef matches the default pool by kind and name only,
		// because the group is v1 or v1alpha2 depending on the cluster.
		{name: "pool in an unexpected group", group: "custom.example.com", kind: "InferencePool", backend: testPeerPool, remove: true},
		{name: "different namespace", backend: testPeerSvc, namespace: "elsewhere"},
		{name: "different kind", kind: "CustomBackend", backend: testPeerSvc},
		{name: "pool name in the core group", kind: "Service", backend: testPeerPool},
		{name: "unrelated Service", backend: "custom-service"},
		{name: "active member", backend: testPeerSvc, active: true},
		// Force-stop is orthogonal to deletion: a terminating member must be
		// pruned either way, or its finalizer never releases.
		{name: "force-stopped and terminating", backend: testPeerSvc, forceStopped: true, remove: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			member := v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "peer", Namespace: "ns"}}
			if !tt.active {
				member.DeletionTimestamp = ptr.To(metav1.Now())
			}
			if tt.forceStopped {
				member.Annotations = map[string]string{constants.StopAnnotationKey: "true"}
			}
			if tt.poolRef != "" {
				member.Spec.Router = &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{Ref: &corev1.LocalObjectReference{Name: tt.poolRef}},
				}}
			}

			ref := gwapiv1.HTTPBackendRef{BackendRef: gwapiv1.BackendRef{
				BackendObjectReference: gwapiv1.BackendObjectReference{Name: gwapiv1.ObjectName(tt.backend)},
				Weight:                 ptr.To[int32](40),
			}}
			if tt.group != "" {
				ref.Group = ptr.To(gwapiv1.Group(tt.group))
			}
			if tt.kind != "" {
				ref.Kind = ptr.To(gwapiv1.Kind(tt.kind))
			}
			if tt.namespace != "" {
				ref.Namespace = ptr.To(gwapiv1.Namespace(tt.namespace))
			}

			route := &gwapiv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "ns"}, Spec: gwapiv1.HTTPRouteSpec{
				Hostnames: []gwapiv1.Hostname{"models.example.com"},
				Rules: []gwapiv1.HTTPRouteRule{{
					Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{
						Type: ptr.To(gwapiv1.PathMatchPathPrefix), Value: ptr.To("/"),
					}}},
					BackendRefs: []gwapiv1.HTTPBackendRef{ref, {BackendRef: gwapiv1.BackendRef{
						BackendObjectReference: gwapiv1.BackendObjectReference{Name: "survivor"},
						Weight:                 ptr.To[int32](60),
					}}},
				}},
			}}

			want := route.DeepCopy()
			if tt.remove {
				want.Spec.Rules[0].BackendRefs = want.Spec.Rules[0].BackendRefs[1:]
			}

			removed := removeTerminatingGroupBackends(route, []v1alpha2.LLMInferenceService{member})
			assert.Equal(t, want, route, "matches, hostnames and surviving weights must be untouched")
			if tt.remove {
				assert.Equal(t, []string{"peer"}, removed)
			} else {
				assert.Empty(t, removed)
			}

			assert.Empty(t, removeTerminatingGroupBackends(route, []v1alpha2.LLMInferenceService{member}),
				"pruning must be idempotent")
			assert.Equal(t, want, route)
		})
	}
}

func TestGroupBackendCleanupSkipsUnownedAndAbsentRoutes(t *testing.T) {
	grouped := func() *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: "owner-uid"},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{Group: ptr.To(testGroup)}},
			},
		}
	}
	noKindMatch := &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: gwapiv1.GroupName, Kind: "HTTPRoute"}}

	t.Run("non-grouped service reads nothing", func(t *testing.T) {
		svc := grouped()
		svc.Spec.Router.Route.Group = nil
		c := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				t.Fatal("a non-grouped service must not read any route")
				return nil
			},
		}).Build()
		require.NoError(t, (&LLMISVCReconciler{Client: c}).reconcileTerminatingGroupBackends(t.Context(), svc))
	})

	t.Run("absent HTTPRoute CRD", func(t *testing.T) {
		c := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
				require.IsType(t, &gwapiv1.HTTPRoute{}, obj)
				return noKindMatch
			},
		}).Build()
		require.NoError(t, (&LLMISVCReconciler{Client: c}).reconcileTerminatingGroupBackends(t.Context(), grouped()),
			"an absent optional CRD means there are no routes to prune")
	})

	t.Run("route not created yet", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(cleanupScheme(t)).Build()
		require.NoError(t, (&LLMISVCReconciler{Client: c}).reconcileTerminatingGroupBackends(t.Context(), grouped()))
	})

	t.Run("forbidden stays an error", func(t *testing.T) {
		denied := apierrors.NewForbidden(schema.GroupResource{Group: gwapiv1.GroupName, Resource: "httproutes"}, "route", errors.New("denied"))
		c := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return denied
			},
		}).Build()
		err := (&LLMISVCReconciler{Client: c}).reconcileTerminatingGroupBackends(t.Context(), grouped())
		require.True(t, apierrors.IsForbidden(err), "an RBAC denial is not absence: %v", err)
	})

	t.Run("route without a group label", func(t *testing.T) {
		owner := grouped()
		route := groupRoute(owner, gwapiv1.HTTPBackendRef{BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{Name: testPeerSvc},
		}})
		route.Labels = nil
		c := fake.NewClientBuilder().WithScheme(cleanupScheme(t)).WithObjects(route).Build()
		require.NoError(t, (&LLMISVCReconciler{Client: c}).reconcileTerminatingGroupBackends(t.Context(), owner))

		current := &gwapiv1.HTTPRoute{}
		require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(route), current))
		require.Len(t, current.Spec.Rules[0].BackendRefs, 1, "an unlabelled route is not a group route")
	})

	t.Run("route owned by somebody else", func(t *testing.T) {
		owner := grouped()
		other := grouped()
		other.Name, other.UID = "intruder", "intruder-uid"
		route := groupRoute(owner)
		route.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(other, v1alpha2.LLMInferenceServiceGVK)}
		c := fake.NewClientBuilder().WithScheme(cleanupScheme(t)).WithObjects(route).Build()

		err := (&LLMISVCReconciler{Client: c}).reconcileTerminatingGroupBackends(t.Context(), owner)
		require.ErrorContains(t, err, "not controlled by this service")
	})

	t.Run("list failure is retryable", func(t *testing.T) {
		owner := grouped()
		c := fake.NewClientBuilder().WithScheme(cleanupScheme(t)).WithObjects(groupRoute(owner)).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return apierrors.NewServiceUnavailable("apiserver is down")
				},
			}).Build()

		err := (&LLMISVCReconciler{Client: c}).reconcileTerminatingGroupBackends(t.Context(), owner)
		require.True(t, apierrors.IsServiceUnavailable(err), "got: %v", err)
	})
}

func TestGroupBackendCleanupConvergesAfterConflict(t *testing.T) {
	owner := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "ns", UID: "owner-uid"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{Group: ptr.To(testGroup)}},
		},
	}
	peer := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "peer", Namespace: "ns",
			DeletionTimestamp: ptr.To(metav1.Now()), Finalizers: []string{"test-finalizer"},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{Group: ptr.To(testGroup)}},
		},
	}
	peerRef := gwapiv1.BackendObjectReference{Name: testPeerSvc}
	memberStatus := func() []v1alpha2.GroupMemberStatus {
		return []v1alpha2.GroupMemberStatus{{Name: peer.Name, BackendRef: &peerRef}}
	}
	owner.Status.Router = &v1alpha2.RouterStatus{
		Group: &v1alpha2.GroupStatus{Name: testGroup, Members: memberStatus()},
	}

	route := groupRoute(owner,
		gwapiv1.HTTPBackendRef{BackendRef: gwapiv1.BackendRef{BackendObjectReference: peerRef}},
		gwapiv1.HTTPBackendRef{BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{Name: "owner-kserve-workload-svc"},
		}})

	patches := 0
	c := fake.NewClientBuilder().WithScheme(cleanupScheme(t)).WithObjects(owner, peer, route).
		WithIndex(&v1alpha2.LLMInferenceService{}, groupFieldIndex, groupIndexValue).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				patches++
				if patches == 1 {
					return apierrors.NewConflict(
						schema.GroupResource{Group: gwapiv1.GroupName, Resource: "httproutes"}, obj.GetName(), nil)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	r := &LLMISVCReconciler{Client: c}
	ctx := t.Context()

	err := r.reconcileTerminatingGroupBackends(ctx, owner)
	require.True(t, apierrors.IsConflict(err), "conflicts must reach controller-runtime for retry: %v", err)
	require.Len(t, owner.Status.Router.Group.Members, 1, "uncommitted pruning must not be published as applied")

	require.NoError(t, r.reconcileTerminatingGroupBackends(ctx, owner))
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(route), route))
	require.Len(t, route.Spec.Rules[0].BackendRefs, 1)
	require.Empty(t, owner.Status.Router.Group.Members)

	// A route patch that lands while the parent status write fails leaves stale
	// membership behind; the next no-op cleanup must still repair it.
	owner.Status.Router.Group.Members = memberStatus()
	require.NoError(t, r.reconcileTerminatingGroupBackends(ctx, owner))
	assert.Empty(t, owner.Status.Router.Group.Members)
	assert.Equal(t, 2, patches, "a route with nothing left to prune must not be patched again")
}

func cleanupScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	require.NoError(t, gwapiv1.Install(scheme))
	return scheme
}

// groupRoute builds the managed group HTTPRoute owned by owner.
func groupRoute(owner *v1alpha2.LLMInferenceService, refs ...gwapiv1.HTTPBackendRef) *gwapiv1.HTTPRoute {
	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:            kmeta.ChildName(owner.Name, "-kserve-route"),
			Namespace:       owner.Namespace,
			Labels:          map[string]string{constants.LLMRoutingGroupLabelKey: testGroup},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(owner, v1alpha2.LLMInferenceServiceGVK)},
		},
		Spec: gwapiv1.HTTPRouteSpec{Rules: []gwapiv1.HTTPRouteRule{{BackendRefs: refs}}},
	}
}
