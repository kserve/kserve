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

package llmisvc_test

import (
	"context"
	"errors"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

// Group route cleanup runs ahead of desired-state reconciliation, so a failure
// on either side must not mask the other: a cleanup failure stays retryable even
// next to a TerminalError, and it must not stop workloads from reconciling.
var _ = Describe("Group route cleanup isolation", func() {
	denied := apierrors.NewForbidden(
		schema.GroupResource{Group: gwapiv1.GroupName, Resource: "httproutes"}, "route", errors.New("denied"))
	noKindMatch := &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: gwapiv1.GroupName, Kind: "HTTPRoute"}}

	// cleanupFailed reports whether err carries the cleanup wrapper, which is the
	// only part of Reconcile's error this spec makes claims about.
	cleanupFailed := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "failed to clean up terminating group backends")
	}

	DescribeTable("classifies errors independently",
		func(ctx SpecContext, routeGetErr error, invalidPreset, wantCleanupErr bool) {
			ns := NewTestNamespace(ctx, envTest)

			svc := LLMInferenceService("cleanup-isolation",
				InNamespace[*v1alpha2.LLMInferenceService](ns.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithModelName("facebook/opt-125m"),
				WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(),
				WithGroup("group"),
			)
			if invalidPreset {
				svc.Spec.BaseRefs = []corev1.LocalObjectReference{{Name: "does-not-exist"}}
			}

			cfgMap := InferenceServiceCfgMap(constants.KServeNamespace)
			presets := SharedConfigPresets(constants.KServeNamespace)
			objects := make([]client.Object, 0, 4+len(presets))
			objects = append(objects, svc, cfgMap, DefaultGateway(constants.KServeNamespace), DefaultGatewayClass())
			for _, preset := range presets {
				objects = append(objects, preset)
			}

			groupIndex, groupIndexValue := llmisvc.GroupFieldIndexForTest()
			c := fake.NewClientBuilder().
				WithScheme(envTest.Client.Scheme()).
				WithRESTMapper(envTest.Client.RESTMapper()).
				WithIndex(&v1alpha2.LLMInferenceService{}, groupIndex, groupIndexValue).
				WithStatusSubresource(&v1alpha2.LLMInferenceService{}).
				WithObjects(objects...).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*gwapiv1.HTTPRoute); ok && routeGetErr != nil {
							return routeGetErr
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).Build()

			r := &llmisvc.LLMISVCReconciler{
				Client:        c,
				Clientset:     kubernetesfake.NewClientset(cfgMap),
				EventRecorder: record.NewFakeRecorder(100),
			}

			// Twice, to prove the classification holds for repeated unchanged inputs.
			for range 2 {
				result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(svc)})
				Expect(result.IsZero()).To(BeTrue(), "cleanup must not request its own requeue delay")

				Expect(cleanupFailed(err)).To(Equal(wantCleanupErr), "got: %v", err)
				if wantCleanupErr {
					Expect(apierrors.IsForbidden(err)).To(BeTrue(), "cleanup failures must stay retryable, got: %v", err)
					Expect(errors.Is(err, reconcile.TerminalError(nil))).To(BeFalse(),
						"a terminal desired-state error must not swallow the cleanup retry")
				}

				if invalidPreset {
					continue
				}
				// A failure reading the route must not stop the rest of reconciliation.
				for _, suffix := range []string{"-kserve", "-kserve-router-scheduler"} {
					Expect(c.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: svc.Name + suffix},
						&appsv1.Deployment{})).To(Succeed())
				}
			}
		},
		Entry("healthy inputs", nil, false, false),
		// An uninstalled Gateway API is absence for cleanup; whatever the router
		// then makes of it is that path's own business.
		Entry("absent HTTPRoute CRD", noKindMatch, false, false),
		Entry("route read forbidden", denied, false, true),
		Entry("route read forbidden alongside a terminal preset error", denied, true, true),
	)
})
