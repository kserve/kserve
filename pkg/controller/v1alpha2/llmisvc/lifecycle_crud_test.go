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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// fakeClientWithRecorder wraps a fake client with event recorder for testing
type fakeClientWithRecorder struct {
	client.Client
	record.EventRecorder
}

// neverEqual forces Update past its equality short-circuit so the write is attempted.
func neverEqual(expected, curr *appsv1.Deployment) bool { return false }

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

// TestPreserveDeploymentSelector covers Update against a Deployment whose stored
// spec.selector differs from the one the caller computes. spec.selector is immutable,
// so the value already on the object is the only one the API server accepts; the
// interceptor below stands in for that validation.
func TestPreserveDeploymentSelector(t *testing.T) {
	storedSelector := map[string]string{
		"app.kubernetes.io/name":    "test-llm",
		"kueue.x-k8s.io/queue-name": "team-alpha",
	}
	computedSelector := map[string]string{
		"app.kubernetes.io/name": "test-llm",
	}

	newExpected := func(templateLabels map[string]string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "test-llm-kserve", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: computedSelector},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: templateLabels},
				},
			},
		}
	}

	tests := []struct {
		name string
		opts []llmisvc.UpdateOption[*appsv1.Deployment]
		// templateLabels defaults to storedSelector, which the stored selector accepts.
		templateLabels map[string]string
		wantErr        bool
		wantTerminal   bool
		wantSent       map[string]string
		errSubstr      string
	}{
		{
			name:     "with PreserveDeploymentSelector the stored selector is sent",
			opts:     []llmisvc.UpdateOption[*appsv1.Deployment]{llmisvc.PreserveDeploymentSelector()},
			wantSent: storedSelector,
		},
		{
			name:      "without it the computed selector is rejected",
			wantErr:   true,
			wantSent:  computedSelector,
			errSubstr: "field is immutable",
		},
		{
			name:           "a pod template that does not satisfy the stored selector fails terminally",
			opts:           []llmisvc.UpdateOption[*appsv1.Deployment]{llmisvc.PreserveDeploymentSelector()},
			templateLabels: computedSelector,
			wantErr:        true,
			wantTerminal:   true,
			errSubstr:      `kueue.x-k8s.io/queue-name="team-alpha"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			owner := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "default", UID: "test-uid"},
			}
			stored := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-llm-kserve",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						*metav1.NewControllerRef(owner, v1alpha2.LLMInferenceServiceGVK),
					},
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: storedSelector},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: storedSelector},
					},
				},
			}

			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{appsv1.SchemeGroupVersion})
			mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)

			var sent map[string]string
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRESTMapper(mapper).
				WithObjects(stored).
				WithInterceptorFuncs(interceptor.Funcs{
					Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
						d, ok := obj.(*appsv1.Deployment)
						if !ok {
							return c.Update(ctx, obj, opts...)
						}
						sent = d.Spec.Selector.MatchLabels

						curr := &appsv1.Deployment{}
						if err := c.Get(ctx, client.ObjectKeyFromObject(d), curr); err != nil {
							return err
						}
						if !equality.Semantic.DeepEqual(curr.Spec.Selector, d.Spec.Selector) {
							return errors.New(`Deployment.apps "test-llm-kserve" is invalid: spec.selector: field is immutable`)
						}
						return c.Update(ctx, obj, opts...)
					},
				}).
				Build()

			clientWithRecorder := &fakeClientWithRecorder{
				Client:        fakeClient,
				EventRecorder: record.NewFakeRecorder(10),
			}

			templateLabels := tt.templateLabels
			if templateLabels == nil {
				templateLabels = storedSelector
			}

			err := llmisvc.Reconcile(t.Context(), clientWithRecorder, owner, &appsv1.Deployment{},
				newExpected(templateLabels), llmisvc.SemanticEqual[*appsv1.Deployment](neverEqual), tt.opts...)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error containing %q, got: %v", tt.errSubstr, err)
				}
				if isTerminal := errors.Is(err, reconcile.TerminalError(nil)); isTerminal != tt.wantTerminal {
					t.Errorf("errors.Is(err, TerminalError) = %v, want %v; got: %v", isTerminal, tt.wantTerminal, err)
				}
			} else if err != nil {
				t.Fatalf("Reconcile should succeed, got: %v", err)
			}

			if !equality.Semantic.DeepEqual(sent, tt.wantSent) {
				t.Errorf("selector sent to the API server = %v, want %v", sent, tt.wantSent)
			}
		})
	}
}
