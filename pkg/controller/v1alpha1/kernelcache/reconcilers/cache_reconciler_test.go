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
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestKernelCacheReconcilerCreatesPrefetchJobAndAggregatesStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	cache := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "team"},
		Spec: v1alpha1.KernelCacheSpec{
			Artifact:     v1alpha1.KernelCacheArtifact{ImageReference: "registry.example/cache@sha256:" + strings.Repeat("a", 64)},
			NodeGroupRef: &corev1.LocalObjectReference{Name: "gpu"},
		},
	}
	group := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"role": "gpu"}},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{"role": "gpu", "kubernetes.io/hostname": "node-a"}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	}
	kernelCacheNode := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: node.Name}, Status: v1alpha1.KernelCacheNodeStatus{CacheStatus: map[string]v1alpha1.KernelCacheNodeCacheInfo{
		"team/cache": {State: v1alpha1.KernelCacheNodePreparationStatePending},
	}}}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, Data: map[string]string{
		"kernelcache": `{"enabled":true,"jobNamespace":"jobs","prefetchImage":"example/prefetch:latest"}`,
	}}
	jobNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "jobs"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cache).WithObjects(cache, group, node, kernelCacheNode, configMap, jobNamespace).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	if _, err := reconciler.Reconcile(t.Context(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(cache)}); err != nil {
		t.Fatal(err)
	}

	updated := &v1alpha1.KernelCache{}
	if err := k8sClient.Get(t.Context(), client.ObjectKeyFromObject(cache), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.State != v1alpha1.KernelCacheStatePreparing ||
		updated.Status.Counts == nil ||
		updated.Status.Counts.NodeCount != 1 ||
		updated.Status.Counts.NodesPreparing != 1 {
		t.Fatalf("unexpected KernelCache status: %#v", updated.Status)
	}

	serviceAccount := &corev1.ServiceAccount{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Name: kernelCachePrefetchServiceAccount, Namespace: "jobs"}, serviceAccount); err != nil {
		t.Fatalf("expected prefetch ServiceAccount: %v", err)
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace("jobs")); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected one prefetch Job, got %d", len(jobs.Items))
	}
	if jobs.Items[0].Spec.Template.Spec.ServiceAccountName != kernelCachePrefetchServiceAccount {
		t.Fatalf("unexpected Job ServiceAccountName: %q", jobs.Items[0].Spec.Template.Spec.ServiceAccountName)
	}
}

func TestAggregateKernelCacheNodeStatuses(t *testing.T) {
	kernelCache := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "team"}}
	kernelCacheNodes := &v1alpha1.KernelCacheNodeList{Items: []v1alpha1.KernelCacheNode{
		{ObjectMeta: metav1.ObjectMeta{Name: "ready"}, Status: v1alpha1.KernelCacheNodeStatus{CacheStatus: map[string]v1alpha1.KernelCacheNodeCacheInfo{
			"team/cache": {State: v1alpha1.KernelCacheNodePreparationStateReady},
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "preparing"}, Status: v1alpha1.KernelCacheNodeStatus{CacheStatus: map[string]v1alpha1.KernelCacheNodeCacheInfo{
			"team/cache": {State: v1alpha1.KernelCacheNodePreparationStatePulling},
		}}},
	}}
	aggregate := aggregateKernelCacheNodeStatuses(kernelCache, []string{"ready", "preparing"}, kernelCacheNodes)
	if aggregate.State != v1alpha1.KernelCacheStatePreparing || aggregate.NodesReady != 1 || aggregate.NodesPreparing != 1 {
		t.Fatalf("unexpected aggregate: %#v", aggregate)
	}
}

func TestKernelCacheNodeGroupNameUsesDefault(t *testing.T) {
	cache := &v1alpha1.KernelCache{}
	if got := kernelCacheNodeGroupName(cache, "default-group"); got != "default-group" {
		t.Fatalf("expected default group, got %q", got)
	}
	cache.Spec.NodeGroupRef = &corev1.LocalObjectReference{Name: "explicit-group"}
	if got := kernelCacheNodeGroupName(cache, "default-group"); got != "explicit-group" {
		t.Fatalf("expected explicit group, got %q", got)
	}
}
