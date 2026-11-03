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

package reconcilers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestBuildKernelCacheUsageKeepsOnlyActivePods(t *testing.T) {
	active := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "active", Namespace: "team", UID: types.UID("active")}, Spec: corev1.PodSpec{NodeName: "node-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	active.Annotations = map[string]string{constants.KernelCacheUsageAnnotationKey: "team/cache"}
	finished := active.DeepCopy()
	finished.Name = "finished"
	finished.UID = types.UID("finished")
	finished.Status.Phase = corev1.PodSucceeded
	other := active.DeepCopy()
	other.Name = "other"
	other.UID = types.UID("other")
	other.Annotations[constants.KernelCacheUsageAnnotationKey] = "team/other"

	usage := buildKernelCacheUsage([]corev1.Pod{active, *finished, *other}, "team/cache", nil)
	if usage == nil || usage.TotalPodsUsing != 1 || len(usage.Pods) != 1 || usage.Pods[0].PodRef.Name != "active" {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestEnqueueKCOnPodUsageChangeRejectsCrossNamespaceReference(t *testing.T) {
	reconciler := &KernelCacheReconciler{}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "team", Annotations: map[string]string{
		constants.KernelCacheUsageAnnotationKey: "other/cache",
	}}}
	if requests := reconciler.enqueueKCOnPodUsageChange(t.Context(), pod); len(requests) != 0 {
		t.Fatalf("expected no request for cross-namespace usage reference, got %#v", requests)
	}
}

func TestEnqueueKCOnPodUsageChangeRefreshesNamespaceWhenAnnotationIsRemoved(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	first := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "team"}}
	second := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: "team"}}
	reconciler := &KernelCacheReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(first, second).Build()}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "team"}}
	requests := reconciler.enqueueKCOnPodUsageChange(t.Context(), pod)
	if len(requests) != 2 {
		t.Fatalf("expected all KCs in the namespace after annotation removal, got %#v", requests)
	}
}
