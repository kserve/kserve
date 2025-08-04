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
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/types"
)

func (r *LLMISVCReconciler) reconcileSingleNodeWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	log.FromContext(ctx).Info("Reconciling single-node workload")

	if err := r.reconcileSingleNodeMainServiceAccount(ctx, llmSvc, storageConfig); err != nil {
		return fmt.Errorf("failed to reconcile service account: %w", err)
	}

	if err := r.reconcileSingleNodeMainWorkload(ctx, llmSvc, storageConfig); err != nil {
		return fmt.Errorf("failed to reconcile main workload: %w", err)
	}

	if err := r.reconcileSingleNodePrefill(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile prefill workload: %w", err)
	}
	return nil
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	expected, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}
	if llmSvc.Spec.Worker != nil {
		return Delete(ctx, r, llmSvc, expected)
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, expected, semanticDeploymentIsEqual); err != nil {
		return err
	}
	return r.propagateDeploymentStatus(ctx, expected, llmSvc.MarkMainWorkloadReady, llmSvc.MarkMainWorkloadNotReady)
}

func (r *LLMISVCReconciler) expectedSingleNodeMainDeployment(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) (*appsv1.Deployment, error) {
	if llmSvc.Spec.Template == nil {
		return nil, errors.New("llmSvc.Spec.Template must not be nil")
	}

	role := "decode"
	if llmSvc.Spec.Prefill == nil {
		role = "both"
	}

	labels := r.singleNodeLabels(llmSvc)
	labels["kserve.io/component"] = "workload"
	labels["llm-d.ai/role"] = role

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: llmSvc.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}

	d.Spec.Template.Spec = *llmSvc.Spec.Template.DeepCopy()
	if hasRoutingSidecar(d.Spec.Template.Spec) {
		log.FromContext(ctx).Info("Main container has a routing sidecar")

		serviceAccount := r.expectedSingleNodeMainServiceAccount(llmSvc)
		d.Spec.Template.Spec.ServiceAccountName = serviceAccount.GetName()
		s := routingSidecar(&d.Spec.Template.Spec)
		if llmSvc.Spec.Router != nil {
			s.Env = append(s.Env, corev1.EnvVar{
				Name:      "INFERENCE_POOL_NAME",
				Value:     llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
				ValueFrom: nil,
			})
		}
	}

	if err := r.attachModelArtifacts(llmSvc, &d.Spec.Template.Spec, storageConfig); err != nil {
		return nil, fmt.Errorf("failed to attach model artifacts to main deployment: %w", err)
	}

	log.FromContext(ctx).V(2).Info("Expected main deployment", "deployment", d)

	return d, nil
}

func (r *LLMISVCReconciler) reconcileSingleNodePrefill(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	prefill := r.expectedPrefillMainDeployment(ctx, llmSvc)
	if llmSvc.Spec.Prefill == nil {
		if err := Delete(ctx, r, llmSvc, prefill); err != nil {
			return fmt.Errorf("failed to delete prefill main deployment: %w", err)
		}
		return nil
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, prefill, semanticDeploymentIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile prefill deployment %s/%s: %w", prefill.GetNamespace(), prefill.GetName(), err)
	}
	return r.propagateDeploymentStatus(ctx, prefill, llmSvc.MarkPrefillWorkloadReady, llmSvc.MarkPrefillWorkloadNotReady)
}

func (r *LLMISVCReconciler) expectedPrefillMainDeployment(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *appsv1.Deployment {
	labels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-prefill",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
		"kserve.io/component":         "workload",
		"llm-d.ai/role":               "prefill",
	}

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: labels,
		},
	}

	if llmSvc.Spec.Prefill != nil {
		d.Spec = appsv1.DeploymentSpec{
			Replicas: llmSvc.Spec.Prefill.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		}

		if llmSvc.Spec.Prefill.Template != nil {
			d.Spec.Template.Spec = *llmSvc.Spec.Prefill.Template.DeepCopy()
		}
	}

	log.FromContext(ctx).V(2).Info("Expected prefill deployment", "deployment", d)

	return d
}

func (r *LLMISVCReconciler) propagateDeploymentStatus(ctx context.Context, expected *appsv1.Deployment, ready func(), notReady func(reason, messageFormat string, messageA ...interface{})) error {
	logger := log.FromContext(ctx)

	curr := &appsv1.Deployment{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Client.Get(ctx, client.ObjectKeyFromObject(expected), curr)
	})
	if err != nil {
		return fmt.Errorf("failed to get current deployment %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	for _, cond := range curr.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable {
			if cond.Status == corev1.ConditionTrue {
				ready()
			} else {
				notReady(cond.Reason, cond.Message)
			}
			return nil
		}
	}
	logger.Info("Deployment processed")
	notReady(string(appsv1.DeploymentProgressing), "")
	return nil
}

func semanticDeploymentIsEqual(expected *appsv1.Deployment, curr *appsv1.Deployment) bool {
	return equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	serviceAccount := r.expectedSingleNodeMainServiceAccount(llmSvc)
	if !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, serviceAccount)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
	}

	if err := r.reconcileSingleNodeMainRole(ctx, llmSvc, storageConfig); err != nil {
		return err
	}

	return r.reconcileSingleNodeMainRoleBinding(ctx, llmSvc, serviceAccount, storageConfig)
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainRole(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *types.StorageInitializerConfig) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	role := r.expectedSingleNodeRole(llmSvc)
	if !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, role)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.Role{}, role, semanticRoleIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node role %s/%s: %w", role.GetNamespace(), role.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainRoleBinding(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount, storageConfig *types.StorageInitializerConfig) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, storageConfig)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	roleBinding := r.expectedSingleNodeRoleBinding(llmSvc, sa)
	if !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, roleBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.RoleBinding{}, roleBinding, semanticRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node rolebinding %s/%s: %w", roleBinding.GetNamespace(), roleBinding.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) expectedSingleNodeMainServiceAccount(llmSvc *v1alpha1.LLMInferenceService) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.singleNodeLabels(llmSvc),
		},
	}
}

func (r *LLMISVCReconciler) expectedSingleNodeRole(llmSvc *v1alpha1.LLMInferenceService) *rbacv1.Role {
	ro := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-role"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.singleNodeLabels(llmSvc),
		},
	}
	ro.Rules = append(ro.Rules, sidecarSSRFProtectionRules...)
	return ro
}

func (r *LLMISVCReconciler) expectedSingleNodeRoleBinding(llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-rb"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.singleNodeLabels(llmSvc),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      sa.GetName(),
			Namespace: sa.GetNamespace(),
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     kmeta.ChildName(llmSvc.GetName(), "-kserve-role"),
		},
	}
}

func (r *LLMISVCReconciler) singleNodeLabels(llmSvc *v1alpha1.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
}
