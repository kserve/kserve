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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/credentials"
)

// restMapperFor returns a RESTMapper that only knows the given namespaced kinds,
// so tests can control which APIs discovery reports as installed.
func restMapperFor(gvks ...schema.GroupVersionKind) *meta.DefaultRESTMapper {
	mapper := meta.NewDefaultRESTMapper(nil)
	for _, gvk := range gvks {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	return mapper
}

// errLWSNoMatch simulates the error returned when the LeaderWorkerSet CRD is not installed.
var errLWSNoMatch = &meta.NoKindMatchError{
	GroupKind: schema.GroupKind{
		Group: "leaderworkerset.x-k8s.io",
		Kind:  "LeaderWorkerSet",
	},
}

func newFakeClientWithLWSNoMatch(t *testing.T) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}
	if err := lwsapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add lwsapi to scheme: %v", err)
	}
	// The prefill ServiceAccount preservation path reads the DisaggregatedSet
	// regardless of the deployed backend, so the fake client scheme needs the
	// type registered to serve those lookups (a missing CRD on a real cluster
	// surfaces as a NoKindMatchError, which currentDisaggregatedSet tolerates).
	if err := disaggregatedsetv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add disaggregatedsetv1 to scheme: %v", err)
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*lwsapi.LeaderWorkerSet); ok {
					return errLWSNoMatch
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
}

// newFakeClientWithLWSNotCached simulates the strict cache rejecting LWS reads
// because no informer was registered: without ReaderFailOnMissingInformer this
// is what a cluster without the LWS CRD returns once watch registration is
// CRD-gated. The RESTMapper reports whether the API is actually installed.
func newFakeClientWithLWSNotCached(t *testing.T, lwsInstalled bool) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 to scheme: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}
	if err := lwsapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add lwsapi to scheme: %v", err)
	}

	gvk := lwsapi.GroupVersion.WithKind("LeaderWorkerSet")
	mapper := restMapperFor(appsv1.SchemeGroupVersion.WithKind("Deployment"))
	if lwsInstalled {
		mapper.Add(gvk, meta.RESTScopeNamespace)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(mapper).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*lwsapi.LeaderWorkerSet); ok {
					return &cache.ErrResourceNotCached{GVK: gvk}
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
}

func newLLMInferenceServiceWithMultiNode(name, namespace string) *v1alpha2.LLMInferenceService {
	modelURL, _ := apis.ParseURL("pvc://my-model/model-dir")
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI: *modelURL,
			},
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Parallelism: &v1alpha2.ParallelismSpec{
					Data:      ptr.To[int32](2),
					DataLocal: ptr.To[int32](1),
				},
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "quay.io/test/vllm:latest"},
					},
				},
				Worker: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "quay.io/test/vllm:latest"},
					},
				},
			},
		},
	}
}

func TestExpectedMainMultiNodeLWS_WhenCRDNotInstalled_ShouldNotFail(t *testing.T) {
	// This test verifies that expectedMainMultiNodeLWS handles the case when the
	// LeaderWorkerSet CRD is not installed on the cluster (meta.NoKindMatchError).
	// Previously, the Get call would fail because only apierrors.IsNotFound was checked,
	// but meta.NoKindMatchError is a distinct error type that must also be handled.

	// given
	fakeClient := newFakeClientWithLWSNoMatch(t)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	llmSvc := newLLMInferenceServiceWithMultiNode("test-llm", "default")
	config := &Config{
		CredentialConfig: &credentials.CredentialConfig{},
	}

	// when
	lws, err := reconciler.expectedMainMultiNodeLWS(t.Context(), llmSvc, config)
	// then - should succeed without error
	if err != nil {
		t.Errorf("expectedMainMultiNodeLWS should not fail when LWS CRD is not installed, got: %v", err)
	}
	if lws == nil {
		t.Fatal("expectedMainMultiNodeLWS should return a non-nil LWS even when CRD is not installed")
	}
}

func TestExpectedPrefillMultiNodeLWS_WhenCRDNotInstalled_ShouldNotFail(t *testing.T) {
	// This test verifies that expectedPrefillMultiNodeLWS handles the case when the
	// LeaderWorkerSet CRD is not installed on the cluster (meta.NoKindMatchError).

	// given
	fakeClient := newFakeClientWithLWSNoMatch(t)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	modelURL, _ := apis.ParseURL("pvc://my-model/model-dir")
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-prefill",
			Namespace: "default",
			UID:       "test-uid-prefill",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI: *modelURL,
			},
			Prefill: &v1alpha2.WorkloadSpec{
				Parallelism: &v1alpha2.ParallelismSpec{
					Data:      ptr.To[int32](2),
					DataLocal: ptr.To[int32](1),
				},
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "quay.io/test/vllm:latest"},
					},
				},
				Worker: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "quay.io/test/vllm:latest"},
					},
				},
			},
		},
	}
	config := &Config{
		CredentialConfig: &credentials.CredentialConfig{},
	}

	// when
	lws, err := reconciler.expectedPrefillMultiNodeLWS(t.Context(), llmSvc, config)
	// then - should succeed without error
	if err != nil {
		t.Errorf("expectedPrefillMultiNodeLWS should not fail when LWS CRD is not installed, got: %v", err)
	}
	if lws == nil {
		t.Fatal("expectedPrefillMultiNodeLWS should return a non-nil LWS even when CRD is not installed")
	}
}

// TestExpectedMainMultiNodeLWS_WhenLWSInformerMissing covers the strict cache
// (ReaderFailOnMissingInformer) rejecting LWS reads on clusters without the LWS
// CRD, where watch registration never started an informer. This path runs for
// every service - reconcileMultiNodeMainRole reads the deployed LWS even for
// single-node services with an existing service account - so treating the
// rejection as "nothing deployed" keeps those services starting.
func TestExpectedMainMultiNodeLWS_WhenLWSInformerMissing(t *testing.T) {
	for _, tc := range []struct {
		name         string
		lwsInstalled bool
		wantErr      bool
	}{
		{name: "uninstalled API means nothing is deployed", lwsInstalled: false},
		{name: "installed API keeps the missing-informer error", lwsInstalled: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := newFakeClientWithLWSNotCached(t, tc.lwsInstalled)
			reconciler := &LLMISVCReconciler{
				Client:        fakeClient,
				EventRecorder: record.NewFakeRecorder(10),
			}
			llmSvc := newLLMInferenceServiceWithMultiNode("test-llm", "default")
			config := &Config{
				CredentialConfig: &credentials.CredentialConfig{},
			}

			lws, err := reconciler.expectedMainMultiNodeLWS(t.Context(), llmSvc, config)
			if tc.wantErr {
				var notCached *cache.ErrResourceNotCached
				if !errors.As(err, &notCached) {
					t.Fatalf("expected the missing-informer error to surface for an installed API, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expectedMainMultiNodeLWS should not fail when the LWS API is not installed, got: %v", err)
			}
			if lws == nil {
				t.Fatal("expectedMainMultiNodeLWS should return a non-nil LWS even when the LWS API is not installed")
			}
		})
	}
}

// TestCurrentMainWorkloadRole_WhenLWSInformerMissing covers the same strict-cache
// rejection for the role label read of the deployed main workload.
func TestCurrentMainWorkloadRole_WhenLWSInformerMissing(t *testing.T) {
	for _, tc := range []struct {
		name         string
		lwsInstalled bool
		wantErr      bool
	}{
		{name: "uninstalled API means no current role", lwsInstalled: false},
		{name: "installed API keeps the missing-informer error", lwsInstalled: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := newFakeClientWithLWSNotCached(t, tc.lwsInstalled)
			reconciler := &LLMISVCReconciler{
				Client:        fakeClient,
				EventRecorder: record.NewFakeRecorder(10),
			}
			llmSvc := newLLMInferenceServiceWithMultiNode("test-llm", "default")

			role, err := reconciler.currentMainWorkloadRole(t.Context(), llmSvc)
			if tc.wantErr {
				var notCached *cache.ErrResourceNotCached
				if !errors.As(err, &notCached) {
					t.Fatalf("expected the missing-informer error to surface for an installed API, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("currentMainWorkloadRole should not fail when the LWS API is not installed, got: %v", err)
			}
			if role != "" {
				t.Errorf("currentMainWorkloadRole should report no role when the LWS API is not installed, got: %q", role)
			}
		})
	}
}

// TestHasNonDisaggregatedWorkloads_WhenLWSInformerMissing covers the existence
// check that decides whether to emit a migration event: an uninstalled LWS API
// must count as "no LWS workloads" instead of logging read failures.
func TestHasNonDisaggregatedWorkloads_WhenLWSInformerMissing(t *testing.T) {
	fakeClient := newFakeClientWithLWSNotCached(t, false)
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}
	llmSvc := newLLMInferenceServiceWithMultiNode("test-llm", "default")

	if reconciler.hasNonDisaggregatedWorkloads(t.Context(), llmSvc) {
		t.Error("an uninstalled LWS API means no LWS workloads exist")
	}
}

// newFakeClientWithDisaggregatedSetNotCached simulates the strict cache rejecting
// DisaggregatedSet reads on clusters without the DisaggregatedSet CRD. The RESTMapper
// reports whether the API is actually installed; the LWS API is always absent so the
// prefill ServiceAccount fallback finds no deployed workload either.
func newFakeClientWithDisaggregatedSetNotCached(t *testing.T, dsInstalled bool) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 to scheme: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}
	if err := lwsapi.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add lwsapi to scheme: %v", err)
	}
	if err := disaggregatedsetv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add disaggregatedsetv1 to scheme: %v", err)
	}

	dsGVK := disaggregatedsetv1.GroupVersion.WithKind("DisaggregatedSet")
	mapper := restMapperFor(appsv1.SchemeGroupVersion.WithKind("Deployment"))
	if dsInstalled {
		mapper.Add(dsGVK, meta.RESTScopeNamespace)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(mapper).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj.(type) {
				case *disaggregatedsetv1.DisaggregatedSet:
					return &cache.ErrResourceNotCached{GVK: dsGVK}
				case *lwsapi.LeaderWorkerSet:
					lwsGVK := lwsapi.GroupVersion.WithKind("LeaderWorkerSet")
					return &cache.ErrResourceNotCached{GVK: lwsGVK}
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
}

// TestExpectedMultiNodePrefillServiceAccount_WhenDisaggregatedSetInformerMissing
// mirrors the e2e regression where a prefill workload on a cluster without the
// DisaggregatedSet CRD failed to build its ServiceAccount: the prefill
// ServiceAccount inheritance reads the deployed workload from the DisaggregatedSet
// or LeaderWorkerSet backends, and both strict-cache rejections must count as
// "nothing deployed" when discovery confirms the APIs are uninstalled.
func TestExpectedMultiNodePrefillServiceAccount_WhenDisaggregatedSetInformerMissing(t *testing.T) {
	for _, tc := range []struct {
		name        string
		dsInstalled bool
		wantErr     bool
	}{
		{name: "uninstalled API means nothing is deployed", dsInstalled: false},
		{name: "installed API keeps the missing-informer error", dsInstalled: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := newFakeClientWithDisaggregatedSetNotCached(t, tc.dsInstalled)
			reconciler := &LLMISVCReconciler{
				Client:        fakeClient,
				EventRecorder: record.NewFakeRecorder(10),
			}
			llmSvc := newLLMInferenceServiceWithMultiNode("test-llm", "default")
			prefill := llmSvc.Spec.DeepCopy()
			llmSvc.Spec.Prefill = &prefill.WorkloadSpec

			sa, _, err := reconciler.expectedMultiNodePrefillServiceAccount(t.Context(), llmSvc)
			if tc.wantErr {
				var notCached *cache.ErrResourceNotCached
				if !errors.As(err, &notCached) {
					t.Fatalf("expected the missing-informer error to surface for an installed API, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expectedMultiNodePrefillServiceAccount should not fail when the DisaggregatedSet API is not installed, got: %v", err)
			}
			if sa == nil {
				t.Fatal("expectedMultiNodePrefillServiceAccount should return a generated ServiceAccount when no workload is deployed")
			}
		})
	}
}
