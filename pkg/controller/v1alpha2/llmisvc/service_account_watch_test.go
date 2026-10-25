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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
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
			name: "update that removes all pull secrets is accepted",
			got:  p.Update(event.UpdateEvent{ObjectOld: serviceAccount("old-pull"), ObjectNew: serviceAccount()}),
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
	svcA := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "ns-a", UID: "uid-a"}}
	svcB := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "ns-a", UID: "uid-b"}}
	svcC := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "svc-c", Namespace: "ns-b", UID: "uid-c"}}
	requestFor := func(svc *v1alpha2.LLMInferenceService) reconcile.Request {
		return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}}
	}
	deploymentFor := func(svc *v1alpha2.LLMInferenceService, labels map[string]string, serviceAccountName string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:            svc.Name + "-" + labels[constants.KubernetesComponentLabelKey],
				Namespace:       svc.Namespace,
				Labels:          labels,
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(svc, v1alpha2.LLMInferenceServiceGVK)},
			},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{ServiceAccountName: serviceAccountName}}},
		}
	}
	tokenizerFor := func(svc *v1alpha2.LLMInferenceService, serviceAccountName string) *appsv1.Deployment {
		return deploymentFor(svc, TokenizerLabels(svc), serviceAccountName)
	}
	schedulerFor := func(svc *v1alpha2.LLMInferenceService, serviceAccountName string) *appsv1.Deployment {
		return deploymentFor(svc, SchedulerLabels(svc), serviceAccountName)
	}
	sharedServiceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:            "svc-a-epp-sa",
		Namespace:       "ns-a",
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(svcA, v1alpha2.LLMInferenceServiceGVK)},
	}}

	for _, tt := range []struct {
		name           string
		serviceAccount *corev1.ServiceAccount
		deployments    []*appsv1.Deployment
		want           []reconcile.Request
	}{
		{
			name:           "default ServiceAccount enqueues every service in its namespace",
			serviceAccount: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns-a"}},
			want:           []reconcile.Request{requestFor(svcA), requestFor(svcB)},
		},
		{
			name:           "user ServiceAccount enqueues every service in its namespace",
			serviceAccount: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "my-sa", Namespace: "ns-a"}},
			want:           []reconcile.Request{requestFor(svcA), requestFor(svcB)},
		},
		{
			name:           "ServiceAccount owned by a service enqueues only its owner",
			serviceAccount: sharedServiceAccount,
			deployments: []*appsv1.Deployment{
				tokenizerFor(svcA, "svc-a-epp-sa"),
				tokenizerFor(svcB, "default"),
				schedulerFor(svcB, "svc-a-epp-sa"),
			},
			want: []reconcile.Request{requestFor(svcA)},
		},
		{
			name:           "ServiceAccount owned by a service also enqueues services whose tokenizer runs as it",
			serviceAccount: sharedServiceAccount,
			deployments: []*appsv1.Deployment{
				tokenizerFor(svcB, "svc-a-epp-sa"),
				tokenizerFor(svcC, "svc-a-epp-sa"),
			},
			want: []reconcile.Request{requestFor(svcA), requestFor(svcB)},
		},
		{
			name: "ServiceAccount controlled by another kind enqueues every service in its namespace",
			serviceAccount: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
				Name:      "other-sa",
				Namespace: "ns-a",
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(
					&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "svc-a", UID: "uid-cm"}},
					corev1.SchemeGroupVersion.WithKind("ConfigMap"))},
			}},
			want: []reconcile.Request{requestFor(svcA), requestFor(svcB)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			r := disaggTestReconciler(t)
			for _, svc := range []*v1alpha2.LLMInferenceService{svcA, svcB, svcC} {
				g.Expect(r.Create(t.Context(), svc.DeepCopy())).To(Succeed())
			}
			for _, deployment := range tt.deployments {
				g.Expect(r.Create(t.Context(), deployment)).To(Succeed())
			}

			reqs := r.enqueueOnServiceAccountChange(logr.Discard())(t.Context(), tt.serviceAccount)

			g.Expect(reqs).To(ConsistOf(tt.want))
		})
	}
}
