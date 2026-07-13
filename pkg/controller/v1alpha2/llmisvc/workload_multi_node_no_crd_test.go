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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/credentials"
)

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
