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

package llmisvc_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// fakeClientWithRecorder wraps a fake client with event recorder for testing
type fakeClientWithRecorder struct {
	client.Client
	record.EventRecorder
}

func TestDelete_WhenCRDNotInstalled_ShouldNotFail(t *testing.T) {
	// given - a client that returns NoMatchError (CRD not installed)
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

	// Simulate NoKindMatchError that happens when CRD is not installed on the cluster
	noMatchErr := &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{
			Group: "leaderworkerset.x-k8s.io",
			Kind:  "LeaderWorkerSet",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				// Return NoMatchError for LeaderWorkerSet to simulate missing CRD
				if _, ok := obj.(*lwsapi.LeaderWorkerSet); ok {
					return noMatchErr
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	clientWithRecorder := &fakeClientWithRecorder{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	owner := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "default",
			UID:       "test-uid",
		},
	}

	// LeaderWorkerSet to delete - CRD not installed
	lws := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm-kserve-mn",
			Namespace: "default",
		},
	}

	// when
	err := llmisvc.Delete(t.Context(), clientWithRecorder, owner, lws)
	// then - should succeed without error (nothing to delete when CRD doesn't exist)
	if err != nil {
		t.Errorf("Delete should not fail when CRD doesn't exist, got: %v", err)
	}
}

func TestDelete_WhenResourceNotFound_ShouldNotFail(t *testing.T) {
	// given - a client with LWS CRD but no resources
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

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	clientWithRecorder := &fakeClientWithRecorder{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	owner := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "default",
			UID:       "test-uid",
		},
	}

	lws := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-existent-lws",
			Namespace: "default",
		},
	}

	// when
	err := llmisvc.Delete(t.Context(), clientWithRecorder, owner, lws)
	// then - should succeed (nothing to delete)
	if err != nil {
		t.Errorf("Delete should not fail when resource doesn't exist, got: %v", err)
	}
}

func TestNoMatchError_ShouldBeDistinguishedFromNotFound(t *testing.T) {
	// This test documents that NoMatchError and NotFound are different error types
	// and we need to handle both in Delete.

	noMatchErr := &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{
			Group: "leaderworkerset.x-k8s.io",
			Kind:  "LeaderWorkerSet",
		},
	}

	if !meta.IsNoMatchError(noMatchErr) {
		t.Error("NoKindMatchError should be identified by meta.IsNoMatchError")
	}
	if apierrors.IsNotFound(noMatchErr) {
		t.Error("NoKindMatchError should NOT be identified as NotFound")
	}

	notFoundErr := apierrors.NewNotFound(
		schema.GroupResource{Group: "leaderworkerset.x-k8s.io", Resource: "leaderworkersets"},
		"test-lws",
	)

	if !apierrors.IsNotFound(notFoundErr) {
		t.Error("NotFound error should be identified by apierrors.IsNotFound")
	}
	if meta.IsNoMatchError(notFoundErr) {
		t.Error("NotFound error should NOT be identified as NoMatchError")
	}
}
