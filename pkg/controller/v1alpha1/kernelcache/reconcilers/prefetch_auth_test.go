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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kernelcachelabels "github.com/kserve/kserve/pkg/kernelcache/labels"
)

func TestReconcilePrefetchServiceAccountAccessCreatesCrossNamespaceBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		rbacv1.AddToScheme,
		batchv1.AddToScheme,
		v1alpha1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}
	kernelCache := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "source"}}
	config := &v1beta1.KernelCacheConfig{
		JobNamespace: "jobs",
		Registry: v1beta1.KernelCacheRegistryConfig{Auth: v1beta1.KernelCacheRegistryAuth{
			Type:        v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken,
			PullRoleRef: &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-puller"},
		}},
	}

	ready, err := reconciler.reconcilePrefetchServiceAccountAccess(t.Context(), kernelCache, config)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("expected prefetch access to be ready")
	}

	serviceAccount := &corev1.ServiceAccount{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Namespace: "jobs", Name: kernelCachePrefetchServiceAccount}, serviceAccount); err != nil {
		t.Fatal(err)
	}
	binding := &rbacv1.RoleBinding{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Namespace: "source", Name: kernelCachePrefetchRoleBinding}, binding); err != nil {
		t.Fatal(err)
	}
	if binding.RoleRef.Name != "registry-puller" || len(binding.Subjects) != 1 || binding.Subjects[0].Namespace != "jobs" {
		t.Fatalf("unexpected prefetch RoleBinding: %#v", binding)
	}
}

func TestReconcilePrefetchServiceAccountAccessRemovesBindingWhenAuthIsDisabled(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		rbacv1.AddToScheme,
		batchv1.AddToScheme,
		v1alpha1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	binding := buildPrefetchImagePullRoleBinding("source", "jobs", &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-puller"})
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name: kernelCachePrefetchServiceAccount, Namespace: "jobs",
			Labels: map[string]string{kernelCachePrefetchManagedLabel: "true"},
		}, AutomountServiceAccountToken: boolPointer(false)},
		binding,
	).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}
	kernelCache := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "source"}}

	ready, err := reconciler.reconcilePrefetchServiceAccountAccess(t.Context(), kernelCache, &v1beta1.KernelCacheConfig{JobNamespace: "jobs"})
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("expected disabled registry access cleanup to complete")
	}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Namespace: "source", Name: kernelCachePrefetchRoleBinding}, &rbacv1.RoleBinding{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected managed pull RoleBinding to be removed, got %v", err)
	}
}

func TestReconcilePrefetchServiceAccountAccessKeepsBindingWhileJobIsActive(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		rbacv1.AddToScheme,
		batchv1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	binding := buildPrefetchImagePullRoleBinding("source", "jobs", &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-puller"})
	jobMetadata := kernelcachelabels.ObjectMeta("cache", "source", "node")
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: "prefetch-job", Namespace: "jobs", Labels: jobMetadata.Labels,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name: kernelCachePrefetchServiceAccount, Namespace: "jobs",
			Labels: map[string]string{kernelCachePrefetchManagedLabel: "true"},
		}, AutomountServiceAccountToken: boolPointer(false)},
		binding,
		job,
	).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}
	kernelCache := &v1alpha1.KernelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "source"}}

	ready, err := reconciler.reconcilePrefetchServiceAccountAccess(t.Context(), kernelCache, &v1beta1.KernelCacheConfig{JobNamespace: "jobs"})
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("expected cleanup to wait for the active prefetch Job")
	}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Namespace: "source", Name: kernelCachePrefetchRoleBinding}, &rbacv1.RoleBinding{}); err != nil {
		t.Fatalf("expected binding to remain while the Job is active: %v", err)
	}
}

func boolPointer(value bool) *bool {
	return &value
}
