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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

// These specs exercise Reconcile against the envtest API server rather than the
// controller, so the selector a Deployment is created with can be chosen directly.
// spec.selector validation - immutability, and the pod template having to satisfy the
// selector - is enforced by the API server, so a fake client cannot stand in here.
var _ = Describe("Deployment selector", func() {
	const (
		nameLabel  = "app.kubernetes.io/name"
		queueLabel = "kueue.x-k8s.io/queue-name"
	)

	// storedSelector stands for a Deployment created by a reconciler that put the
	// propagated queue label in the selector. The current one computes nameLabel only.
	storedSelector := map[string]string{nameLabel: "selector-fixture", queueLabel: "team-alpha"}

	// alwaysDiffer forces Update past the equality short-circuit so the write is attempted.
	alwaysDiffer := func(expected, curr *appsv1.Deployment) bool { return false }

	deployment := func(namespace string, owner *v1alpha2.LLMInferenceService, selector, podLabels map[string]string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "selector-fixture",
				Namespace: namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(owner, v1alpha2.LLMInferenceServiceGVK),
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)),
				Selector: &metav1.LabelSelector{MatchLabels: selector},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "main", Image: "busybox"}},
					},
				},
			},
		}
	}

	// owner is never created: envtest runs no garbage collector, so an ownerReference
	// is enough to satisfy the controlled-by check in Update.
	owner := func(namespace string) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "selector-fixture",
				Namespace: namespace,
				UID:       types.UID("11111111-2222-3333-4444-555555555555"),
			},
		}
	}

	reconcile := func(ctx SpecContext, o *v1alpha2.LLMInferenceService, expected *appsv1.Deployment) error {
		c := &fakeClientWithRecorder{Client: envTest.Client, EventRecorder: record.NewFakeRecorder(10)}
		return llmisvc.Reconcile(ctx, c, o, &appsv1.Deployment{}, expected,
			llmisvc.SemanticEqual[*appsv1.Deployment](alwaysDiffer),
			llmisvc.PreserveDeploymentSelector())
	}

	It("should reconcile a Deployment whose stored selector the reconciler no longer computes", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		o := owner(testNs.Name)

		stored := deployment(testNs.Name, o, storedSelector, storedSelector)
		Expect(envTest.Create(ctx, stored)).To(Succeed())

		// The pod template still carries the queue label, so it satisfies the stored
		// selector even though the selector is no longer what would be computed.
		expected := deployment(testNs.Name, o,
			map[string]string{nameLabel: "selector-fixture"},
			map[string]string{nameLabel: "selector-fixture", queueLabel: "team-alpha"})

		Expect(reconcile(ctx, o, expected)).To(Succeed())

		curr := &appsv1.Deployment{}
		Expect(envTest.Get(ctx, client.ObjectKeyFromObject(stored), curr)).To(Succeed())
		Expect(curr.Spec.Selector.MatchLabels).To(Equal(storedSelector),
			"the stored selector must be carried over unchanged")
	})

	It("should reject the update once the pod template stops satisfying the stored selector", func(ctx SpecContext) {
		testNs := NewTestNamespace(ctx, envTest)
		o := owner(testNs.Name)

		stored := deployment(testNs.Name, o, storedSelector, storedSelector)
		Expect(envTest.Create(ctx, stored)).To(Succeed())

		// Dropping the queue label from the LLMInferenceService removes it from the pod
		// template, leaving the preserved selector requiring a label the pods no longer
		// carry. Such a Deployment has to be recreated to reconcile again.
		expected := deployment(testNs.Name, o,
			map[string]string{nameLabel: "selector-fixture"},
			map[string]string{nameLabel: "selector-fixture"})

		err := reconcile(ctx, o, expected)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.template.metadata.labels"))
	})
})
