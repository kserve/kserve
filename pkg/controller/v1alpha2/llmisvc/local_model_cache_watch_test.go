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
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// A workload waits on a cache through the base model label or through a LoRA
// adapter entry of the localmodel-lora annotation; both bindings must be
// enqueued when the cache's readiness changes, so fail-closed workloads whose
// base model is uncached but an adapter is cached also resume immediately.
func TestEnqueueOnLocalModelCacheChange(t *testing.T) {
	requestFor := func(namespace, name string) reconcile.Request {
		return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}
	}

	for _, tt := range []struct {
		name   string
		cache  client.Object
		llmSvc func() *v1alpha2.LLMInferenceService
		want   []reconcile.Request
	}{
		{
			name:  "namespace cache enqueues the base-model-bound service",
			cache: &v1alpha1.LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns-a"}},
			llmSvc: func() *v1alpha2.LLMInferenceService {
				return &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
					Name:      "base-bound",
					Namespace: "ns-a",
					Labels: map[string]string{
						constants.LocalModelLabel:          "cache",
						constants.LocalModelNamespaceLabel: "ns-a",
					},
				}}
			},
			want: []reconcile.Request{requestFor("ns-a", "base-bound")},
		},
		{
			name:  "namespace cache enqueues a service bound only through a LoRA adapter",
			cache: &v1alpha1.LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns-a"}},
			llmSvc: func() *v1alpha2.LLMInferenceService {
				return &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
					Name:      "lora-bound",
					Namespace: "ns-a",
					Annotations: map[string]string{
						constants.LocalModelLoRAAnnotationKey: `{"adapter":{"cache":"cache","namespace":"ns-a"}}`,
					},
				}}
			},
			want: []reconcile.Request{requestFor("ns-a", "lora-bound")},
		},
		{
			name:  "namespace cache ignores a service in another namespace",
			cache: &v1alpha1.LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns-a"}},
			llmSvc: func() *v1alpha2.LLMInferenceService {
				return &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
					Name:      "other-namespace",
					Namespace: "ns-b",
					Annotations: map[string]string{
						constants.LocalModelLoRAAnnotationKey: `{"adapter":{"cache":"cache","namespace":"ns-a"}}`,
					},
				}}
			},
			want: nil,
		},
		{
			name:  "namespace cache ignores an unbound service",
			cache: &v1alpha1.LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns-a"}},
			llmSvc: func() *v1alpha2.LLMInferenceService {
				return &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
					Name:      "unbound",
					Namespace: "ns-a",
				}}
			},
			want: nil,
		},
		{
			name:  "cluster cache enqueues the base-model-bound service",
			cache: &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}},
			llmSvc: func() *v1alpha2.LLMInferenceService {
				return &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-bound",
					Namespace: "ns-a",
					Labels: map[string]string{
						constants.LocalModelLabel: "cache",
					},
				}}
			},
			want: []reconcile.Request{requestFor("ns-a", "cluster-bound")},
		},
		{
			name:  "cluster cache enqueues a service bound only through a LoRA adapter",
			cache: &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}},
			llmSvc: func() *v1alpha2.LLMInferenceService {
				return &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
					Name:      "lora-bound",
					Namespace: "ns-a",
					Annotations: map[string]string{
						constants.LocalModelLoRAAnnotationKey: `{"adapter":{"cache":"cache"}}`,
					},
				}}
			},
			want: []reconcile.Request{requestFor("ns-a", "lora-bound")},
		},
		{
			name:  "cluster cache ignores a service bound to a same-named namespace cache",
			cache: &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}},
			llmSvc: func() *v1alpha2.LLMInferenceService {
				return &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
					Name:      "namespace-bound",
					Namespace: "ns-a",
					Labels: map[string]string{
						constants.LocalModelLabel:          "cache",
						constants.LocalModelNamespaceLabel: "ns-a",
					},
				}}
			},
			want: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			scheme := runtime.NewScheme()
			g.Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
			r := &LLMISVCReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.llmSvc()).Build(),
			}

			reqs := r.enqueueOnLocalModelCacheChange(logr.Discard())(t.Context(), tt.cache)

			g.Expect(reqs).To(ConsistOf(tt.want))
		})
	}
}
