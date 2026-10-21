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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestServiceAccountImagePullSecretsChangedPredicate(t *testing.T) {
	serviceAccount := func(secrets ...string) *corev1.ServiceAccount {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns"}}
		for _, name := range secrets {
			sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
		}
		return sa
	}
	p := serviceAccountImagePullSecretsChangedPredicate()

	for _, tt := range []struct {
		name string
		got  bool
		want bool
	}{
		{
			name: "update that replaces a pull secret is accepted",
			got:  p.Update(event.UpdateEvent{ObjectOld: serviceAccount("old-pull"), ObjectNew: serviceAccount("new-pull")}),
			want: true,
		},
		{
			name: "update that adds a pull secret is accepted",
			got:  p.Update(event.UpdateEvent{ObjectOld: serviceAccount(), ObjectNew: serviceAccount("new-pull")}),
			want: true,
		},
		{
			name: "update that keeps the pull secrets is dropped",
			got: p.Update(event.UpdateEvent{
				ObjectOld: serviceAccount("pull"),
				ObjectNew: func() *corev1.ServiceAccount {
					sa := serviceAccount("pull")
					sa.Labels = map[string]string{"changed": "true"}
					return sa
				}(),
			}),
			want: false,
		},
		{
			name: "create with pull secrets is accepted",
			got:  p.Create(event.CreateEvent{Object: serviceAccount("pull")}),
			want: true,
		},
		{
			name: "create without pull secrets is dropped",
			got:  p.Create(event.CreateEvent{Object: serviceAccount()}),
			want: false,
		},
		{
			name: "delete with pull secrets is accepted",
			got:  p.Delete(event.DeleteEvent{Object: serviceAccount("pull")}),
			want: true,
		},
		{
			name: "delete without pull secrets is dropped",
			got:  p.Delete(event.DeleteEvent{Object: serviceAccount()}),
			want: false,
		},
		{
			name: "generic event is dropped",
			got:  p.Generic(event.GenericEvent{Object: serviceAccount("pull")}),
			want: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			NewGomegaWithT(t).Expect(tt.got).To(Equal(tt.want))
		})
	}
}

func TestEnqueueOnServiceAccountChange(t *testing.T) {
	g := NewGomegaWithT(t)
	r := disaggTestReconciler(t)
	for _, svc := range []*v1alpha2.LLMInferenceService{
		{ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "ns-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "ns-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "svc-c", Namespace: "ns-b"}},
	} {
		g.Expect(r.Create(t.Context(), svc)).To(Succeed())
	}

	reqs := r.enqueueOnServiceAccountChange(logr.Discard())(t.Context(),
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns-a"}})

	g.Expect(reqs).To(ConsistOf(
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns-a", Name: "svc-a"}},
		reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "ns-a", Name: "svc-b"}},
	))
}
