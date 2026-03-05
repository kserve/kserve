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

package reconcilers

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

func TestLocalModelConfigMapFunc(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	t.Run("enqueues all LocalModelCaches on ConfigMap change", func(t *testing.T) {
		model1 := &v1alpha1.LocalModelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name: "model-1",
			},
		}
		model2 := &v1alpha1.LocalModelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name: "model-2",
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(model1, model2).Build()

		reconciler := &LocalModelReconciler{
			Client: fakeClient,
			Log:    logr.Discard(),
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inferenceservice-config",
				Namespace: "kserve",
			},
		}

		requests := reconciler.localModelConfigMapFunc(context.Background(), cm)
		if len(requests) != 2 {
			t.Fatalf("expected 2 requests, got %d", len(requests))
		}

		names := map[string]bool{}
		for _, r := range requests {
			names[r.NamespacedName.Name] = true
		}
		if !names["model-1"] || !names["model-2"] {
			t.Errorf("expected requests for model-1 and model-2, got %v", requests)
		}
	})

	t.Run("returns empty when no LocalModelCaches exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		reconciler := &LocalModelReconciler{
			Client: fakeClient,
			Log:    logr.Discard(),
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "inferenceservice-config",
				Namespace: "kserve",
			},
		}

		requests := reconciler.localModelConfigMapFunc(context.Background(), cm)
		if len(requests) != 0 {
			t.Fatalf("expected 0 requests, got %d", len(requests))
		}
	})
}
