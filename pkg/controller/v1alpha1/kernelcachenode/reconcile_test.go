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
	"errors"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// Retry KCN status updates after a resource version conflict.
func TestReconcileRetriesStatusConflict(t *testing.T) {
	const (
		nodeName       = "gpu-node-1"
		cacheNamespace = "team"
		cacheName      = "cache"
		imageReference = "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		v1alpha1.AddToScheme,
		corev1.AddToScheme,
		batchv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{"kubernetes.io/hostname": nodeName},
		},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type:   corev1.NodeReady,
			Status: corev1.ConditionTrue,
		}}},
	}
	nodeGroup := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-workers"},
		Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"kubernetes.io/hostname": nodeName},
		},
	}
	kernelCache := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{Name: cacheName, Namespace: cacheNamespace},
		Spec: v1alpha1.KernelCacheSpec{
			NodeGroupRef: &corev1.LocalObjectReference{Name: nodeGroup.Name},
			Artifact:     v1alpha1.KernelCacheArtifact{ImageReference: imageReference},
		},
	}
	kernelCacheNode := &v1alpha1.KernelCacheNode{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Status: v1alpha1.KernelCacheNodeStatus{CacheStatus: map[string]v1alpha1.KernelCacheNodeCacheInfo{
			cacheNamespace + "/" + cacheName: {
				KernelCacheRef: v1alpha1.NamespacedName{Namespace: cacheNamespace, Name: cacheName},
				ImageReference: imageReference,
				State:          v1alpha1.KernelCacheNodePreparationStatePending,
			},
		}},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prefetch-job",
			Namespace: "jobs",
			Labels: map[string]string{
				kernelCacheNameLabel:      cacheName,
				kernelCacheNamespaceLabel: cacheNamespace,
				kernelCacheNodeLabel:      nodeName,
			},
		},
		Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{{Name: "kernel-cache", VolumeSource: corev1.VolumeSource{
				Image: &corev1.ImageVolumeSource{Reference: imageReference},
			}}},
		}}},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
			Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
		}}},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true,"jobNamespace":"jobs"}`},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(kernelCacheNode).
		WithObjects(node, nodeGroup, kernelCache, kernelCacheNode, job, configMap).
		Build()
	conflictClient := &conflictStatusClient{Client: baseClient, conflicts: 1}
	reconciler := &KernelCacheNodeReconciler{Client: conflictClient, NodeName: nodeName}

	if _, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: types.NamespacedName{Name: nodeName}}); err != nil {
		t.Fatalf("expected status conflict to be retried: %v", err)
	}
	if conflictClient.statusUpdates != 2 {
		t.Fatalf("expected one conflict and one successful status update, got %d updates", conflictClient.statusUpdates)
	}

	updated := &v1alpha1.KernelCacheNode{}
	if err := baseClient.Get(t.Context(), client.ObjectKey{Name: nodeName}, updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Status.CacheStatus[cacheNamespace+"/"+cacheName].State; got != v1alpha1.KernelCacheNodePreparationStateReady {
		t.Fatalf("expected cache state Ready, got %q", got)
	}
}

func TestReconcileKeepsReadyCacheAfterJobDeletion(t *testing.T) {
	const (
		nodeName       = "gpu-node-1"
		cacheNamespace = "team"
		cacheName      = "cache"
		imageReference = "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		v1alpha1.AddToScheme,
		corev1.AddToScheme,
		batchv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{"kubernetes.io/hostname": nodeName},
		},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type: corev1.NodeReady, Status: corev1.ConditionTrue,
		}}},
	}
	nodeGroup := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-workers"},
		Spec: v1alpha1.KernelCacheNodeGroupSpec{
			NodeSelector: map[string]string{"kubernetes.io/hostname": nodeName},
		},
	}
	kernelCache := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{Name: cacheName, Namespace: cacheNamespace},
		Spec: v1alpha1.KernelCacheSpec{
			NodeGroupRef: &corev1.LocalObjectReference{Name: nodeGroup.Name},
			Artifact:     v1alpha1.KernelCacheArtifact{ImageReference: imageReference},
		},
	}
	kernelCacheNode := &v1alpha1.KernelCacheNode{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Status: v1alpha1.KernelCacheNodeStatus{CacheStatus: map[string]v1alpha1.KernelCacheNodeCacheInfo{
			cacheNamespace + "/" + cacheName: {
				KernelCacheRef: v1alpha1.NamespacedName{Namespace: cacheNamespace, Name: cacheName},
				ImageReference: imageReference,
				State:          v1alpha1.KernelCacheNodePreparationStateReady,
			},
		}},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true,"jobNamespace":"jobs"}`},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(kernelCacheNode).
		WithObjects(node, nodeGroup, kernelCache, kernelCacheNode, configMap).
		Build()
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient, NodeName: nodeName}

	if _, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: types.NamespacedName{Name: nodeName}}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	updated := &v1alpha1.KernelCacheNode{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Name: nodeName}, updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Status.CacheStatus[cacheNamespace+"/"+cacheName].State; got != v1alpha1.KernelCacheNodePreparationStateReady {
		t.Fatalf("expected cache state Ready after Job deletion, got %q", got)
	}

	if err := reconciler.validateNodeImages(t.Context()); err != nil {
		t.Fatalf("image validation failed: %v", err)
	}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Name: nodeName}, updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Status.CacheStatus[cacheNamespace+"/"+cacheName].State; got != v1alpha1.KernelCacheNodePreparationStatePending {
		t.Fatalf("expected periodic image validation to mark cache Pending, got %q", got)
	}
}

func TestReconcileValidatesNewCacheStatusFromNodeImage(t *testing.T) {
	const (
		nodeName       = "gpu-node-1"
		cacheNamespace = "team"
		cacheName      = "cache"
		imageReference = "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{v1alpha1.AddToScheme, corev1.AddToScheme, batchv1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: map[string]string{"kubernetes.io/hostname": nodeName}},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Images:     []corev1.ContainerImage{{Names: []string{imageReference}}},
		},
	}
	nodeGroup := &v1alpha1.KernelCacheNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu-workers"},
		Spec:       v1alpha1.KernelCacheNodeGroupSpec{NodeSelector: map[string]string{"kubernetes.io/hostname": nodeName}},
	}
	kernelCache := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{Name: cacheName, Namespace: cacheNamespace},
		Spec: v1alpha1.KernelCacheSpec{
			NodeGroupRef: &corev1.LocalObjectReference{Name: nodeGroup.Name},
			Artifact:     v1alpha1.KernelCacheArtifact{ImageReference: imageReference},
		},
	}
	kernelCacheNode := &v1alpha1.KernelCacheNode{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true,"jobNamespace":"jobs"}`},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(kernelCacheNode).
		WithObjects(node, nodeGroup, kernelCache, kernelCacheNode, configMap).
		Build()
	reconciler := &KernelCacheNodeReconciler{Client: k8sClient, NodeName: nodeName}

	if _, err := reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: types.NamespacedName{Name: nodeName}}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	updated := &v1alpha1.KernelCacheNode{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Name: nodeName}, updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Status.CacheStatus[cacheNamespace+"/"+cacheName].State; got != v1alpha1.KernelCacheNodePreparationStateReady {
		t.Fatalf("expected cache state Ready, got %q", got)
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(t.Context(), jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("expected no prefetch Job for an image already on the node, got %d", len(jobs.Items))
	}
}

type conflictStatusClient struct {
	client.Client
	conflicts     int
	statusUpdates int
}

func (c *conflictStatusClient) Status() client.SubResourceWriter {
	return &conflictStatusWriter{SubResourceWriter: c.Client.Status(), client: c}
}

type conflictStatusWriter struct {
	client.SubResourceWriter
	client *conflictStatusClient
}

func (w *conflictStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	w.client.statusUpdates++
	if w.client.conflicts > 0 {
		w.client.conflicts--
		return apierrors.NewConflict(
			schema.GroupResource{Group: "serving.kserve.io", Resource: "kernelcachenodes"},
			obj.GetName(),
			errors.New("injected conflict"),
		)
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}
