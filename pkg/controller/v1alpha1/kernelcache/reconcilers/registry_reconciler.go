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
	"context"
	"errors"
	"fmt"
	"reflect"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/kernelcache/registryauth"
)

// ensurePusherIdentityForCapture creates the capture-scoped identity used for
// registry publication and applies the configured RoleRef to its RoleBinding.
func (r *KernelCacheCaptureControllerReconciler) ensurePusherIdentityForCapture(
	ctx context.Context,
	capture *v1alpha1.KernelCacheCapture,
	config v1beta1.KernelCacheRegistryConfig,
) error {
	namespace := capture.Namespace
	serviceAccountName := registryauth.PusherServiceAccountName(capture.Name)
	owner := captureOwnerReference(capture)
	if config.Auth.PushRoleRef == nil {
		return errors.New("registry push RoleRef is required for serviceAccountToken authentication")
	}
	if err := r.ensureManagedServiceAccountWithOwner(ctx, namespace, serviceAccountName, registryauth.ManagedLabel, owner); err != nil {
		return err
	}
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            registryauth.PusherRoleBindingName(capture.Name),
			Namespace:       namespace,
			Labels:          map[string]string{registryauth.ManagedLabel: "true"},
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		RoleRef:  kernelCacheRegistryRoleRef(config.Auth.PushRoleRef),
		Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: serviceAccountName, Namespace: namespace}},
	}
	return r.reconcilePusherRoleBindingForActiveCapture(ctx, capture, desired)
}

// reconcilePusherRoleBindingForActiveCapture keeps an active capture's
// authorization stable when the registry ConfigMap changes. A new capture
// receives the current configured RoleRef when its binding is created.
func (r *KernelCacheCaptureControllerReconciler) reconcilePusherRoleBindingForActiveCapture(
	ctx context.Context,
	capture *v1alpha1.KernelCacheCapture,
	desired *rbacv1.RoleBinding,
) error {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	current := &rbacv1.RoleBinding{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, desired); err == nil {
			return nil
		} else if !apierrors.IsAlreadyExists(err) {
			return err
		}
		if err := reader.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
			return err
		}
	}
	if current.Labels[registryauth.ManagedLabel] != "true" {
		return fmt.Errorf("reserved RoleBinding %s/%s is not managed by kernelcache", desired.Namespace, desired.Name)
	}
	desiredOwner := metav1.GetControllerOf(desired)
	currentOwner := metav1.GetControllerOf(current)
	if desiredOwner == nil || currentOwner == nil || currentOwner.UID != desiredOwner.UID {
		return fmt.Errorf("reserved RoleBinding %s/%s has a different owner", desired.Namespace, desired.Name)
	}
	if current.RoleRef != desired.RoleRef {
		if captureInProgress(capture.Status.Phase) {
			return nil
		}
		return fmt.Errorf("managed pusher RoleBinding %s/%s has an unexpected RoleRef", desired.Namespace, desired.Name)
	}
	if reflect.DeepEqual(current.Subjects, desired.Subjects) {
		return nil
	}
	base := current.DeepCopy()
	current.Subjects = desired.Subjects
	return r.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

// ensureControllerRegistryBindings grants the localmodel controller registry
// access in a workload namespace. Artifact signing and verification run in the
// controller Pod, not in the capture pusher or prefetcher Pods.
func (r *KernelCacheCaptureControllerReconciler) ensureControllerRegistryBindings(ctx context.Context, namespace string, config v1beta1.KernelCacheRegistryConfig) error {
	if config.Auth.PushRoleRef == nil || config.Auth.PullRoleRef == nil {
		return errors.New("registry push and pull RoleRefs are required for controller access")
	}
	subject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      r.OperatorServiceAccount,
		Namespace: r.OperatorNamespace,
	}
	bindings := []*rbacv1.RoleBinding{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryauth.ControllerPushRoleBindingName,
				Namespace: namespace,
				Labels:    map[string]string{registryauth.ManagedLabel: "true"},
			},
			RoleRef:  kernelCacheRegistryRoleRef(config.Auth.PushRoleRef),
			Subjects: []rbacv1.Subject{subject},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryauth.ControllerPullRoleBindingName,
				Namespace: namespace,
				Labels:    map[string]string{registryauth.ManagedLabel: "true"},
			},
			RoleRef:  kernelCacheRegistryRoleRef(config.Auth.PullRoleRef),
			Subjects: []rbacv1.Subject{subject},
		},
	}
	for _, binding := range bindings {
		if err := r.ensureControllerRegistryRoleBinding(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func (r *KernelCacheCaptureControllerReconciler) ensureControllerRegistryRoleBinding(ctx context.Context, desired *rbacv1.RoleBinding) error {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	current := &rbacv1.RoleBinding{}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := r.Create(ctx, desired); err == nil {
			return nil
		} else if !apierrors.IsAlreadyExists(err) {
			return err
		}
		if err := reader.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
			return err
		}
	}
	if current.Labels[registryauth.ManagedLabel] != "true" {
		return fmt.Errorf("reserved controller registry RoleBinding %s/%s is not managed by kernelcache", desired.Namespace, desired.Name)
	}
	if current.RoleRef != desired.RoleRef {
		if err := r.Delete(ctx, current, client.Preconditions{UID: &current.UID, ResourceVersion: &current.ResourceVersion}); err != nil {
			return client.IgnoreNotFound(err)
		}
		return r.Create(ctx, desired)
	}
	if reflect.DeepEqual(current.Subjects, desired.Subjects) {
		return nil
	}
	base := current.DeepCopy()
	current.Subjects = desired.Subjects
	return r.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
}

func (r *KernelCacheCaptureControllerReconciler) cleanupControllerRegistryBindings(ctx context.Context, namespace string) error {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	for _, name := range []string{
		registryauth.ControllerPushRoleBindingName,
		registryauth.ControllerPullRoleBindingName,
	} {
		binding := &rbacv1.RoleBinding{}
		if err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, binding); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if binding.Labels[registryauth.ManagedLabel] != "true" {
			continue
		}
		if err := r.Delete(ctx, binding, client.Preconditions{UID: &binding.UID, ResourceVersion: &binding.ResourceVersion}); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func kernelCacheRegistryRoleRef(ref *v1beta1.KernelCacheRegistryRoleRef) rbacv1.RoleRef {
	return rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: ref.Kind, Name: ref.Name}
}
