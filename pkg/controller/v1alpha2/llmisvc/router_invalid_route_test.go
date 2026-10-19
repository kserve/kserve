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
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	igwv1alpha2pool "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	kservescheme "github.com/kserve/kserve/pkg/scheme"
	"github.com/kserve/kserve/pkg/utils"
)

// seedInferencePoolV1Alpha2Discovery stands in for the rest.Config the scheduler needs to
// probe the v1alpha2 InferencePool CRD. The probe reads a process-global cache, and
// whichever test seeds it first decides for the whole binary, so the verdict has to match
// the one the envtest suites in this package reach - they install the CRD from
// test/crds/gateway-inference-extension-v1alpha2pool.yaml.
func seedInferencePoolV1Alpha2Discovery(t *testing.T) {
	t.Helper()

	utils.SetAvailableResourcesForApi(igwv1alpha2pool.GroupVersion.String(), &metav1.APIResourceList{
		GroupVersion: igwv1alpha2pool.GroupVersion.String(),
		APIResources: []metav1.APIResource{{Kind: "InferencePool"}},
	})
}

// httpRouteGV is gwapiv1.GroupVersion in the schema flavour the RESTMapper and the
// apierrors constructors take.
var httpRouteGV = schema.GroupVersion{Group: gwapiv1.GroupVersion.Group, Version: gwapiv1.GroupVersion.Version}

// invalidRouteScheme covers everything reconcileRouter touches before it reaches the
// managed route: the scheduler sub-reconcilers delete their resources when
// spec.router.scheduler is unset, and a missing kind fails the client, not the Get.
func invalidRouteScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, kservescheme.AddKServeAPIs(scheme))
	require.NoError(t, kservescheme.AddCoreKubernetesAPIs(scheme))
	require.NoError(t, kservescheme.AddGatewayAPIs(scheme))
	return scheme
}

// invalidRouteService is a service with a managed route and no scheduler, gateway refs
// or route refs, so reference validation passes and the managed route is the only thing
// reconcileRouter writes.
func invalidRouteService() *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "test-ns"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			Router: &v1alpha2.RouterSpec{
				Route: &v1alpha2.GatewayRoutesSpec{
					HTTP: &v1alpha2.HTTPRouteSpec{
						Spec: &gwapiv1.HTTPRouteSpec{
							Rules: []gwapiv1.HTTPRouteRule{{
								Matches: []gwapiv1.HTTPRouteMatch{{
									Path: &gwapiv1.HTTPPathMatch{
										Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
										Value: ptr.To("/"),
									},
								}},
							}},
						},
					},
				},
			},
		},
	}
}

// managedRoute is the route reconcileHTTPRoutes owns, seeded so the reconciler takes the
// update path and its dry-run reaches the API server.
func managedRoute(llmSvc *v1alpha2.LLMInferenceService) *gwapiv1.HTTPRoute {
	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:            kmeta.ChildName(llmSvc.GetName(), "-kserve-route"),
			Namespace:       llmSvc.GetNamespace(),
			Labels:          RouterLabels(llmSvc),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK)},
		},
	}
}

// rejectedHeaderValue is the shape of rejection this classification exists for: a value
// the CRD's own schema refuses. The pattern is kept verbatim - the '%' in it is what
// catches the message being used as a format string.
func rejectedHeaderValue(routeName string) *apierrors.StatusError {
	return apierrors.NewInvalid(
		httpRouteGV.WithKind("HTTPRoute").GroupKind(),
		routeName,
		field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "rules").Index(0).Child("matches").Index(0).
					Child("headers").Index(0).Child("value"),
				"<oversized>",
				`must match "^[A-Za-z0-9!#$%&'*+\-.^_|~]+$"`,
			),
		},
	)
}

// routeUpdateInterceptor fails the managed route's dry-run with failWith and counts the
// writes that got past it, so a test can assert the live route was never touched.
func routeUpdateInterceptor(failWith error, writes *int) interceptor.Funcs {
	return interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, isRoute := obj.(*gwapiv1.HTTPRoute); isRoute {
				options := &client.UpdateOptions{}
				for _, opt := range opts {
					opt.ApplyToUpdate(options)
				}
				if slices.Contains(options.DryRun, metav1.DryRunAll) {
					return failWith
				}
				*writes++
			}
			return c.Update(ctx, obj, opts...)
		},
	}
}

// newRejectingRouteReconciler wires a reconciler whose managed route the API server
// refuses with failWith on the dry-run.
func newRejectingRouteReconciler(t *testing.T, llmSvc *v1alpha2.LLMInferenceService, route *gwapiv1.HTTPRoute, failWith error, writes *int) *LLMISVCReconciler {
	t.Helper()

	seedInferencePoolV1Alpha2Discovery(t)

	// Update resolves the route's scope through the RESTMapper, and the fake client's
	// default mapper knows nothing. The managed HTTPRoute is the only object on this
	// path that reaches it - the scheduler's resources take Delete, which infers scope
	// from the namespace instead.
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{httpRouteGV})
	restMapper.Add(httpRouteGV.WithKind("HTTPRoute"), meta.RESTScopeNamespace)

	return &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(invalidRouteScheme(t)).
			WithRESTMapper(restMapper).
			WithObjects(llmSvc, route).
			WithInterceptorFuncs(routeUpdateInterceptor(failWith, writes)).
			Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}
}

func TestReconcileRouter_APIServerRejectionIsTerminal(t *testing.T) {
	llmSvc := invalidRouteService()
	route := managedRoute(llmSvc)

	writes := 0
	reconciler := newRejectingRouteReconciler(t, llmSvc, route, rejectedHeaderValue(route.Name), &writes)

	err := reconciler.reconcileRouter(t.Context(), llmSvc, &Config{})
	require.ErrorIs(t, err, reconcile.TerminalError(nil),
		"a route the API server rejects is a verdict on the spec - retrying re-sends the same bytes")
	assert.Zero(t, writes, "the live route must survive a rejected dry-run untouched")

	condition := llmSvc.Status.GetCondition(v1alpha2.HTTPRoutesReady)
	require.NotNil(t, condition)
	assert.True(t, condition.IsFalse())
	assert.Equal(t, "InvalidHTTPRoute", condition.Reason)
	assert.Contains(t, condition.Message, "spec.rules[0].matches[0].headers[0].value",
		"the offending field path should be surfaced")
	assert.Contains(t, condition.Message, route.Namespace+"/"+route.Name,
		"the message should name the route the API server refused")
	assert.NotContains(t, condition.Message, "failed to get defaults",
		"the API server's field errors should not arrive behind Update's wrapper")
	assert.Equal(t, 1, strings.Count(strings.ToLower(condition.Message), "failed to reconcile httproute"),
		"the caller should not prepend a prefix the raising site already wrote")
	assert.NotContains(t, condition.Message, "%!",
		"apiserver text must be passed as an argument, not as a format string")

	for _, rollup := range []apis.ConditionType{v1alpha2.RouterReady, apis.ConditionReady} {
		c := llmSvc.Status.GetCondition(rollup)
		require.NotNil(t, c, "%s should be set", rollup)
		assert.True(t, c.IsFalse())
		assert.Equal(t, "InvalidHTTPRoute", c.Reason,
			"the actionable reason should reach %s, where the condition type is gone", rollup)
	}
}

func TestReconcileRouter_APIServerRejectionClearsStaleGatewaysReady(t *testing.T) {
	llmSvc := invalidRouteService()
	route := managedRoute(llmSvc)
	// A gateway that was not yet Programmed when this service was first reconciled.
	// DetermineRouterReadiness surfaces the first False sub-condition in
	// Gateways -> HTTPRoutes -> Pool order, so a retained False here would hide the
	// reason the operator can act on.
	llmSvc.MarkGatewaysNotReady("GatewaysNotReady", "The following Gateways are not ready: [default/kserve-ingress-gateway]")

	writes := 0
	reconciler := newRejectingRouteReconciler(t, llmSvc, route, rejectedHeaderValue(route.Name), &writes)

	require.ErrorIs(t, reconciler.reconcileRouter(t.Context(), llmSvc, &Config{}), reconcile.TerminalError(nil))

	assert.Nil(t, llmSvc.Status.GetCondition(v1alpha2.GatewaysReady),
		"validateRouterReferences clears this on every pass that reaches the managed route")

	ready := llmSvc.Status.GetCondition(apis.ConditionReady)
	require.NotNil(t, ready)
	assert.Equal(t, "InvalidHTTPRoute", ready.Reason)
}

func TestReconcileRouter_TransientRouteFailureRequeues(t *testing.T) {
	llmSvc := invalidRouteService()
	route := managedRoute(llmSvc)

	// The dry-run never reached a verdict: the webhook was unreachable. Nothing about
	// the service changed, so no watch event will fire when it recovers.
	transientErr := apierrors.NewInternalError(errors.New(
		`failed calling webhook "gateway.networking.k8s.io": connection refused`))

	writes := 0
	reconciler := newRejectingRouteReconciler(t, llmSvc, route, transientErr, &writes)

	err := reconciler.reconcileRouter(t.Context(), llmSvc, &Config{})
	require.Error(t, err, "only the server's verdict is terminal; an unreached verdict must requeue")
	assert.ErrorIs(t, err, transientErr)
	assert.NotErrorIs(t, err, reconcile.TerminalError(nil),
		"an unreached verdict must requeue, not stop the controller")

	condition := llmSvc.Status.GetCondition(v1alpha2.HTTPRoutesReady)
	require.NotNil(t, condition)
	assert.Equal(t, "HTTPRouteReconcileError", condition.Reason,
		"a failed write and a rejected route must not arrive under the same reason")
}
