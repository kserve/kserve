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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// TestReconcilePVCEvictorsRollsUpWorkloadReadiness covers the status a single pass
// leaves behind. reconcilePVCEvictors runs after reconcileWorkload has already rolled
// up the stored sub-conditions, so WorkloadsReady and Ready must follow the new
// PVCEvictorReady within the same status write.
func TestReconcilePVCEvictorsRollsUpWorkloadReadiness(t *testing.T) {
	const namespace = "test"
	errInjectedDelete := errors.New("injected Deployment delete failure")

	service := func(withEvictor bool, stored duckv1.Conditions) *v1alpha2.LLMInferenceService {
		svc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: namespace},
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{Containers: []corev1.Container{
					{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				}},
			}},
			Status: v1alpha2.LLMInferenceServiceStatus{Status: duckv1.Status{Conditions: stored}},
		}
		if withEvictor {
			svc.Spec.Template.Containers = append(svc.Spec.Template.Containers, corev1.Container{Name: pvcEvictorTemplateName})
			svc.Spec.KVCacheOffloading = &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
					},
				}}},
			}
		}
		return svc
	}
	// evictor is the shared Deployment for the evictor tier, whether or not the
	// service under test still references it.
	evictor := func(availableReplicas int32) *appsv1.Deployment {
		owner := service(true, nil)
		deployment := expectedPVCEvictorDeployment(owner, collectPVCEvictorTargets(owner)[0])
		deployment.Status.AvailableReplicas = availableReplicas
		return deployment
	}
	cond := func(conditionType apis.ConditionType, status corev1.ConditionStatus, reason string) *apis.Condition {
		return &apis.Condition{Type: conditionType, Status: status, Reason: reason}
	}
	// conditions builds a status with the router and presets ready, so Ready follows
	// WorkloadsReady alone. A nil argument leaves that condition out.
	conditions := func(main, evictor, workloads, ready *apis.Condition) duckv1.Conditions {
		result := duckv1.Conditions{}
		for _, c := range []*apis.Condition{
			cond(v1alpha2.PresetsCombined, corev1.ConditionTrue, ""),
			cond(v1alpha2.RouterReady, corev1.ConditionTrue, ""),
			main, evictor, workloads, ready,
		} {
			if c != nil {
				result = append(result, *c)
			}
		}
		return result
	}

	mainReady := cond(v1alpha2.MainWorkloadReady, corev1.ConditionTrue, "")
	mainNotReady := cond(v1alpha2.MainWorkloadReady, corev1.ConditionFalse, "MinimumReplicasUnavailable")
	mainStopped := cond(v1alpha2.MainWorkloadReady, corev1.ConditionFalse, "Stopped")
	evictorReady := cond(v1alpha2.PVCEvictorReady, corev1.ConditionTrue, "")
	evictorNotReady := cond(v1alpha2.PVCEvictorReady, corev1.ConditionFalse, "PVCEvictorNotReady")
	evictorError := cond(v1alpha2.PVCEvictorReady, corev1.ConditionFalse, "ReconcilePVCEvictorError")
	workloadsReady := cond(v1alpha2.WorkloadReady, corev1.ConditionTrue, "")
	ready := cond(apis.ConditionReady, corev1.ConditionTrue, "")

	notReadyBecause := func(reason string) (*apis.Condition, *apis.Condition) {
		return cond(v1alpha2.WorkloadReady, corev1.ConditionFalse, reason), cond(apis.ConditionReady, corev1.ConditionFalse, reason)
	}
	workloadsBlockedByEvictor, readyBlockedByEvictor := notReadyBecause("PVCEvictorNotReady")
	workloadsBlockedByError, readyBlockedByError := notReadyBecause("ReconcilePVCEvictorError")
	workloadsBlockedByMain, readyBlockedByMain := notReadyBecause("MinimumReplicasUnavailable")
	workloadsBlockedByStop, readyBlockedByStop := notReadyBecause("Stopped")

	tests := []struct {
		name        string
		withEvictor bool
		stopped     bool
		stored      duckv1.Conditions
		// evictor already in the namespace; nil means none exists yet.
		evictor           *appsv1.Deployment
		noPVCReader       bool
		failEvictorDelete bool
		wantErr           bool
		want              duckv1.Conditions
	}{
		{
			name:        "evictor gets its first available replica",
			withEvictor: true,
			stored:      conditions(mainReady, evictorNotReady, workloadsBlockedByEvictor, readyBlockedByEvictor),
			evictor:     evictor(1),
			want:        conditions(mainReady, evictorReady, workloadsReady, ready),
		},
		{
			name:        "evictor loses its replicas while Ready",
			withEvictor: true,
			stored:      conditions(mainReady, evictorReady, workloadsReady, ready),
			evictor:     evictor(0),
			want:        conditions(mainReady, evictorNotReady, workloadsBlockedByEvictor, readyBlockedByEvictor),
		},
		{
			name:        "first pass after upgrading a Ready service creates the evictor",
			withEvictor: true,
			stored:      conditions(mainReady, nil, workloadsReady, ready),
			want:        conditions(mainReady, evictorNotReady, workloadsBlockedByEvictor, readyBlockedByEvictor),
		},
		{
			name:   "evictor no longer needed",
			stored: conditions(mainReady, evictorNotReady, workloadsBlockedByEvictor, readyBlockedByEvictor),
			want:   conditions(mainReady, nil, workloadsReady, ready),
		},
		{
			name:        "main workload keeps blocking readiness",
			withEvictor: true,
			stored:      conditions(mainNotReady, evictorNotReady, workloadsBlockedByMain, readyBlockedByMain),
			evictor:     evictor(1),
			want:        conditions(mainNotReady, evictorReady, workloadsBlockedByMain, readyBlockedByMain),
		},
		{
			name:        "reconcile error while Ready",
			withEvictor: true,
			stored:      conditions(mainReady, evictorReady, workloadsReady, ready),
			noPVCReader: true,
			wantErr:     true,
			want:        conditions(mainReady, evictorError, workloadsBlockedByError, readyBlockedByError),
		},
		{
			name:              "orphan cleanup fails on a service without evictors",
			stored:            conditions(mainReady, nil, workloadsReady, ready),
			evictor:           evictor(1),
			failEvictorDelete: true,
			wantErr:           true,
			want:              conditions(mainReady, nil, workloadsReady, ready),
		},
		{
			name:              "orphan cleanup fails right after the service stopped using the evictor",
			stored:            conditions(mainReady, evictorReady, workloadsReady, ready),
			evictor:           evictor(1),
			failEvictorDelete: true,
			wantErr:           true,
			want:              conditions(mainReady, evictorReady, workloadsReady, ready),
		},
		{
			name:              "orphan cleanup fails on a stopped service",
			withEvictor:       true,
			stopped:           true,
			stored:            conditions(mainStopped, nil, workloadsBlockedByStop, readyBlockedByStop),
			evictor:           evictor(1),
			failEvictorDelete: true,
			wantErr:           true,
			want:              conditions(mainStopped, nil, workloadsBlockedByStop, readyBlockedByStop),
		},
	}

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := service(tt.withEvictor, tt.stored)
			if tt.stopped {
				svc.Annotations = map[string]string{constants.StopAnnotationKey: "true"}
			}
			objects := []client.Object{svc}
			if tt.evictor != nil {
				objects = append(objects, tt.evictor)
			}
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...)
			if tt.failEvictorDelete {
				builder = builder.WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if _, ok := obj.(*appsv1.Deployment); ok {
							return errInjectedDelete
						}
						return c.Delete(ctx, obj, opts...)
					},
				})
			}
			reconciler := &LLMISVCReconciler{Client: builder.Build()}
			if !tt.noPVCReader {
				reconciler.APIReader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: namespace},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					},
				}).Build()
			}

			err := reconciler.reconcilePVCEvictors(t.Context(), svc, &Config{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("reconcilePVCEvictors() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.failEvictorDelete && !errors.Is(err, errInjectedDelete) {
				t.Fatalf("reconcilePVCEvictors() error = %v, want the injected orphan Delete failure", err)
			}

			if diff := cmp.Diff(tt.want, svc.Status.Conditions,
				cmpopts.IgnoreFields(apis.Condition{}, "LastTransitionTime", "Severity", "Message"),
				cmpopts.SortSlices(func(a, b apis.Condition) bool { return a.Type < b.Type }),
			); diff != "" {
				t.Errorf("conditions after one pass (-want +got):\n%s", diff)
			}
		})
	}
}
