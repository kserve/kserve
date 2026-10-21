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
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestPackInlineRouteForValidation(t *testing.T) {
	spec := &gwapiv1.HTTPRouteSpec{Rules: append(repeatRules(17, 1), pathRule("lora", 70))}
	spec.Hostnames = []gwapiv1.Hostname{"example.com"}
	routes, sources := packInlineRouteForValidation("ns", "svc", spec)
	require.Len(t, routes, 3)
	for _, r := range routes {
		assertWithinGatewayLimits(t, r.Spec.Rules)
		require.Equal(t, "ns", r.Namespace)
		require.Equal(t, spec.Hostnames, r.Spec.Hostnames, "route level fields are kept")
	}
	require.Equal(t, "inline route rules 0-15", describeSourceRange(sources[0]))
	require.Equal(t, "inline route rules 16-17", describeSourceRange(sources[1]))
	require.Equal(t, "inline route rule 17", describeSourceRange(sources[2]))
}

func TestInlineRouteDryRun(t *testing.T) {
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{httpRouteGV})
	restMapper.Add(httpRouteGV.WithKind("HTTPRoute"), meta.RESTScopeNamespace)
	spec := &gwapiv1.HTTPRouteSpec{Rules: repeatRules(20, 1)}

	t.Run("every route is a dry-run create and nothing is stored", func(t *testing.T) {
		var created []string
		c := fake.NewClientBuilder().WithScheme(invalidRouteScheme(t)).WithRESTMapper(restMapper).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					o := &client.CreateOptions{}
					for _, opt := range opts {
						opt.ApplyToCreate(o)
					}
					require.True(t, slices.Contains(o.DryRun, metav1.DryRunAll), "only dry-run creates are allowed")
					created = append(created, obj.GetName())
					return nil
				},
			}).Build()
		require.NoError(t, NewInlineRouteDryRun(c)(t.Context(), "ns", "svc", spec))
		require.Len(t, created, 2)
	})

	t.Run("a rejection names the source rules", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(invalidRouteScheme(t)).WithRESTMapper(restMapper).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
					if obj.GetName() == "svc-kserve-route-validate-1" {
						return rejectedHeaderValue(obj.GetName())
					}
					return nil
				},
			}).Build()
		err := NewInlineRouteDryRun(c)(t.Context(), "ns", "svc", spec)
		require.ErrorContains(t, err, "inline route rules 16-19")
	})

	t.Run("no Gateway API means nothing to validate", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(invalidRouteScheme(t)).WithRESTMapper(restMapper).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
					return &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: httpRouteGV.Group, Kind: "HTTPRoute"}}
				},
			}).Build()
		require.NoError(t, NewInlineRouteDryRun(c)(t.Context(), "ns", "svc", spec))
	})
}
