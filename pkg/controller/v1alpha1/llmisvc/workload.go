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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

const (
	// routingSidecarContainerName is the name of the routing sidecar container
	// that handles prefill disaggregation routing.
	routingSidecarContainerName = "llm-d-routing-sidecar"
)

// sidecarSSRFProtectionRules defines RBAC rules for the routing sidecar
// These permissions are needed to discover and monitor inference pools and pods.
var sidecarSSRFProtectionRules = []rbacv1.PolicyRule{
	{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
	{APIGroups: []string{"inference.networking.x-k8s.io"}, Resources: []string{"inferencepools"}, Verbs: []string{"get", "list", "watch"}},
}

// reconcileWorkload manages the Deployments and Services for the LLM.
// It handles standard, multi-node, and disaggregated (prefill/decode) deployment patterns.
func (r *LLMISVCReconciler) reconcileWorkload(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, config *Config) error {
	logger := log.FromContext(ctx).WithName("reconcileWorkload")
	ctx = log.IntoContext(ctx, logger)

	logger.Info("Reconciling Workload")

	// Ensure readiness is determined even if errors occur
	defer llmSvc.DetermineWorkloadReadiness()

	// Set up TLS certificates for secure communication
	if err := r.reconcileSelfSignedCertsSecret(ctx, llmSvc); err != nil {
		llmSvc.MarkMainWorkloadNotReady("ReconcileCertsError", err.Error())
		return fmt.Errorf("failed to reconcile self-signed certificates secret: %w", err)
	}

	// We need to always reconcile every type of workload to handle transitions from P/D to another topology (meaning
	// finalizing superfluous workloads).

	// Handle multi-node deployments using LeaderWorkerSets
	if err := r.reconcileMultiNodeWorkload(ctx, llmSvc, config); err != nil {
		llmSvc.MarkMainWorkloadNotReady("ReconcileMultiNodeWorkloadError", err.Error())
		return fmt.Errorf("failed to reconcile multi node workload: %w", err)
	}

	// Handle single-node deployments using standard Deployments
	if err := r.reconcileSingleNodeWorkload(ctx, llmSvc, config); err != nil {
		llmSvc.MarkMainWorkloadNotReady("ReconcileSingleNodeWorkloadError", err.Error())
		return fmt.Errorf("failed to reconcile single node workload: %w", err)
	}

	// Create Service to expose workload pods
	if err := r.reconcileWorkloadService(ctx, llmSvc); err != nil {
		llmSvc.MarkMainWorkloadNotReady("ReconcileWorkloadServiceError", err.Error())
		return fmt.Errorf("failed to reconcile workload service: %w", err)
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileWorkloadService(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	expected := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/component": "llminferenceservice-workload",
				"app.kubernetes.io/name":      llmSvc.GetName(),
				"app.kubernetes.io/part-of":   "llminferenceservice",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:        "https",
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: ptr.To("https"),
					Port:        8000,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 8000,
					},
				},
			},
			Selector: GetWorkloadLabelSelector(llmSvc.ObjectMeta, &llmSvc.Spec),
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	return Reconcile(ctx, r, llmSvc, &corev1.Service{}, expected, semanticServiceIsEqual)
}

func GetWorkloadLabelSelector(meta metav1.ObjectMeta, _ *v1alpha1.LLMInferenceServiceSpec) map[string]string {
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

// routingSidecar returns the routing sidecar container within a pod if present.
// The routing sidecar is used for disaggregated serving (separate prefill and decode instances) for multi node workloads
// and is used in the single node deployment topology when prefill/decode is enabled for single node workloads.
func routingSidecar(pod *corev1.PodSpec) *corev1.Container {
	if pod != nil {
		for i := range pod.InitContainers {
			if pod.InitContainers[i].Name == routingSidecarContainerName {
				return &pod.InitContainers[i]
			}
		}
	}
	return nil
}
