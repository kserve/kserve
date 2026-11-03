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

// The renderers in this file build the pod part of each DisaggregatedSet role from the
// same pod template renderers the Deployment and LeaderWorkerSet builders use, so that a
// service runs the same pods whichever backend it uses.

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/utils"
)

// deployedRoleSpecs are the pod specs a role currently runs, read from the
// DisaggregatedSet. Leader is nil when the role has no leader template.
type deployedRoleSpecs struct {
	Leader *corev1.PodSpec
	Worker corev1.PodSpec
}

// disaggregatedRoleTemplate is one rendered role: its group template and the metadata
// of the LeaderWorkerSets the DisaggregatedSet creates for it.
type disaggregatedRoleTemplate struct {
	Template    lwsapi.LeaderWorkerTemplate
	Labels      map[string]string
	Annotations map[string]string
}

// disaggregatedSingleNodeDecodeTemplate renders the decode role of a single-node
// service with the pod template renderer of expectedSingleNodeMainDeployment.
func (r *LLMISVCReconciler) disaggregatedSingleNodeDecodeTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	pod, err := r.expectedSingleNodeMainPodTemplate(ctx, llmSvc, config, deployed.Worker)
	if err != nil {
		return nil, err
	}
	return singleNodeRoleTemplate(llmSvc, pod.IdentityLabels, pod.Template), nil
}

// disaggregatedSingleNodePrefillTemplate renders the prefill role of a single-node
// service with the pod template renderer of expectedPrefillMainDeployment.
func (r *LLMISVCReconciler) disaggregatedSingleNodePrefillTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	pod, err := r.expectedSingleNodePrefillPodTemplate(ctx, llmSvc, config, deployed.Worker)
	if err != nil {
		return nil, err
	}
	return singleNodeRoleTemplate(llmSvc, pod.IdentityLabels, pod.Template), nil
}

// singleNodeRoleTemplate wraps a single-node pod template into a group of one. Without a
// leader template, a LeaderWorkerSet runs the worker template as the group's only pod.
// The role metadata matches what the Deployment builders put on the Deployment.
func singleNodeRoleTemplate(llmSvc *v1alpha2.LLMInferenceService, identityLabels map[string]string, template corev1.PodTemplateSpec) *disaggregatedRoleTemplate {
	roleLabels := maps.Clone(identityLabels)
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &roleLabels, deploymentApprovedLabelPrefixes...)
	var roleAnnotations map[string]string
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &roleAnnotations, deploymentApprovedAnnotationPrefixes...)

	return &disaggregatedRoleTemplate{
		Template: lwsapi.LeaderWorkerTemplate{
			Size:           ptr.To[int32](1),
			WorkerTemplate: template,
			// Restart a failed container in place, as the Deployment does. Recreating
			// the group would replace the pod and run its init containers, such as
			// the model download, again.
			RestartPolicy: lwsapi.NoneRestartPolicy,
		},
		Labels:      roleLabels,
		Annotations: roleAnnotations,
	}
}

// disaggregatedMultiNodeDecodeTemplate renders the decode role of a multi-node service
// with the group template renderer of expectedMainMultiNodeLWS.
func (r *LLMISVCReconciler) disaggregatedMultiNodeDecodeTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	group, err := r.expectedMultiNodeMainLeaderWorkerTemplate(ctx, llmSvc, config, deployed.Leader, deployed.Worker)
	if err != nil {
		return nil, err
	}
	return multiNodeRoleTemplate(llmSvc, group.Template), nil
}

// disaggregatedMultiNodePrefillTemplate renders the prefill role of a multi-node service
// with the group template renderer of expectedPrefillMultiNodeLWS.
func (r *LLMISVCReconciler) disaggregatedMultiNodePrefillTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	group, err := r.expectedMultiNodePrefillLeaderWorkerTemplate(ctx, llmSvc, config, deployed.Leader, deployed.Worker)
	if err != nil {
		return nil, err
	}
	return multiNodeRoleTemplate(llmSvc, group.Template), nil
}

// multiNodeRoleTemplate completes a multi-node role. The role metadata matches what the
// LeaderWorkerSet builders put on the LeaderWorkerSet, which shares its labels with the
// worker template.
func multiNodeRoleTemplate(llmSvc *v1alpha2.LLMInferenceService, group lwsapi.LeaderWorkerTemplate) *disaggregatedRoleTemplate {
	var roleAnnotations map[string]string
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &roleAnnotations, leaderWorkerSetApprovedAnnotationPrefixes...)

	return &disaggregatedRoleTemplate{
		Template:    group,
		Labels:      maps.Clone(group.WorkerTemplate.Labels),
		Annotations: roleAnnotations,
	}
}
