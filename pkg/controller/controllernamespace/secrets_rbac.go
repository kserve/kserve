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

package controllernamespace

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/constants"
)

// ReconcileKServeControllerSecretRBAC grants the kserve controller service account
// namespace-scoped secret read access in the target namespace.
func ReconcileKServeControllerSecretRBAC(ctx context.Context, c client.Client, namespace string) error {
	role := expectedKServeControllerSecretsRole(namespace)
	curr := &rbacv1.Role{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(role), curr); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get controller secrets role %s/%s: %w", role.Namespace, role.Name, err)
		}
		if err := c.Create(ctx, role); err != nil {
			return fmt.Errorf("failed to create controller secrets role %s/%s: %w", role.Namespace, role.Name, err)
		}
	} else if !kserveControllerSecretsRoleEqual(role, curr) {
		role.SetResourceVersion(curr.GetResourceVersion())
		if err := c.Update(ctx, role); err != nil {
			return fmt.Errorf("failed to update controller secrets role %s/%s: %w", role.Namespace, role.Name, err)
		}
	}

	roleBinding := expectedKServeControllerSecretsRoleBinding(namespace)
	currBinding := &rbacv1.RoleBinding{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(roleBinding), currBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get controller secrets rolebinding %s/%s: %w", roleBinding.Namespace, roleBinding.Name, err)
		}
		if err := c.Create(ctx, roleBinding); err != nil {
			return fmt.Errorf("failed to create controller secrets rolebinding %s/%s: %w", roleBinding.Namespace, roleBinding.Name, err)
		}
	} else if !kserveControllerSecretsRoleBindingEqual(roleBinding, currBinding) {
		roleBinding.SetResourceVersion(currBinding.GetResourceVersion())
		if err := c.Update(ctx, roleBinding); err != nil {
			return fmt.Errorf("failed to update controller secrets rolebinding %s/%s: %w", roleBinding.Namespace, roleBinding.Name, err)
		}
	}

	return nil
}

func expectedKServeControllerSecretsRole(namespace string) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.KServeControllerSecretsRoleName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.KubernetesPartOfLabelKey: "kserve",
				"app.kubernetes.io/component":      "controller-namespace-rbac",
			},
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get"},
		}},
	}
}

func expectedKServeControllerSecretsRoleBinding(namespace string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.KServeControllerSecretsRoleBindingName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.KubernetesPartOfLabelKey: "kserve",
				"app.kubernetes.io/component":      "controller-namespace-rbac",
			},
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      constants.KServeControllerServiceAccountName,
			Namespace: constants.KServeNamespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     constants.KServeControllerSecretsRoleName,
		},
	}
}

func kserveControllerSecretsRoleEqual(expected, current *rbacv1.Role) bool {
	return equality.Semantic.DeepDerivative(expected.Rules, current.Rules) &&
		equality.Semantic.DeepDerivative(expected.Labels, current.Labels)
}

func kserveControllerSecretsRoleBindingEqual(expected, current *rbacv1.RoleBinding) bool {
	return equality.Semantic.DeepDerivative(expected.Subjects, current.Subjects) &&
		equality.Semantic.DeepDerivative(expected.RoleRef, current.RoleRef) &&
		equality.Semantic.DeepDerivative(expected.Labels, current.Labels)
}
