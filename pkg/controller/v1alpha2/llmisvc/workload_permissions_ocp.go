//go:build distro

/*
Copyright 2025 The KServe Authors.

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
	"k8s.io/utils/env"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// sccDisabled indicates whether SCC role binding reconciliation is disabled at runtime.
// When set to "true", the controller will skip creating SCC RoleBinding resources,
// useful for environments where SecurityContextConstraints are not needed.
var sccDisabled, _ = env.GetBool("LLMISVC_SCC_DISABLED", false)

func (r *LLMISVCReconciler) reconcileWorkloadPlatformPermissions(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	if sccDisabled {
		log.FromContext(ctx).V(2).Info("SCC is disabled via LLMISVC_SCC_DISABLED, skipping SCC role binding reconciliation")
		return nil
	}
	if err := r.reconcileMultiNodeSCCRoleBinding(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile multi-node SCC role binding: %w", err)
	}
	return nil
}

func (r *LLMISVCReconciler) reconcileMultiNodeSCCRoleBinding(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	expected, err := r.expectedMultiNodeSCCRoleBinding(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to create expected multi node scc role binding: %w", err)
	}
	if utils.GetForceStopRuntime(llmSvc) || (llmSvc.Spec.Worker == nil && (llmSvc.Spec.Prefill == nil || llmSvc.Spec.Prefill.Worker == nil)) {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &rbacv1.RoleBinding{}, expected, semanticRoleBindingIsEqual)
}

func (r *LLMISVCReconciler) expectedMultiNodeSCCRoleBinding(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*rbacv1.RoleBinding, error) {
	m, _, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to create expected multi node main service account: %w", err)
	}
	p, _, err := r.expectedMultiNodePrefillServiceAccount(ctx, llmSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to create expected multi node prefill service account: %w", err)
	}

	expected := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-scc"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesAppNameLabelKey: llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "openshift-ai-llminferenceservice-scc",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: m.Name,
			},
			{
				Kind: "ServiceAccount",
				Name: p.Name,
			},
		},
	}

	log.FromContext(ctx).V(2).Info("Expected SCC multi-node role binding", "rolebinding", expected)

	return expected, nil
}
