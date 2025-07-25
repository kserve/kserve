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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	kserveTypes "github.com/kserve/kserve/pkg/types"
)

const (
	routingSidecarContainerName = "llm-d-routing-sidecar"
)

var sidecarSSRFProtectionRules = []rbacv1.PolicyRule{
	{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
	{APIGroups: []string{"inference.networking.x-k8s.io"}, Resources: []string{"inferencepools"}, Verbs: []string{"get", "list", "watch"}},
}

// reconcileWorkload manages the Deployments and Services for the LLM.
// It handles standard, multi-node, and disaggregated (prefill/decode) deployment patterns.
func (r *LLMInferenceServiceReconciler) reconcileWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, storageConfig *kserveTypes.StorageInitializerConfig) error {
	logger := log.FromContext(ctx).WithName("reconcileWorkload")
	ctx = log.IntoContext(ctx, logger)

	logger.Info("Reconciling Workload")

	defer llmSvc.DetermineWorkloadReadiness()

	if err := r.reconcileSelfSignedCertsSecret(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile self-signed certificates secret: %w", err)
	}

	// We need to always reconcile every type of workload to handle transitions from P/D to another topology (meaning
	// finalizing superfluous workloads).

	if err := r.reconcileMultiNodeWorkload(ctx, llmSvc); err != nil {
		llmSvc.MarkWorkloadNotReady("ReconcileMultiNodeWorkloadError", err.Error())
		return fmt.Errorf("failed to reconcile multi node workload: %w", err)
	}

	if err := r.reconcileSingleNodeWorkload(ctx, llmSvc, storageConfig); err != nil {
		llmSvc.MarkWorkloadNotReady("ReconcileSingleNodeWorkloadError", err.Error())
		return fmt.Errorf("failed to reconcile single node workload: %w", err)
	}

	return nil
}

func getInferencePoolWorkloadLabelSelector(meta metav1.ObjectMeta, _ *v1alpha1.LLMInferenceServiceSpec) map[string]string {
	s := map[string]string{
		"app.kubernetes.io/part-of": "llminferenceservice",
		"app.kubernetes.io/name":    meta.GetName(),
		"kserve.io/component":       "workload",
	}

	// TODO https://github.com/llm-d/llm-d-inference-scheduler/issues/220 and DP template

	return s
}

func hasRoutingSidecar(pod corev1.PodSpec) bool {
	return routingSidecar(&pod) != nil
}

func routingSidecar(pod *corev1.PodSpec) *corev1.Container {
	for i := range pod.InitContainers {
		if pod.InitContainers[i].Name == routingSidecarContainerName {
			return &pod.InitContainers[i]
		}
	}
	return nil
}
