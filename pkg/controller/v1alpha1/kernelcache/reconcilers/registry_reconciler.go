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

func kernelCacheRegistryRoleRef(ref *v1beta1.KernelCacheRegistryRoleRef) rbacv1.RoleRef {
	return rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: ref.Kind, Name: ref.Name}
}
