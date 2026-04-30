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

package localmodelnode

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

func TestLocalModelNodeConfigMapFunc(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	t.Run("enqueues all LocalModelNodes on ConfigMap change", func(t *testing.T) {
		node1 := &v1alpha1.LocalModelNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-1",
				Namespace: "default",
			},
		}
		node2 := &v1alpha1.LocalModelNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "node-2",
				Namespace: "other-ns",
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node1, node2).Build()

		reconciler := &LocalModelNodeReconciler{
			Client: fakeClient,
			Log:    logr.Discard(),
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inferenceservice-config",
				Namespace: "kserve",
			},
		}

		requests := reconciler.localModelNodeConfigMapFunc(context.Background(), cm)
		if len(requests) != 2 {
			t.Fatalf("expected 2 requests, got %d", len(requests))
		}

		names := map[string]bool{}
		for _, r := range requests {
			names[r.Name] = true
		}
		if !names["node-1"] || !names["node-2"] {
			t.Errorf("expected requests for node-1 and node-2, got %v", requests)
		}
	})

	t.Run("returns empty when no LocalModelNodes exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		reconciler := &LocalModelNodeReconciler{
			Client: fakeClient,
			Log:    logr.Discard(),
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inferenceservice-config",
				Namespace: "kserve",
			},
		}

		requests := reconciler.localModelNodeConfigMapFunc(context.Background(), cm)
		if len(requests) != 0 {
			t.Fatalf("expected 0 requests, got %d", len(requests))
		}
	})
}
