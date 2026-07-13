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

package llmisvc

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

// reconcileControllerNamespaceSecretRBAC grants the llmisvc controller service account
// namespace-scoped secret permissions in the LLMInferenceService namespace. This avoids
// granting cluster-wide secret access via the manager ClusterRole.
func (r *LLMISVCReconciler) reconcileControllerNamespaceSecretRBAC(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	role := expectedControllerNamespaceSecretsRole(llmSvc)
	if err := Reconcile[*v1alpha2.LLMInferenceService, *rbacv1.Role](ctx, r, nil, &rbacv1.Role{}, role, semanticRoleIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile controller secrets role %s/%s: %w", role.Namespace, role.Name, err)
	}

	roleBinding := expectedControllerNamespaceSecretsRoleBinding(llmSvc)
	if err := Reconcile[*v1alpha2.LLMInferenceService, *rbacv1.RoleBinding](ctx, r, nil, &rbacv1.RoleBinding{}, roleBinding, semanticRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile controller secrets rolebinding %s/%s: %w", roleBinding.Namespace, roleBinding.Name, err)
	}

	return nil
}

func expectedControllerNamespaceSecretsRole(llmSvc *v1alpha2.LLMInferenceService) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.LLMISvcControllerSecretsRoleName,
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesPartOfLabelKey: constants.LLMInferenceServicePartOfValue,
				"app.kubernetes.io/component":      "controller-namespace-rbac",
			},
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
		}},
	}
}

func expectedControllerNamespaceSecretsRoleBinding(llmSvc *v1alpha2.LLMInferenceService) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.LLMISvcControllerSecretsRoleBindingName,
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesPartOfLabelKey: constants.LLMInferenceServicePartOfValue,
				"app.kubernetes.io/component":      "controller-namespace-rbac",
			},
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      constants.LLMISvcControllerServiceAccountName,
			Namespace: constants.KServeNamespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     constants.LLMISvcControllerSecretsRoleName,
		},
	}
}
