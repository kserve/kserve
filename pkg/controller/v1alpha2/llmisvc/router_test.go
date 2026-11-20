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

package llmisvc

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// TestExpectedHTTPRouteRouterShapes asserts that expectedHTTPRoute does not panic
// for any combination of nil/empty Router/Route/HTTP and that the produced
// HTTPRoute carries an inline spec only when one was supplied.
//
// Regression: the third case ("route set, http nil") used to panic at
// `llmSvc.Spec.Router.Route.HTTP.Spec != nil` because HTTP itself was nil.
// See https://github.com/kserve/kserve/issues/<TBD>.
func TestExpectedHTTPRouteRouterShapes(t *testing.T) {
	hostname := gwapiv1.Hostname("example.com")

	tests := []struct {
		name        string
		router      *v1alpha2.RouterSpec
		wantNonZero bool // true => returned HTTPRoute.Spec should carry the user-supplied spec
	}{
		{
			name:   "router nil",
			router: nil,
		},
		{
			name:   "route nil",
			router: &v1alpha2.RouterSpec{},
		},
		{
			// Regression: panic at router.go:203 before the fix.
			name:   "route set, http nil",
			router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{}},
		},
		{
			// Exact shape from the bug report: gateway/route/scheduler all set to
			// the empty object. With Scheduler != nil and Pool unset, expectedHTTPRoute
			// also exercises the v1alpha2/v1 InferencePool migration block (router.go
			// lines ~209-275), so this guards against a panic on that path too.
			name: "router with gateway, route, scheduler all empty (issue reproducer)",
			router: &v1alpha2.RouterSpec{
				Gateway:   &v1alpha2.GatewaySpec{},
				Route:     &v1alpha2.GatewayRoutesSpec{},
				Scheduler: &v1alpha2.SchedulerSpec{},
			},
		},
		{
			name: "route set, http empty",
			router: &v1alpha2.RouterSpec{
				Route: &v1alpha2.GatewayRoutesSpec{HTTP: &v1alpha2.HTTPRouteSpec{}},
			},
		},
		{
			name: "route set, http with refs only (BYO HTTPRoute)",
			router: &v1alpha2.RouterSpec{
				Route: &v1alpha2.GatewayRoutesSpec{HTTP: &v1alpha2.HTTPRouteSpec{
					Refs: []corev1.LocalObjectReference{{Name: "byo-route"}},
				}},
			},
		},
		{
			name: "route set, http with inline spec",
			router: &v1alpha2.RouterSpec{
				Route: &v1alpha2.GatewayRoutesSpec{HTTP: &v1alpha2.HTTPRouteSpec{
					Spec: &gwapiv1.HTTPRouteSpec{Hostnames: []gwapiv1.Hostname{hostname}},
				}},
			},
			wantNonZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			scheme := runtime.NewScheme()
			g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			g.Expect(gwapiv1.Install(scheme)).To(Succeed())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "my-llm", Namespace: "llm-demo"},
				Spec:       v1alpha2.LLMInferenceServiceSpec{Router: tt.router},
			}

			r := &LLMISVCReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			}

			var got *gwapiv1.HTTPRoute
			g.Expect(func() {
				got = r.expectedHTTPRoute(context.Background(), llmSvc)
			}).NotTo(Panic())

			g.Expect(got).NotTo(BeNil())
			g.Expect(got.Namespace).To(Equal("llm-demo"))
			g.Expect(got.OwnerReferences).To(HaveLen(1))
			g.Expect(got.OwnerReferences[0].Name).To(Equal("my-llm"))

			if tt.wantNonZero {
				g.Expect(got.Spec.Hostnames).To(ContainElement(hostname))
			} else {
				g.Expect(got.Spec.Hostnames).To(BeEmpty())
				g.Expect(got.Spec.Rules).To(BeEmpty())
			}
		})
	}
}
