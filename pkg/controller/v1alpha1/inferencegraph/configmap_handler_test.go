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

package inferencegraph

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestInferenceGraphConfigMapFunc(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	t.Run("enqueues all InferenceGraphs on ConfigMap change", func(t *testing.T) {
		graph1 := &v1alpha1.InferenceGraph{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "graph-1",
				Namespace: "default",
			},
		}
		graph2 := &v1alpha1.InferenceGraph{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "graph-2",
				Namespace: "other-ns",
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(graph1, graph2).Build()

		reconciler := &InferenceGraphReconciler{
			Client: fakeClient,
			Log:    logr.Discard(),
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inferenceservice-config",
				Namespace: "kserve",
			},
		}

		requests := reconciler.inferenceGraphConfigMapFunc(context.Background(), cm)
		if len(requests) != 2 {
			t.Fatalf("expected 2 requests, got %d", len(requests))
		}

		names := map[string]bool{}
		for _, r := range requests {
			names[r.NamespacedName.Name] = true
		}
		if !names["graph-1"] || !names["graph-2"] {
			t.Errorf("expected requests for graph-1 and graph-2, got %v", requests)
		}
	})

	t.Run("returns empty when no InferenceGraphs exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		reconciler := &InferenceGraphReconciler{
			Client: fakeClient,
			Log:    logr.Discard(),
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inferenceservice-config",
				Namespace: "kserve",
			},
		}

		requests := reconciler.inferenceGraphConfigMapFunc(context.Background(), cm)
		if len(requests) != 0 {
			t.Fatalf("expected 0 requests, got %d", len(requests))
		}
	})

	t.Run("returns nil when object is not a ConfigMap", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		reconciler := &InferenceGraphReconciler{
			Client: fakeClient,
			Log:    logr.Discard(),
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-pod",
				Namespace: "default",
			},
		}

		requests := reconciler.inferenceGraphConfigMapFunc(context.Background(), pod)
		if requests != nil {
			t.Fatalf("expected nil, got %v", requests)
		}
	})
}
