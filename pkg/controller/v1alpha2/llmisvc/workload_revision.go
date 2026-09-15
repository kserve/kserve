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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

const workloadRevisionLength = 8

// workloadRevisionRole is the rollout-relevant template for one disaggregated
// role. Replicas and rollout strategy are intentionally absent: scaling a role
// must not create a new compatibility boundary.
type workloadRevisionRole struct {
	Name                 string                       `json:"name"`
	DeploymentTemplate   *corev1.PodTemplateSpec      `json:"deploymentTemplate,omitempty"`
	LeaderWorkerTemplate *lwsapi.LeaderWorkerTemplate `json:"leaderWorkerTemplate,omitempty"`
}

// reconcileWorkloadRevision computes one revision from the fully rendered
// prefill and decode templates. Both roles therefore move to the same revision
// even though KServe reconciles them as independent workload resources.
func (r *LLMISVCReconciler) reconcileWorkloadRevision(
	ctx context.Context,
	llmSvc *v1alpha2.LLMInferenceService,
	config *Config,
) error {
	if llmSvc.Spec.Prefill == nil {
		config.WorkloadRevision = ""
		return nil
	}

	// Build without a generated revision label so the label cannot feed back
	// into its own hash. The normal reconciliation rebuilds these resources
	// after WorkloadRevision has been assigned.
	configWithoutRevision := *config
	configWithoutRevision.WorkloadRevision = ""

	roles := make([]workloadRevisionRole, 0, 2)
	if llmSvc.Spec.Worker == nil {
		decode, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, &configWithoutRevision)
		if err != nil {
			return fmt.Errorf("failed to build decode template for workload revision: %w", err)
		}
		roles = append(roles, workloadRevisionRole{
			Name:               constants.LLMDRoleDecode,
			DeploymentTemplate: decode.Spec.Template.DeepCopy(),
		})
	} else {
		decode, err := r.expectedMainMultiNodeLWS(ctx, llmSvc, &configWithoutRevision)
		if err != nil {
			return fmt.Errorf("failed to build decode template for workload revision: %w", err)
		}
		roles = append(roles, workloadRevisionRole{
			Name:                 constants.LLMDRoleDecode,
			LeaderWorkerTemplate: decode.Spec.LeaderWorkerTemplate.DeepCopy(),
		})
	}

	if llmSvc.Spec.Prefill.Worker == nil {
		prefill, err := r.expectedPrefillMainDeployment(ctx, llmSvc, &configWithoutRevision)
		if err != nil {
			return fmt.Errorf("failed to build prefill template for workload revision: %w", err)
		}
		roles = append(roles, workloadRevisionRole{
			Name:               constants.LLMDRolePrefill,
			DeploymentTemplate: prefill.Spec.Template.DeepCopy(),
		})
	} else {
		prefill, err := r.expectedPrefillMultiNodeLWS(ctx, llmSvc, &configWithoutRevision)
		if err != nil {
			return fmt.Errorf("failed to build prefill template for workload revision: %w", err)
		}
		roles = append(roles, workloadRevisionRole{
			Name:                 constants.LLMDRolePrefill,
			LeaderWorkerTemplate: prefill.Spec.LeaderWorkerTemplate.DeepCopy(),
		})
	}

	revision, err := computeWorkloadRevision(roles)
	if err != nil {
		return err
	}
	config.WorkloadRevision = revision
	return nil
}

func computeWorkloadRevision(roles []workloadRevisionRole) (string, error) {
	roles = deepCopyRevisionRoles(roles)
	for i := range roles {
		clearRevisionLabel(roles[i].DeploymentTemplate)
		if roles[i].LeaderWorkerTemplate != nil {
			clearRevisionLabel(&roles[i].LeaderWorkerTemplate.WorkerTemplate)
			clearRevisionLabel(roles[i].LeaderWorkerTemplate.LeaderTemplate)
		}
	}

	data, err := json.Marshal(roles)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workload templates for revision: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])[:workloadRevisionLength], nil
}

func deepCopyRevisionRoles(in []workloadRevisionRole) []workloadRevisionRole {
	out := make([]workloadRevisionRole, len(in))
	for i := range in {
		out[i].Name = in[i].Name
		if in[i].DeploymentTemplate != nil {
			out[i].DeploymentTemplate = in[i].DeploymentTemplate.DeepCopy()
		}
		if in[i].LeaderWorkerTemplate != nil {
			out[i].LeaderWorkerTemplate = in[i].LeaderWorkerTemplate.DeepCopy()
		}
	}
	return out
}

func clearRevisionLabel(template *corev1.PodTemplateSpec) {
	if template != nil {
		delete(template.Labels, constants.LLMInferenceServiceRevisionLabelKey)
	}
}

func applyWorkloadRevision(template *corev1.PodTemplateSpec, config *Config) {
	if template == nil || config == nil || config.WorkloadRevision == "" {
		return
	}

	// Clone the labels before adding the generated revision. This helper must
	// not mutate a label map that may also be referenced by another workload
	// field, such as an immutable Deployment selector.
	labels := make(map[string]string, len(template.Labels)+1)
	for key, value := range template.Labels {
		labels[key] = value
	}
	labels[constants.LLMInferenceServiceRevisionLabelKey] = config.WorkloadRevision
	template.Labels = labels
}

func applyLeaderWorkerSetWorkloadRevision(template *lwsapi.LeaderWorkerTemplate, config *Config) {
	if template == nil {
		return
	}
	applyWorkloadRevision(&template.WorkerTemplate, config)
	applyWorkloadRevision(template.LeaderTemplate, config)
}
