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
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/constants"
)

func prefixRule(path string) gwapiv1.HTTPRouteRule {
	return gwapiv1.HTTPRouteRule{Matches: []gwapiv1.HTTPRouteMatch{{Path: &gwapiv1.HTTPPathMatch{
		Type: ptr.To(gwapiv1.PathMatchPathPrefix), Value: ptr.To(path),
	}}}}
}

func routeWithRules(name string, paths ...string) *gwapiv1.HTTPRoute {
	route := &gwapiv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: name}}
	for _, p := range paths {
		route.Spec.Rules = append(route.Spec.Rules, prefixRule(p))
	}
	return route
}

func TestHTTPRouteShardWriteOrder(t *testing.T) {
	tests := []struct {
		name    string
		stored  []*gwapiv1.HTTPRoute
		desired []*gwapiv1.HTTPRoute
		want    []int
	}{
		{
			name:    "rules moving forward write the higher shard first",
			stored:  []*gwapiv1.HTTPRoute{routeWithRules("r", "/a", "/b")},
			desired: []*gwapiv1.HTTPRoute{routeWithRules("r", "/new", "/a"), routeWithRules("r-1", "/b")},
			want:    []int{1, 0},
		},
		{
			name:    "rules moving back write the lower shard first",
			stored:  []*gwapiv1.HTTPRoute{routeWithRules("r", "/a", "/b"), routeWithRules("r-1", "/c")},
			desired: []*gwapiv1.HTTPRoute{routeWithRules("r", "/b", "/c"), routeWithRules("r-1", "/d")},
			want:    []int{0, 1},
		},
		{
			name:    "a fresh route uses the default order",
			desired: []*gwapiv1.HTTPRoute{routeWithRules("r", "/a"), routeWithRules("r-1", "/b")},
			want:    []int{1, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, httpRouteShardWriteOrder(tt.stored, tt.desired))
		})
	}
}

func TestMatchKeyAppliesDefaults(t *testing.T) {
	bare := gwapiv1.HTTPRouteMatch{
		Path:    &gwapiv1.HTTPPathMatch{Value: ptr.To("/x")},
		Headers: []gwapiv1.HTTPHeaderMatch{{Name: "X-Model", Value: "m"}},
	}
	defaulted := gwapiv1.HTTPRouteMatch{
		Path:    &gwapiv1.HTTPPathMatch{Type: ptr.To(gwapiv1.PathMatchPathPrefix), Value: ptr.To("/x")},
		Headers: []gwapiv1.HTTPHeaderMatch{{Type: ptr.To(gwapiv1.HeaderMatchExact), Name: "x-model", Value: "m"}},
	}
	assert.Equal(t, matchKey(bare), matchKey(defaulted))
	assert.Equal(t, matchKey(gwapiv1.HTTPRouteMatch{}), matchKey(*prefixRule("/").Matches[0].DeepCopy()))
}

// TestReconcileRouter_ShardDryRunRejectionWritesNothing covers a route split
// in two where the API server would reject the second shard: the first shard
// must not be rewritten, since the rules it would hand over would then be in
// no route at all.
func TestReconcileRouter_ShardDryRunRejectionWritesNothing(t *testing.T) {
	seedInferencePoolV1Alpha2Discovery(t)

	llmSvc := invalidRouteService()
	llmSvc.Spec.Annotations = map[string]string{constants.HTTPRouteMaxShardsAnnotationKey: "2"}
	rules := make([]gwapiv1.HTTPRouteRule, 0, 20)
	for i := range 20 {
		rules = append(rules, prefixRule(fmt.Sprintf("/r%d", i)))
	}
	llmSvc.Spec.Router.Route.HTTP.Spec.Rules = rules
	route := managedRoute(llmSvc)
	route.Spec.Rules = rules[:16]
	shard1 := managedHTTPRouteName(llmSvc.GetName(), 1)

	writes := 0
	isDryRun := func(dry []string) bool { return slices.Contains(dry, metav1.DryRunAll) }
	funcs := interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*gwapiv1.HTTPRoute); ok {
				o := &client.CreateOptions{}
				for _, opt := range opts {
					opt.ApplyToCreate(o)
				}
				if isDryRun(o.DryRun) && obj.GetName() == shard1 {
					return rejectedHeaderValue(shard1)
				}
				if !isDryRun(o.DryRun) {
					writes++
				}
			}
			return c.Create(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*gwapiv1.HTTPRoute); ok {
				o := &client.UpdateOptions{}
				for _, opt := range opts {
					opt.ApplyToUpdate(o)
				}
				if !isDryRun(o.DryRun) {
					writes++
				}
			}
			return c.Update(ctx, obj, opts...)
		},
	}
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{httpRouteGV})
	restMapper.Add(httpRouteGV.WithKind("HTTPRoute"), meta.RESTScopeNamespace)
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(invalidRouteScheme(t)).
			WithRESTMapper(restMapper).
			WithObjects(llmSvc, route).
			WithInterceptorFuncs(funcs).
			Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}

	err := reconciler.reconcileRouter(t.Context(), llmSvc, &Config{})
	require.ErrorIs(t, err, reconcile.TerminalError(nil))
	assert.Zero(t, writes, "no shard may be written when any shard fails its dry-run")

	condition := llmSvc.Status.GetCondition("HTTPRoutesReady")
	require.NotNil(t, condition)
	assert.Equal(t, "InvalidHTTPRoute", condition.Reason)
	assert.Contains(t, condition.Message, shard1)
	assert.Contains(t, condition.Message, "no HTTPRoute was written")
}

// TestManagedHTTPRoutesUsesNameAndOwnership checks that only routes at the
// expected shard names that the service controls are treated as its shards:
// an HTTPRoute that copies the labels, or sits at a shard name without being
// controlled by the service, must neither block finalization nor be deleted.
func TestManagedHTTPRoutesUsesNameAndOwnership(t *testing.T) {
	llmSvc := invalidRouteService()
	llmSvc.UID = "svc-uid"
	owned := func(name string) *gwapiv1.HTTPRoute {
		r := managedRoute(llmSvc)
		r.Name = name
		return r
	}
	base := owned(managedHTTPRouteName(llmSvc.Name, 0))
	shard2 := owned(managedHTTPRouteName(llmSvc.Name, 2))
	foreignLabels := &gwapiv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
		Name: "someone-else", Namespace: llmSvc.Namespace,
		Labels: map[string]string{constants.KubernetesAppNameLabelKey: llmSvc.Name, constants.LLMHTTPRouteShardLabelKey: "1"},
	}}
	foreignAtShardName := &gwapiv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
		Name: managedHTTPRouteName(llmSvc.Name, 1), Namespace: llmSvc.Namespace,
	}}

	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{httpRouteGV})
	restMapper.Add(httpRouteGV.WithKind("HTTPRoute"), meta.RESTScopeNamespace)
	c := fake.NewClientBuilder().
		WithScheme(invalidRouteScheme(t)).
		WithRESTMapper(restMapper).
		WithObjects(llmSvc, base, shard2, foreignLabels, foreignAtShardName).
		Build()

	routes, err := managedHTTPRoutes(t.Context(), c, llmSvc)
	require.NoError(t, err)
	names := make([]string, 0, len(routes))
	for _, r := range routes {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{base.Name, shard2.Name}, names)

	reconciler := &LLMISVCReconciler{Client: c, EventRecorder: record.NewFakeRecorder(10)}
	require.NoError(t, reconciler.deleteStaleHTTPRouteShards(t.Context(), llmSvc, 1))
	left := &gwapiv1.HTTPRouteList{}
	require.NoError(t, c.List(t.Context(), left))
	remaining := make([]string, 0, len(left.Items))
	for _, r := range left.Items {
		remaining = append(remaining, r.Name)
	}
	assert.ElementsMatch(t, []string{base.Name, foreignLabels.Name, foreignAtShardName.Name}, remaining,
		"only the owned stale shard is deleted")
}
