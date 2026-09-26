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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/kernelcache/registryauth"
)

func TestEnsurePusherIdentityForCaptureCreatesOwnedResources(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	capture := &v1alpha1.KernelCacheCapture{ObjectMeta: metav1.ObjectMeta{
		Name: "model-kcc-revision", Namespace: "team", UID: "capture-uid",
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(capture).Build()
	reconciler := &KernelCacheCaptureControllerReconciler{Client: k8sClient, Reader: k8sClient}
	config := v1beta1.KernelCacheRegistryConfig{Auth: v1beta1.KernelCacheRegistryAuth{
		Type:        v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken,
		PushRoleRef: &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-pusher"},
	}}

	if err := reconciler.ensurePusherIdentityForCapture(t.Context(), capture, config); err != nil {
		t.Fatal(err)
	}

	serviceAccount := &corev1.ServiceAccount{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Namespace: capture.Namespace, Name: registryauth.PusherServiceAccountName(capture.Name)}, serviceAccount); err != nil {
		t.Fatal(err)
	}
	if owner := metav1.GetControllerOf(serviceAccount); owner == nil || owner.UID != capture.UID {
		t.Fatalf("unexpected ServiceAccount owner: %#v", serviceAccount.OwnerReferences)
	}

	binding := &rbacv1.RoleBinding{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Namespace: capture.Namespace, Name: registryauth.PusherRoleBindingName(capture.Name)}, binding); err != nil {
		t.Fatal(err)
	}
	if binding.RoleRef.Name != "registry-pusher" || binding.RoleRef.Kind != "ClusterRole" {
		t.Fatalf("unexpected pusher RoleRef: %#v", binding.RoleRef)
	}
	if owner := metav1.GetControllerOf(binding); owner == nil || owner.UID != capture.UID {
		t.Fatalf("unexpected RoleBinding owner: %#v", binding.OwnerReferences)
	}
}

func TestEnsurePusherIdentityForCapturePreservesActiveRoleRef(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		rbacv1.AddToScheme,
		v1alpha1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "model-kcc-revision", Namespace: "team", UID: "capture-uid"},
		Status:     v1alpha1.KernelCacheCaptureStatus{Phase: v1alpha1.KernelCacheCapturePhaseCapturing},
	}
	owner := captureOwnerReference(capture)
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            registryauth.PusherRoleBindingName(capture.Name),
			Namespace:       capture.Namespace,
			Labels:          map[string]string{registryauth.ManagedLabel: "true"},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "registry-pusher-v1"},
		Subjects: []rbacv1.Subject{{
			Kind: "ServiceAccount", Name: registryauth.PusherServiceAccountName(capture.Name), Namespace: capture.Namespace,
		}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(capture, binding).Build()
	reconciler := &KernelCacheCaptureControllerReconciler{Client: k8sClient, Reader: k8sClient}
	config := v1beta1.KernelCacheRegistryConfig{Auth: v1beta1.KernelCacheRegistryAuth{
		Type:        v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken,
		PushRoleRef: &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-pusher-v2"},
	}}

	if err := reconciler.ensurePusherIdentityForCapture(t.Context(), capture, config); err != nil {
		t.Fatal(err)
	}

	updated := &rbacv1.RoleBinding{}
	if err := k8sClient.Get(t.Context(), client.ObjectKeyFromObject(binding), updated); err != nil {
		t.Fatal(err)
	}
	if updated.RoleRef.Name != "registry-pusher-v1" {
		t.Fatalf("active RoleBinding was changed: %#v", updated.RoleRef)
	}
}

func TestEnsurePusherIdentityForCaptureRejectsInactiveRoleRefDrift(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		rbacv1.AddToScheme,
		v1alpha1.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	capture := &v1alpha1.KernelCacheCapture{
		ObjectMeta: metav1.ObjectMeta{Name: "model-kcc-revision", Namespace: "team", UID: "capture-uid"},
		Status:     v1alpha1.KernelCacheCaptureStatus{Phase: v1alpha1.KernelCacheCapturePhaseComplete},
	}
	owner := captureOwnerReference(capture)
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            registryauth.PusherRoleBindingName(capture.Name),
			Namespace:       capture.Namespace,
			Labels:          map[string]string{registryauth.ManagedLabel: "true"},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "registry-pusher-v1"},
		Subjects: []rbacv1.Subject{{
			Kind: "ServiceAccount", Name: registryauth.PusherServiceAccountName(capture.Name), Namespace: capture.Namespace,
		}},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(capture, binding).Build()
	reconciler := &KernelCacheCaptureControllerReconciler{Client: k8sClient, Reader: k8sClient}
	config := v1beta1.KernelCacheRegistryConfig{Auth: v1beta1.KernelCacheRegistryAuth{
		Type:        v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken,
		PushRoleRef: &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-pusher-v2"},
	}}

	if err := reconciler.ensurePusherIdentityForCapture(t.Context(), capture, config); err == nil {
		t.Fatal("expected inactive RoleRef drift to fail")
	}
}
