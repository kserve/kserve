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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestKernelCacheRequestsForMatchingNodeGroupsIncludesDefaultGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-a", Labels: map[string]string{"role": "gpu"}}}
	group := &v1alpha1.KernelCacheNodeGroup{ObjectMeta: metav1.ObjectMeta{Name: "gpu"}, Spec: v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}}}
	defaultCache := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "team"}}
	explicitCache := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "explicit", Namespace: "team"}, Spec: v1alpha1.KernelCacheSpec{NodeGroupRef: &corev1.LocalObjectReference{Name: "gpu"}}}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, Data: map[string]string{
		"kernelcache": `{"enabled":true,"defaultNodeGroup":"gpu"}`,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, group, defaultCache, explicitCache, configMap).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	requests := reconciler.kcRequestsForMatchingNodeGroups(t.Context(), node)
	if len(requests) != 2 {
		t.Fatalf("expected both default and explicit KCs, got %#v", requests)
	}
}

func TestEnqueueKCsOnKCNChangeDeduplicatesReferences(t *testing.T) {
	reconciler := &KernelCacheReconciler{}
	node := &v1alpha1.KernelCacheNode{Status: v1alpha1.KernelCacheNodeStatus{CacheStatus: map[string]v1alpha1.KernelCacheNodeCacheInfo{
		"first":   {KernelCacheRef: v1alpha1.NamespacedName{Namespace: "team", Name: "cache"}},
		"second":  {KernelCacheRef: v1alpha1.NamespacedName{Namespace: "team", Name: "cache"}},
		"invalid": {KernelCacheRef: v1alpha1.NamespacedName{}},
	}}}
	requests := reconciler.enqueueKCsOnKCNChange(t.Context(), node)
	if len(requests) != 1 || requests[0].NamespacedName != (client.ObjectKey{Namespace: "team", Name: "cache"}) {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestInferenceServiceConfigMapPredicate(t *testing.T) {
	matching := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}}
	other := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: constants.KServeNamespace}}
	if !isInferenceServiceConfigMap(matching) || isInferenceServiceConfigMap(other) {
		t.Fatal("unexpected ConfigMap predicate result")
	}
}
