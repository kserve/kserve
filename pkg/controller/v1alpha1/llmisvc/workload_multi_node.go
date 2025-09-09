/*
Copyright 2023 The KServe Authors.

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
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/credentials"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

func (r *LLMInferenceServiceReconciler) reconcileMultiNodeWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	log.FromContext(ctx).Info("Reconciling multi-node workload")

	if err := r.reconcileMultiNodeMainServiceAccount(ctx, llmSvc, storageConfig, credentialConfig); err != nil {
		return fmt.Errorf("failed to reconcile multi-node service account: %w", err)
	}
	if err := r.reconcileMultiNodePrefillServiceAccount(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile multi-node service account: %w", err)
	}
	if err := r.reconcileMultiNodeOCPRoleBinding(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile multi-node role binding: %w", err)
	}
	if err := r.reconcileMultiNodeMainWorkload(ctx, llmSvc, storageConfig, credentialConfig); err != nil {
		return fmt.Errorf("failed to reconcile multi-node main workload: %w", err)
	}
	if err := r.reconcileMultiNodePrefillWorkload(ctx, llmSvc, storageConfig, credentialConfig); err != nil {
		return fmt.Errorf("failed to reconcile multi-node prefill workload: %w", err)
	}
	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileMultiNodeMainWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	expected, err := r.expectedMainMultiNodeLWS(ctx, llmSvc, storageConfig, credentialConfig)
	if err != nil {
		return fmt.Errorf("failed to build the expected main LWS: %w", err)
	}

	if llmSvc.Spec.Worker == nil {
		if err := Delete(ctx, r, llmSvc, expected); err != nil {
			return err
		}
		return nil
	}
	if err := Reconcile(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{}, expected, semanticLWSIsEqual); err != nil {
		return err
	}
	return r.propagateLeaderWorkerSetStatus(ctx, expected, llmSvc.MarkMainWorkloadReady, llmSvc.MarkMainWorkloadNotReady)
}

func (r *LLMInferenceServiceReconciler) reconcileMultiNodePrefillWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	expected, err := r.expectedPrefillMultiNodeLWS(ctx, llmSvc, storageConfig, credentialConfig)
	if err != nil {
		return fmt.Errorf("failed to build the expected prefill LWS: %w", err)
	}
	if llmSvc.Spec.Prefill == nil || llmSvc.Spec.Prefill.Worker == nil {
		if err := Delete(ctx, r, llmSvc, expected); err != nil {
			return err
		}
		return nil
	}
	if err := Reconcile(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{}, expected, semanticLWSIsEqual); err != nil {
		return err
	}
	return r.propagateLeaderWorkerSetStatus(ctx, expected, llmSvc.MarkPrefillWorkloadReady, llmSvc.MarkPrefillWorkloadNotReady)
}

func (r *LLMInferenceServiceReconciler) propagateLeaderWorkerSetStatus(ctx context.Context, expected *lwsapi.LeaderWorkerSet, ready func(), notReady func(reason string, messageFormat string, messageA ...interface{})) error {
	logger := log.FromContext(ctx)

	curr := &lwsapi.LeaderWorkerSet{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Client.Get(ctx, client.ObjectKeyFromObject(expected), curr)
	})
	if err != nil {
		return fmt.Errorf("failed to get current leaderworkerset %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	for _, cond := range curr.Status.Conditions {
		if cond.Type == string(lwsapi.LeaderWorkerSetAvailable) {
			if cond.Status == metav1.ConditionTrue {
				ready()
			} else {
				notReady(cond.Reason, cond.Message)
			}
			return nil
		}
	}
	logger.Info("LeaderWorkerSet processed")
	notReady(string(lwsapi.LeaderWorkerSetProgressing), "")
	return nil
}

func (r *LLMInferenceServiceReconciler) expectedMainMultiNodeLWS(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) (*lwsapi.LeaderWorkerSet, error) {
	workerLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-worker",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
	if llmSvc.Spec.Template == nil {
		// When there is no leader template, workers become part of the InferencePool selector.
		workerLabels["kserve.io/component"] = "workload"
		workerLabels["llm-d.ai/role"] = "decode"
	}
	role := "decode"
	if llmSvc.Spec.Prefill == nil {
		role = "both"
	}
	leaderLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-leader",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
		"kserve.io/component":         "workload",
		"llm-d.ai/role":               role,
	}

	expected := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: workerLabels,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			Replicas: llmSvc.Spec.Replicas,
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				Size: llmSvc.Spec.Parallelism.GetSize(),
				WorkerTemplate: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: workerLabels,
					},
				},
				RestartPolicy: lwsapi.RecreateGroupOnPodRestart,
			},
			RolloutStrategy: lwsapi.RolloutStrategy{
				Type: lwsapi.RollingUpdateStrategyType,
			},
			StartupPolicy: lwsapi.LeaderCreatedStartupPolicy,
		},
	}

	if llmSvc.Spec.Template != nil {
		expected.Spec.LeaderWorkerTemplate.LeaderTemplate = &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: leaderLabels,
			},
			Spec: *llmSvc.Spec.Template.DeepCopy(),
		}

		serviceAccount, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
		if err != nil {
			return nil, fmt.Errorf("failed to create expected multi node service account: %w", err)
		}
		expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, llmSvc, &expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec, storageConfig, credentialConfig); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to leader template: %w", err)
		}

		if hasRoutingSidecar(expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec) {
			log.FromContext(ctx).V(2).Info("Main container has a routing sidecar")

			s := routingSidecar(&expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec)
			if llmSvc.Spec.Router != nil {
				s.Env = append(s.Env, corev1.EnvVar{
					Name:  "INFERENCE_POOL_NAME",
					Value: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
				})
			}
		}
	}
	if llmSvc.Spec.Worker != nil {
		expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec = *llmSvc.Spec.Worker.DeepCopy()

		serviceAccount, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
		if err != nil {
			return nil, fmt.Errorf("failed to create expected multi node service account: %w", err)
		}
		expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, llmSvc, &expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec, storageConfig, credentialConfig); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to worker template: %w", err)
		}

		if hasRoutingSidecar(expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec) {
			log.FromContext(ctx).V(2).Info("Main (worker) container has a routing sidecar")

			s := routingSidecar(&expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec)
			if llmSvc.Spec.Router != nil {
				s.Env = append(s.Env, corev1.EnvVar{
					Name:  "INFERENCE_POOL_NAME",
					Value: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
				})
			}
		}
	}

	r.propagateLeaderWorkerSetMetadata(llmSvc, expected)

	log.FromContext(ctx).V(2).Info("Expected main LWS", "leaderworkerset", expected)

	return expected, nil
}

func (r *LLMInferenceServiceReconciler) expectedPrefillMultiNodeLWS(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) (*lwsapi.LeaderWorkerSet, error) {
	workerLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-worker-prefill",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Template == nil {
		// When there is no leader template, workers become part of the InferencePool selector.
		workerLabels["kserve.io/component"] = "workload"
		workerLabels["llm-d.ai/role"] = "prefill"
	}
	leaderLabels := map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-workload-leader-prefill",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
		"kserve.io/component":         "workload",
		"llm-d.ai/role":               "prefill",
	}

	expected := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-prefill"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: workerLabels,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: lwsapi.LeaderWorkerTemplate{
				WorkerTemplate: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: workerLabels,
					},
				},
				RestartPolicy: lwsapi.RecreateGroupOnPodRestart,
			},
			RolloutStrategy: lwsapi.RolloutStrategy{
				Type: lwsapi.RollingUpdateStrategyType,
			},
			StartupPolicy: lwsapi.LeaderCreatedStartupPolicy,
		},
	}

	if llmSvc.Spec.Prefill != nil {
		expected.Spec.Replicas = llmSvc.Spec.Prefill.Replicas
		expected.Spec.LeaderWorkerTemplate.Size = llmSvc.Spec.Prefill.Parallelism.GetSize()

		serviceAccount, err := r.expectedMultiNodePrefillServiceAccount(ctx, llmSvc)
		if err != nil {
			return nil, fmt.Errorf("failed to create exptected multi node service account: %w", err)
		}

		if llmSvc.Spec.Prefill.Template != nil {
			expected.Spec.LeaderWorkerTemplate.LeaderTemplate = &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: leaderLabels,
				},
				Spec: *llmSvc.Spec.Prefill.Template.DeepCopy(),
			}
			expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

			if err := r.attachModelArtifacts(ctx, llmSvc, &expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec, storageConfig, credentialConfig); err != nil {
				return nil, fmt.Errorf("failed to attach model artifacts to prefill leader template: %w", err)
			}
		}
		if llmSvc.Spec.Prefill.Worker != nil {
			expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec = *llmSvc.Spec.Prefill.Worker.DeepCopy()
			expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

			if err := r.attachModelArtifacts(ctx, llmSvc, &expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec, storageConfig, credentialConfig); err != nil {
				return nil, fmt.Errorf("failed to attach model artifacts to prefill worker template: %w", err)
			}
		}

		if llmSvc.Spec.Prefill.Parallelism.IsDataParallel() && expected.Spec.LeaderWorkerTemplate.Size != nil {
			expected.Spec.LeaderWorkerTemplate.SubGroupPolicy = &lwsapi.SubGroupPolicy{
				SubGroupSize: expected.Spec.LeaderWorkerTemplate.Size,
			}
		}
	}

	r.propagateLeaderWorkerSetMetadata(llmSvc, expected)

	log.FromContext(ctx).V(2).Info("Expected prefill LWS", "leaderworkerset", expected)

	return expected, nil
}

func (r *LLMInferenceServiceReconciler) reconcileMultiNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	serviceAccount, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to create expected multi node service account: %w", err)
	}
	if llmSvc.Spec.Worker == nil {
		return Delete(ctx, r, llmSvc, serviceAccount)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile multi node service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
	}

	if err := r.reconcileMultiNodeMainRole(ctx, llmSvc, storageConfig, credentialConfig); err != nil {
		return err
	}

	return r.reconcileMultiNodeMainRoleBinding(ctx, llmSvc, serviceAccount, storageConfig, credentialConfig)
}

func (r *LLMInferenceServiceReconciler) reconcileMultiNodePrefillServiceAccount(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	serviceAccount, err := r.expectedMultiNodePrefillServiceAccount(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to create expected multi node service account: %w", err)
	}
	if llmSvc.Spec.Prefill == nil || llmSvc.Spec.Prefill.Worker == nil {
		return Delete(ctx, r, llmSvc, serviceAccount)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile multi node service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
	}

	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileMultiNodeMainRole(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	lws, err := r.expectedMainMultiNodeLWS(ctx, llmSvc, storageConfig, credentialConfig)
	if err != nil {
		return fmt.Errorf("failed to build the expected main LWS for building the Role: %w", err)
	}

	role := r.expectedMultiNodeMainRole(llmSvc)
	if !hasRoutingSidecar(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec) && (lws.Spec.LeaderWorkerTemplate.LeaderTemplate == nil || !hasRoutingSidecar(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec)) {
		return Delete(ctx, r, llmSvc, role)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.Role{}, role, semanticRoleIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile multi node role %s/%s: %w", role.GetNamespace(), role.GetName(), err)
	}

	return nil
}

func (r *LLMInferenceServiceReconciler) reconcileMultiNodeMainRoleBinding(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount, storageConfig *kserveTypes.StorageInitializerConfig, credentialConfig *credentials.CredentialConfig) error {
	lws, err := r.expectedMainMultiNodeLWS(ctx, llmSvc, storageConfig, credentialConfig)
	if err != nil {
		return fmt.Errorf("failed to build the expected main LWS for building the RoleBinding: %w", err)
	}

	roleBinding := r.expectedMultiNodeRoleBinding(llmSvc, sa)
	if !hasRoutingSidecar(lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec) && (lws.Spec.LeaderWorkerTemplate.LeaderTemplate == nil || !hasRoutingSidecar(lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec)) {
		return Delete(ctx, r, llmSvc, roleBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.RoleBinding{}, roleBinding, semanticRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile multi node rolebinding %s/%s: %w", roleBinding.GetNamespace(), roleBinding.GetName(), err)
	}

	return nil
}

func (r *LLMInferenceServiceReconciler) expectedMultiNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) (*corev1.ServiceAccount, error) {
	expectedServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
	}

	// An existing service account attached to the main leader template takes precedence over any attached to the prefill worker template.
	var existingServiceAccountName string
	if llmSvc.Spec.Template != nil && llmSvc.Spec.Template.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Template.ServiceAccountName
	} else if llmSvc.Spec.Worker != nil && llmSvc.Spec.Worker.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Worker.ServiceAccountName
	}

	// Copy all relevant data from the existing service account to the created service account
	if existingServiceAccountName != "" {
		existingServiceAccount := &corev1.ServiceAccount{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: existingServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch existing multi node service account %s/%s: %w", existingServiceAccountName, llmSvc.Namespace, err)
		}
		expectedServiceAccount.Annotations = existingServiceAccount.Annotations
		expectedServiceAccount.Labels = existingServiceAccount.Labels
		expectedServiceAccount.Secrets = existingServiceAccount.Secrets
		expectedServiceAccount.ImagePullSecrets = existingServiceAccount.ImagePullSecrets
		expectedServiceAccount.AutomountServiceAccountToken = existingServiceAccount.AutomountServiceAccountToken
	}

	// Add required labels to the created service account
	if expectedServiceAccount.Labels == nil {
		expectedServiceAccount.Labels = make(map[string]string)
	}
	expectedServiceAccount.Labels["app.kubernetes.io/name"] = llmSvc.GetName()
	expectedServiceAccount.Labels["app.kubernetes.io/part-of"] = "llminferenceservice"

	return expectedServiceAccount, nil
}

func (r *LLMInferenceServiceReconciler) expectedMultiNodePrefillServiceAccount(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) (*corev1.ServiceAccount, error) {
	expectedServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-prefill"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
	}

	// An existing service account attached to the prefill leader template takes precedence over any attached to the prefill worker template.
	var existingServiceAccountName string
	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Template != nil && llmSvc.Spec.Prefill.Template.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Prefill.Template.ServiceAccountName
	} else if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Worker != nil && llmSvc.Spec.Prefill.Worker.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Prefill.Worker.ServiceAccountName
	}

	// Copy all relevant data from the existing service account to the created service account
	if existingServiceAccountName != "" {
		existingServiceAccount := &corev1.ServiceAccount{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: existingServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch existing multi node service account %s/%s: %w", existingServiceAccountName, llmSvc.Namespace, err)
		}
		expectedServiceAccount.Annotations = existingServiceAccount.Annotations
		expectedServiceAccount.Labels = existingServiceAccount.Labels
		expectedServiceAccount.Secrets = existingServiceAccount.Secrets
		expectedServiceAccount.ImagePullSecrets = existingServiceAccount.ImagePullSecrets
		expectedServiceAccount.AutomountServiceAccountToken = existingServiceAccount.AutomountServiceAccountToken
	}

	// Add required labels to the created service account
	if expectedServiceAccount.Labels == nil {
		expectedServiceAccount.Labels = make(map[string]string)
	}
	expectedServiceAccount.Labels["app.kubernetes.io/name"] = llmSvc.GetName()
	expectedServiceAccount.Labels["app.kubernetes.io/part-of"] = "llminferenceservice"

	return expectedServiceAccount, nil
}

func (r *LLMInferenceServiceReconciler) expectedMultiNodeMainRole(llmSvc *v1alpha1.LLMInferenceService) *rbacv1.Role {
	ro := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-role"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: map[string]string{
				"app.kubernetes.io/name":    llmSvc.GetName(),
				"app.kubernetes.io/part-of": "llminferenceservice",
			},
		},
	}
	ro.Rules = append(ro.Rules, sidecarSSRFProtectionRules...)
	return ro
}

func (r *LLMInferenceServiceReconciler) expectedMultiNodeRoleBinding(llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-rb"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: map[string]string{
				"app.kubernetes.io/name":    llmSvc.GetName(),
				"app.kubernetes.io/part-of": "llminferenceservice",
			},
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      sa.GetName(),
			Namespace: sa.GetNamespace(),
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-role"),
		},
	}
}

func (r *LLMInferenceServiceReconciler) propagateLeaderWorkerSetMetadata(llmSvc *v1alpha1.LLMInferenceService, expected *lwsapi.LeaderWorkerSet) {
	ann := make(map[string]string, len(expected.Annotations))
	for k, v := range llmSvc.GetAnnotations() {
		if strings.HasPrefix(k, "leaderworkerset.sigs.k8s.io") ||
			strings.HasPrefix(k, "k8s.v1.cni.cncf.io") {
			ann[k] = v
			if expected.Annotations == nil {
				expected.Annotations = make(map[string]string, 1)
			}
			expected.Annotations[k] = v
		}
	}

	if expected.Spec.LeaderWorkerTemplate.LeaderTemplate != nil {
		if expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Annotations == nil {
			expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Annotations = ann
		} else {
			for k, v := range ann {
				expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Annotations[k] = v
			}
		}
	}

	if expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations == nil {
		expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations = ann
	} else {
		for k, v := range ann {
			expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Annotations[k] = v
		}
	}
}

func semanticLWSIsEqual(expected *lwsapi.LeaderWorkerSet, curr *lwsapi.LeaderWorkerSet) bool {
	isLeaderEqual := true
	if (expected.Spec.LeaderWorkerTemplate.LeaderTemplate != nil) != (curr.Spec.LeaderWorkerTemplate.LeaderTemplate != nil) {
		isLeaderEqual = false
	}
	if expected.Spec.LeaderWorkerTemplate.LeaderTemplate != nil && curr.Spec.LeaderWorkerTemplate.LeaderTemplate != nil {
		// Use DeepEqual for the Pod Spec so that when fields are removed (like resource requirements, we push them down
		// to the child resource)
		isLeaderEqual = equality.Semantic.DeepEqual(
			expected.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec,
			curr.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec,
		)
	}

	// Use DeepEqual for the Pod Spec so that when fields are removed (like resource requirements, we push them down
	// to the child resource)
	isWorkerEqual := equality.Semantic.DeepEqual(expected.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec, curr.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec)

	return isLeaderEqual &&
		isWorkerEqual &&
		equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}
