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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// The shared PVC evictor objects have other writers, so losing a write race on one
// is routine: the service requeues on the error but has nothing to warn about.
var _ = Describe("PVC evictor write conflicts", func() {
	const (
		svcName               = "evictor-conflict"
		evictorServiceAccount = "kserve-llm-isvc-kv-pvc-evictor"
	)
	lostRace := apierrors.NewConflict(schema.GroupResource{Resource: "serviceaccounts"}, evictorServiceAccount, errors.New("modified by a peer"))

	isEvictorServiceAccount := func(obj client.Object) bool {
		_, ok := obj.(*corev1.ServiceAccount)
		return ok && obj.GetName() == evictorServiceAccount
	}
	isWorkloadDeployment := func(obj client.Object) bool {
		_, ok := obj.(*appsv1.Deployment)
		return ok && obj.GetName() == svcName+"-kserve"
	}

	// reconcileOnce reconciles a service with a pvc.ref tier against a fake client and
	// returns the Reconciliation failed events. The evictor ServiceAccount is seeded
	// without the evictor label, so the evictor pass has to update it.
	reconcileOnce := func(ctx SpecContext, funcs interceptor.Funcs, opts ...LLMInferenceServiceOption) ([]string, error) {
		ns := NewTestNamespace(ctx, envTest)
		pvcName := "shared-kv-cache"

		svc := LLMInferenceService(svcName, append([]LLMInferenceServiceOption{
			InNamespace[*v1alpha2.LLMInferenceService](ns.Name),
			WithModelURI("pvc://facebook-models/opt-125m"),
			WithModelName("foo"),
		}, opts...)...)
		svc.Spec.KVCacheOffloading = &v1alpha2.KVCacheOffloadingSpec{
			CPU: resource.MustParse("10Gi"),
			Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: pvcName, Path: "kv/"}},
			}}},
		}
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: ns.Name},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			},
		}
		serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: evictorServiceAccount, Namespace: ns.Name}}

		cfgMap := InferenceServiceCfgMap(constants.KServeNamespace)
		presets := SharedConfigPresets(constants.KServeNamespace)
		objects := make([]client.Object, 0, 6+len(presets))
		objects = append(objects, svc, pvc, serviceAccount, cfgMap, DefaultGateway(constants.KServeNamespace), DefaultGatewayClass())
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
			WithInterceptorFuncs(funcs).
			Build()

		recorder := record.NewFakeRecorder(100)
		r := &llmisvc.LLMISVCReconciler{
			Client:        c,
			APIReader:     c,
			Clientset:     kubernetesfake.NewClientset(cfgMap),
			EventRecorder: recorder,
		}

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(svc)})

		var failures []string
		for len(recorder.Events) > 0 {
			if event := <-recorder.Events; strings.Contains(event, "Reconciliation failed") {
				failures = append(failures, event)
			}
		}
		return failures, err
	}

	DescribeTable("Reconciliation failed event",
		func(ctx SpecContext, failOn func(client.Object) bool, writeErr error, wantFailures int) {
			failures, err := reconcileOnce(ctx, interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if failOn(obj) {
						return writeErr
					}
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if failOn(obj) {
						return writeErr
					}
					return c.Update(ctx, obj, opts...)
				},
			})

			Expect(err).To(MatchError(writeErr), "the failed write must reach Reconcile's caller to requeue")
			Expect(failures).To(HaveLen(wantFailures))
		},
		Entry("is skipped for a lost race on the shared evictor ServiceAccount",
			isEvictorServiceAccount, lostRace, 0),
		Entry("is recorded for any other failure writing it",
			isEvictorServiceAccount, apierrors.NewInternalError(errors.New("etcd unavailable")), 1),
		Entry("is recorded for a conflict outside the evictor pass",
			isWorkloadDeployment, apierrors.NewConflict(schema.GroupResource{}, svcName+"-kserve", errors.New("modified")), 1),
	)

	It("keeps the event of a group cleanup failure next to a lost race", func(ctx SpecContext) {
		denied := apierrors.NewForbidden(schema.GroupResource{Group: gwapiv1.GroupName, Resource: "httproutes"},
			svcName+"-kserve-route", errors.New("denied"))
		routeReads, lostRaces := 0, 0

		failures, err := reconcileOnce(ctx, interceptor.Funcs{
			// Group cleanup reads the route before reconcile does. Failing only that read
			// lets reconcile get past the router and end on the evictor conflict.
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*gwapiv1.HTTPRoute); ok {
					if routeReads++; routeReads == 1 {
						return denied
					}
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if isEvictorServiceAccount(obj) {
					lostRaces++
					return lostRace
				}
				return c.Update(ctx, obj, opts...)
			},
		}, WithManagedRoute(), WithManagedGateway(), WithManagedScheduler(), WithGroup("group"))

		Expect(err).To(MatchError(denied))
		Expect(lostRaces).To(Equal(1))
		Expect(failures).To(ConsistOf("Warning Error Reconciliation failed: " + denied.Error()))
	})

	// Reconcile can skip the evictor conflict only because reconcile never returns it
	// together with a later step's failure.
	It("keeps the event of a router failure after a lost race", func(ctx SpecContext) {
		routeDown := apierrors.NewInternalError(errors.New("route write failed"))
		lostRaces := 0

		failures, err := reconcileOnce(ctx, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*gwapiv1.HTTPRoute); ok {
					return routeDown
				}
				return c.Create(ctx, obj, opts...)
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if isEvictorServiceAccount(obj) {
					lostRaces++
					return lostRace
				}
				return c.Update(ctx, obj, opts...)
			},
		}, WithManagedRoute(), WithManagedGateway(), WithManagedScheduler())

		Expect(err).To(MatchError(routeDown))
		Expect(lostRaces).To(Equal(1))
		Expect(failures).To(ConsistOf(ContainSubstring(routeDown.Error())))
	})
})
