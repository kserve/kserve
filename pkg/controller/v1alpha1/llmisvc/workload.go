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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// reconcileWorkload manages the Deployments and Services for the LLM.
// It handles standard, multi-node, and disaggregated (prefill/decode) deployment patterns.
func (r *LLMISVCReconciler) reconcileWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	logger := log.FromContext(ctx).WithName("reconcileWorkload")
	ctx = log.IntoContext(ctx, logger)

	defer llmSvc.DetermineWorkloadReadiness()

	// We need to always reconcile every type of workload to handle transitions from P/D to another topology (meaning
	// finalizing superfluous workloads).

	if err := r.reconcileMultiNodeWorkload(ctx, llmSvc); err != nil {
		llmSvc.MarkWorkloadNotReady("ReconcileMultiNodeWorkloadError", err.Error())
		return fmt.Errorf("failed to reconcile multi node workload: %w", err)
	}

	if err := r.reconcileSingleNodeWorkload(ctx, llmSvc); err != nil {
		llmSvc.MarkWorkloadNotReady("ReconcileSingleNodeWorkloadError", err.Error())
		return fmt.Errorf("failed to reconcile single node workload: %w", err)
	}

	return nil
}

func getWorkloadLabelSelector(meta metav1.ObjectMeta, spec *v1alpha1.LLMInferenceServiceSpec) map[string]string {
	s := map[string]string{
		"app.kubernetes.io/name": meta.GetName(),
	}

	componentLabelValue := "llminferenceservice-workload"
	if spec.Worker != nil {
		if spec.Template == nil {
			// If there is no separate leader pod spec, send requests to the worker with index 0.
			componentLabelValue = "llminferenceservice-workload-worker"
			s["leaderworkerset.sigs.k8s.io/worker-index"] = "0"
		} else {
			componentLabelValue = "llminferenceservice-workload-leader"
		}
	}
	s["app.kubernetes.io/component"] = componentLabelValue

	return s
}
