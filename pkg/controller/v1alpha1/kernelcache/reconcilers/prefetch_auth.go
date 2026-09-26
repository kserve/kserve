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

	batchv1 "k8s.io/api/batch/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kernelcachelabels "github.com/kserve/kserve/pkg/kernelcache/labels"
)

// reconcilePrefetchServiceAccountAccess reconciles the prefetch ServiceAccount
// and its source-namespace RoleBinding. It returns false while a non-terminal
// prefetch Job still depends on the current binding.
func (r *KernelCacheReconciler) reconcilePrefetchServiceAccountAccess(
	ctx context.Context,
	kernelCache *v1alpha1.KernelCache,
	config *v1beta1.KernelCacheConfig,
) (bool, error) {
	requiresPullBinding := config.Registry.Auth.Type ==
		v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken

	if requiresPullBinding && config.Registry.Auth.PullRoleRef == nil {
		return false, errors.New("registry pull RoleRef is required for serviceAccountToken authentication")
	}

	if err := r.ensurePrefetchServiceAccount(ctx, config.JobNamespace); err != nil {
		return false, err
	}

	desired := buildPrefetchImagePullRoleBinding(kernelCache.Namespace, config.JobNamespace, config.Registry.Auth.PullRoleRef)
	current := &rbacv1.RoleBinding{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		if !requiresPullBinding {
			return true, nil
		}
		if err := r.Create(ctx, desired); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}

	if current.Labels[kernelCachePrefetchManagedLabel] != "true" {
		if !requiresPullBinding {
			return true, nil
		}
		return false, fmt.Errorf("reserved RoleBinding %s/%s already exists and is not managed by KernelCache", kernelCache.Namespace, kernelCachePrefetchRoleBinding)
	}
	if len(current.Subjects) != 1 || current.Subjects[0].Kind != "ServiceAccount" || current.Subjects[0].Name != kernelCachePrefetchServiceAccount {
		return false, fmt.Errorf("managed RoleBinding %s/%s has an unexpected ServiceAccount subject", kernelCache.Namespace, kernelCachePrefetchRoleBinding)
	}

	jobNamespaces := prefetchJobServiceAccountNamespaces(current, config.JobNamespace)
	hasActiveJobs, err := r.hasActivePrefetchJobsUsingSourceNamespace(ctx, kernelCache.Namespace, jobNamespaces)
	if err != nil {
		return false, err
	}
	if hasActiveJobs {
		return false, nil
	}

	if !requiresPullBinding {
		if err := r.deleteManagedPrefetchRoleBinding(ctx, current); err != nil {
			return false, err
		}
		return true, nil
	}

	if current.RoleRef != desired.RoleRef {
		if err := r.replaceManagedPrefetchRoleBinding(ctx, current, desired); err != nil {
			return false, err
		}
		return true, nil
	}

	if len(current.Subjects) == 1 && current.Subjects[0] == desired.Subjects[0] {
		return true, nil
	}

	base := current.DeepCopy()
	current.Subjects = desired.Subjects
	if err := r.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return false, err
	}
	return true, nil
}

// cleanupPrefetchRoleBindingAfterKernelCacheDeletion removes the shared pull
// RoleBinding when no KernelCache or non-terminal prefetch Job still uses it.
// It returns false while a prefetch Job still depends on the binding.
func (r *KernelCacheReconciler) cleanupPrefetchRoleBindingAfterKernelCacheDeletion(
	ctx context.Context,
	sourceNamespace string,
	config *v1beta1.KernelCacheConfig,
) (bool, error) {
	kernelCaches := &v1alpha1.KernelCacheList{}
	if err := r.List(ctx, kernelCaches, client.InNamespace(sourceNamespace)); err != nil {
		return false, err
	}
	if len(kernelCaches.Items) != 0 {
		return true, nil
	}

	binding := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: sourceNamespace, Name: kernelCachePrefetchRoleBinding}, binding); err != nil {
		return true, client.IgnoreNotFound(err)
	}
	if binding.Labels[kernelCachePrefetchManagedLabel] != "true" {
		return true, nil
	}
	if len(binding.Subjects) != 1 || binding.Subjects[0].Kind != "ServiceAccount" || binding.Subjects[0].Name != kernelCachePrefetchServiceAccount {
		return false, fmt.Errorf("managed RoleBinding %s/%s has an unexpected ServiceAccount subject", sourceNamespace, kernelCachePrefetchRoleBinding)
	}

	jobNamespaces := prefetchJobServiceAccountNamespaces(binding, config.JobNamespace)
	hasActiveJobs, err := r.hasActivePrefetchJobsUsingSourceNamespace(ctx, sourceNamespace, jobNamespaces)
	if err != nil {
		return false, err
	}
	if hasActiveJobs {
		return false, nil
	}
	return true, r.deleteManagedPrefetchRoleBinding(ctx, binding)
}

func buildPrefetchImagePullRoleBinding(sourceNamespace, jobNamespace string, roleRef *v1beta1.KernelCacheRegistryRoleRef) *rbacv1.RoleBinding {
	var desiredRoleRef rbacv1.RoleRef
	if roleRef != nil {
		desiredRoleRef = kernelCacheRegistryRoleRef(roleRef)
	}
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kernelCachePrefetchRoleBinding,
			Namespace: sourceNamespace,
			Labels:    map[string]string{kernelCachePrefetchManagedLabel: "true"},
		},
		RoleRef: desiredRoleRef,
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      kernelCachePrefetchServiceAccount,
			Namespace: jobNamespace,
		}},
	}
}

func prefetchJobServiceAccountNamespaces(binding *rbacv1.RoleBinding, desiredJobNamespace string) []string {
	namespaces := make([]string, 0, 2)
	seen := map[string]struct{}{}
	add := func(namespace string) {
		if namespace == "" {
			return
		}
		if _, exists := seen[namespace]; exists {
			return
		}
		seen[namespace] = struct{}{}
		namespaces = append(namespaces, namespace)
	}
	add(desiredJobNamespace)
	if len(binding.Subjects) == 1 && binding.Subjects[0].Kind == "ServiceAccount" {
		add(binding.Subjects[0].Namespace)
	}
	return namespaces
}

func (r *KernelCacheReconciler) hasActivePrefetchJobsUsingSourceNamespace(ctx context.Context, sourceNamespace string, jobNamespaces []string) (bool, error) {
	for _, jobNamespace := range jobNamespaces {
		jobs := &batchv1.JobList{}
		if err := r.List(ctx, jobs, client.InNamespace(jobNamespace), client.MatchingLabels{
			kernelcachelabels.KernelCacheNamespaceLabel: kernelcachelabels.Value(sourceNamespace),
		}); err != nil {
			return false, err
		}
		for index := range jobs.Items {
			if !isPrefetchJobTerminal(&jobs.Items[index]) {
				return true, nil
			}
		}
	}
	return false, nil
}

func isPrefetchJobTerminal(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed {
			return true
		}
	}
	return false
}

func (r *KernelCacheReconciler) deleteManagedPrefetchRoleBinding(ctx context.Context, binding *rbacv1.RoleBinding) error {
	return client.IgnoreNotFound(r.Delete(ctx, binding, client.Preconditions{
		UID:             &binding.UID,
		ResourceVersion: &binding.ResourceVersion,
	}))
}

func (r *KernelCacheReconciler) replaceManagedPrefetchRoleBinding(ctx context.Context, current, desired *rbacv1.RoleBinding) error {
	if err := r.validatePrefetchRoleBindingCreation(ctx, desired); err != nil {
		return fmt.Errorf("cannot apply prefetch RoleBinding %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	if err := r.deleteManagedPrefetchRoleBinding(ctx, current); err != nil {
		return err
	}
	return r.Create(ctx, desired)
}

func (r *KernelCacheReconciler) validatePrefetchRoleBindingCreation(ctx context.Context, desired *rbacv1.RoleBinding) error {
	probe := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    desired.Namespace,
			GenerateName: "kserve-kernelcache-rb-check-",
			Labels:       desired.Labels,
		},
		RoleRef:  desired.RoleRef,
		Subjects: desired.Subjects,
	}
	return r.Create(ctx, probe, client.DryRunAll)
}
