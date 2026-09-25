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

package kernelcachenode

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestDiscoverCachesForNode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "gpu-node-1",
			Labels: map[string]string{"kubernetes.io/hostname": "gpu-node-1"},
		},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type:   corev1.NodeReady,
			Status: corev1.ConditionTrue,
		}}},
	}
	nodeGroup := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-workers"},
		Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"kubernetes.io/hostname": "gpu-node-1"},
		},
	}
	kernelCache := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-cache", Namespace: "staging"},
		Spec: v1alpha1.KernelCacheSpec{
			Artifact: v1alpha1.KernelCacheArtifact{
				Identity: v1alpha1.KernelCacheIdentity{
					Footprints: v1alpha1.KernelCacheFootprints{
						WorkloadFootprint:      "sha256:1111111111111111111111111111111111111111111111111111111111111111",
						CompatibilityFootprint: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
					},
				},
			},
		},
		Status: v1alpha1.KernelCacheStatus{
			Usage: &v1alpha1.KernelCacheUsage{Pods: []v1alpha1.KernelCachePodUsage{{
				PodUID:   "pod-1",
				NodeName: node.Name,
			}}},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node, nodeGroup, kernelCache).
		Build()
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient, NodeName: node.Name}
	kernelCacheNode := &v1alpha1.KernelCacheNode{}

	podsUsing, changed, err := reconciler.discoverCaches(context.Background(), kernelCacheNode, nodeGroup.Name)
	if err != nil {
		t.Fatal(err)
	}
	if podsUsing != 1 {
		t.Fatalf("expected one Pod using the cache, got %d", podsUsing)
	}
	if !changed {
		t.Fatal("expected cache discovery to report a new CacheStatus entry")
	}

	cacheInfo, ok := kernelCacheNode.Status.CacheStatus["staging/qwen-cache"]
	if !ok {
		t.Fatal("expected matching KernelCache in node status")
	}
	if cacheInfo.KernelCacheRef.Name != kernelCache.Name || cacheInfo.KernelCacheRef.Namespace != kernelCache.Namespace {
		t.Fatalf("unexpected cache reference: %#v", cacheInfo.KernelCacheRef)
	}
	if cacheInfo.State != v1alpha1.KernelCacheNodePreparationStatePending {
		t.Fatalf("expected pending state, got %q", cacheInfo.State)
	}
	if cacheInfo.Footprints != kernelCache.Spec.Artifact.Identity.Footprints {
		t.Fatal("expected cache footprints to be copied")
	}
}

func TestCacheWithoutNodeGroupUsesDefault(t *testing.T) {
	cache := &v1alpha1.KernelCache{}
	matchingGroups := map[string]struct{}{"gpu-workers": {}}
	if !matchesNodeGroup(cache, matchingGroups, "gpu-workers") {
		t.Fatal("expected cache without nodeGroupRef to use the default node group")
	}
	if matchesNodeGroup(cache, matchingGroups, "cpu-workers") {
		t.Fatal("expected cache without nodeGroupRef not to match a different default node group")
	}
	if matchesNodeGroup(cache, matchingGroups, "") {
		t.Fatal("expected cache without nodeGroupRef and default not to match")
	}
}

func TestDiscoverCachesRemovesAndRestoresEntryWithNodeReadiness(t *testing.T) {
	const nodeName = "gpu-node-1"

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{v1alpha1.AddToScheme, corev1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: map[string]string{"role": "gpu"}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}},
	}
	nodeGroup := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-workers"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	kernelCache := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "team"},
		Spec:       v1alpha1.KernelCacheSpec{NodeGroupRef: &corev1.LocalObjectReference{Name: nodeGroup.Name}},
	}
	kernelCacheNode := &v1alpha1.KernelCacheNode{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Status: v1alpha1.KernelCacheNodeStatus{CacheStatus: map[string]v1alpha1.KernelCacheNodeCacheInfo{
			"team/cache": {State: v1alpha1.KernelCacheNodePreparationStateReady},
		}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(node).WithObjects(node, nodeGroup, kernelCache).Build()
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient, NodeName: nodeName}

	if _, _, err := reconciler.discoverCaches(t.Context(), kernelCacheNode, ""); err != nil {
		t.Fatal(err)
	}
	if len(kernelCacheNode.Status.CacheStatus) != 0 {
		t.Fatalf("expected cache status to be removed for a NotReady node, got %#v", kernelCacheNode.Status.CacheStatus)
	}

	node.Status.Conditions[0].Status = corev1.ConditionTrue
	if err := k8sClient.Status().Update(t.Context(), node); err != nil {
		t.Fatal(err)
	}
	storedNode := &corev1.Node{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Name: nodeName}, storedNode); err != nil {
		t.Fatal(err)
	}
	if storedNode.Status.Conditions[0].Status != corev1.ConditionTrue {
		t.Fatalf("expected stored node to be Ready, got %q", storedNode.Status.Conditions[0].Status)
	}
	_, changed, err := reconciler.discoverCaches(t.Context(), kernelCacheNode, "")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected cache discovery to report a restored CacheStatus entry")
	}
	if _, ok := kernelCacheNode.Status.CacheStatus["team/cache"]; !ok {
		t.Fatal("expected cache status to be restored for a Ready node")
	}
}
