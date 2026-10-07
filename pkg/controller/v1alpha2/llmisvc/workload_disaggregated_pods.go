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

// The renderers in this file build the pod part of each DisaggregatedSet role. Each one
// mirrors the pod logic of a Deployment or LeaderWorkerSet builder, so that a service
// runs the same pods whichever backend it uses. Keep them in sync with those builders
// until the two are unified into a common renderer; TestDisaggregatedRolesMatchWorkloadBuilders
// fails when they drift apart.

import (
	"context"
	"fmt"
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// Top-level metadata keys propagated onto role pod templates and the LeaderWorkerSets a
// DisaggregatedSet creates. They match the lists of the Deployment and LeaderWorkerSet
// builders respectively.
var (
	disaggregatedSingleNodeAnnotationPrefixes = []string{
		"k8s.v1.cni.cncf.io",
		constants.KueueAPIGroupName,
		"prometheus.io",
		constants.LocalModelLabel,
	}
	disaggregatedMultiNodeAnnotationPrefixes = []string{
		"leaderworkerset.sigs.k8s.io",
		"k8s.v1.cni.cncf.io",
		constants.KueueAPIGroupName,
		"prometheus.io",
		constants.LocalModelLabel,
	}
	disaggregatedLabelPrefixes = []string{
		constants.KueueAPIGroupName,
		constants.LocalModelLabel,
	}
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
// service. It mirrors the pod logic of expectedSingleNodeMainDeployment.
func (r *LLMISVCReconciler) disaggregatedSingleNodeDecodeTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	labels := r.singleNodeLabels(llmSvc)
	labels[constants.KServeComponentLabelKey] = constants.KServeComponentWorkload
	labels[constants.LLMDRoleLabelKey] = constants.LLMDRoleDecode

	annotationsToPass := []string{
		"prometheus.io/scrape",
		"prometheus.io/port",
		"prometheus.io/path",
		"prometheus.io/scheme",
	}
	podAnnotations := map[string]string{}
	llmSvcAnnotations := llmSvc.GetAnnotations()
	for _, annotationKey := range annotationsToPass {
		if _, ok := llmSvcAnnotations[annotationKey]; ok {
			podAnnotations[annotationKey] = llmSvcAnnotations[annotationKey]
		}
	}

	if err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, labels); err != nil {
		return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
	}

	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      maps.Clone(labels),
			Annotations: podAnnotations,
		},
	}

	if llmSvc.Spec.Template != nil {
		template.Spec = *llmSvc.Spec.Template.DeepCopy()

		var serviceAccount *corev1.ServiceAccount
		if hasRoutingSidecar(template.Spec) {
			log.FromContext(ctx).Info("Main container has a routing sidecar")

			var err error
			serviceAccount, _, err = r.expectedSingleNodeMainServiceAccount(ctx, llmSvc)
			if err != nil {
				return nil, fmt.Errorf("failed to created expected single node service account: %w", err)
			}
			template.Spec.ServiceAccountName = serviceAccount.GetName()
			s := routingSidecar(&template.Spec)
			if llmSvc.Spec.Router != nil {
				s.Env = append(s.Env, corev1.EnvVar{
					Name:  "INFERENCE_POOL_NAME",
					Value: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
				})
			}
		} else if llmSvc.Spec.Template.ServiceAccountName != "" {
			serviceAccount = &corev1.ServiceAccount{}
			if err := r.Get(ctx, types.NamespacedName{Name: llmSvc.Spec.Template.ServiceAccountName, Namespace: llmSvc.Namespace}, serviceAccount); err != nil {
				return nil, fmt.Errorf("failed to fetch existing single node main service account %s/%s: %w", llmSvc.Namespace, llmSvc.Spec.Template.ServiceAccountName, err)
			}
		}

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, deployed.Worker, &template.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to decode role: %w", err)
		}
		if llmSvc.Spec.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&template.Spec, llmSvc.Spec.KVCacheOffloading.Secondary, "main")
		}
	}

	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &template.Annotations, disaggregatedSingleNodeAnnotationPrefixes...)
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &template.Labels, disaggregatedLabelPrefixes...)
	utils.PropagateMap(llmSvc.Spec.Labels, &template.Labels)
	utils.PropagateMap(llmSvc.Spec.Annotations, &template.Annotations, routingSpecAnnotations...)

	if llmSvc.Spec.Tracing != nil {
		if mainIdx := slices.IndexFunc(template.Spec.Containers, func(c corev1.Container) bool { return c.Name == "main" }); mainIdx >= 0 {
			injectServerTracing(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-decode", &template.Spec.Containers[mainIdx])
		}
	}

	return singleNodeRoleTemplate(llmSvc, labels, template), nil
}

// disaggregatedSingleNodePrefillTemplate renders the prefill role of a single-node
// service. It mirrors the pod logic of expectedPrefillMainDeployment.
func (r *LLMISVCReconciler) disaggregatedSingleNodePrefillTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	labels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadPrefill,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
		constants.KServeComponentLabelKey:     constants.KServeComponentWorkload,
		constants.LLMDRoleLabelKey:            constants.LLMDRolePrefill,
	}

	if err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, labels); err != nil {
		return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
	}

	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: maps.Clone(labels),
		},
	}

	prefill := llmSvc.Spec.Prefill
	if prefill.Template != nil {
		template.Spec = *prefill.Template.DeepCopy()

		var serviceAccount *corev1.ServiceAccount
		if prefill.Template.ServiceAccountName != "" {
			serviceAccount = &corev1.ServiceAccount{}
			if err := r.Get(ctx, types.NamespacedName{Name: prefill.Template.ServiceAccountName, Namespace: llmSvc.Namespace}, serviceAccount); err != nil {
				return nil, fmt.Errorf("failed to fetch existing single node prefill service account %s/%s: %w", llmSvc.Namespace, prefill.Template.ServiceAccountName, err)
			}
		}

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, deployed.Worker, &template.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to prefill role: %w", err)
		}
		if prefill.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&template.Spec, prefill.KVCacheOffloading.Secondary, "main")
		}
	}

	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &template.Annotations, disaggregatedSingleNodeAnnotationPrefixes...)
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &template.Labels, disaggregatedLabelPrefixes...)
	utils.PropagateMap(prefill.Labels, &template.Labels)
	utils.PropagateMap(prefill.Annotations, &template.Annotations, routingSpecAnnotations...)

	if llmSvc.Spec.Tracing != nil {
		if mainIdx := slices.IndexFunc(template.Spec.Containers, func(c corev1.Container) bool { return c.Name == "main" }); mainIdx >= 0 {
			injectServerTracing(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-prefill", &template.Spec.Containers[mainIdx])
		}
	}

	return singleNodeRoleTemplate(llmSvc, labels, template), nil
}

// singleNodeRoleTemplate wraps a single-node pod template into a group of one. Without a
// leader template, a LeaderWorkerSet runs the worker template as the group's only pod.
// The role metadata matches what the Deployment builders put on the Deployment.
func singleNodeRoleTemplate(llmSvc *v1alpha2.LLMInferenceService, identityLabels map[string]string, template corev1.PodTemplateSpec) *disaggregatedRoleTemplate {
	roleLabels := maps.Clone(identityLabels)
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &roleLabels, disaggregatedLabelPrefixes...)
	var roleAnnotations map[string]string
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &roleAnnotations, disaggregatedSingleNodeAnnotationPrefixes...)

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

// disaggregatedMultiNodeDecodeTemplate renders the decode role of a multi-node service.
// It mirrors the pod logic of expectedMainMultiNodeLWS.
func (r *LLMISVCReconciler) disaggregatedMultiNodeDecodeTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	workerLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadWorker,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
	if llmSvc.Spec.Template == nil {
		// When there is no leader template, workers become part of the InferencePool selector.
		workerLabels[constants.KServeComponentLabelKey] = constants.KServeComponentWorkload
		workerLabels[constants.LLMDRoleLabelKey] = constants.LLMDRoleDecode

		if err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, workerLabels); err != nil {
			return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
		}
	}
	leaderLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadLeader,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
		constants.KServeComponentLabelKey:     constants.KServeComponentWorkload,
		constants.LLMDRoleLabelKey:            constants.LLMDRoleDecode,
	}

	group := lwsapi.LeaderWorkerTemplate{
		Size: llmSvc.Spec.Parallelism.GetSize(),
		WorkerTemplate: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: workerLabels,
			},
		},
		RestartPolicy: lwsapi.RecreateGroupOnPodRestart,
	}

	serviceAccount, _, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to create expected multi node service account: %w", err)
	}

	if llmSvc.Spec.Template != nil {
		if err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, leaderLabels); err != nil {
			return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
		}

		group.LeaderTemplate = &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: leaderLabels,
			},
			Spec: *llmSvc.Spec.Template.DeepCopy(),
		}
		group.LeaderTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, ptr.Deref(deployed.Leader, corev1.PodSpec{}), &group.LeaderTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to decode leader template: %w", err)
		}
		if llmSvc.Spec.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&group.LeaderTemplate.Spec, llmSvc.Spec.KVCacheOffloading.Secondary, "main")
		}

		if hasRoutingSidecar(group.LeaderTemplate.Spec) && llmSvc.Spec.Router != nil {
			s := routingSidecar(&group.LeaderTemplate.Spec)
			s.Env = append(s.Env, corev1.EnvVar{
				Name:  "INFERENCE_POOL_NAME",
				Value: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
			})
		}
	}
	if llmSvc.Spec.Worker != nil {
		group.WorkerTemplate.Spec = *llmSvc.Spec.Worker.DeepCopy()
		group.WorkerTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, deployed.Worker, &group.WorkerTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to decode worker template: %w", err)
		}
		if llmSvc.Spec.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&group.WorkerTemplate.Spec, llmSvc.Spec.KVCacheOffloading.Secondary, "main")
		}

		if hasRoutingSidecar(group.WorkerTemplate.Spec) && llmSvc.Spec.Router != nil {
			s := routingSidecar(&group.WorkerTemplate.Spec)
			s.Env = append(s.Env, corev1.EnvVar{
				Name:  "INFERENCE_POOL_NAME",
				Value: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
			})
		}
	}

	propagateDisaggregatedGroupMetadata(llmSvc, &group)

	if group.LeaderTemplate != nil {
		utils.PropagateMap(llmSvc.Spec.Labels, &group.LeaderTemplate.Labels)
		utils.PropagateMap(llmSvc.Spec.Annotations, &group.LeaderTemplate.Annotations, routingSpecAnnotations...)
	}
	utils.PropagateMap(llmSvc.Spec.Labels, &group.WorkerTemplate.Labels)
	utils.PropagateMap(llmSvc.Spec.Annotations, &group.WorkerTemplate.Annotations, routingSpecAnnotations...)

	if llmSvc.Spec.Tracing != nil {
		if group.LeaderTemplate != nil {
			injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-decode", &group.LeaderTemplate.Spec)
		}
		injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-decode", &group.WorkerTemplate.Spec)
	}

	return multiNodeRoleTemplate(llmSvc, group), nil
}

// disaggregatedMultiNodePrefillTemplate renders the prefill role of a multi-node
// service. It mirrors the pod logic of expectedPrefillMultiNodeLWS.
func (r *LLMISVCReconciler) disaggregatedMultiNodePrefillTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed deployedRoleSpecs) (*disaggregatedRoleTemplate, error) {
	prefill := llmSvc.Spec.Prefill

	workerLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadWorkerPrefill,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
	if prefill.Template == nil {
		// When there is no leader template, workers become part of the InferencePool selector.
		workerLabels[constants.KServeComponentLabelKey] = constants.KServeComponentWorkload
		workerLabels[constants.LLMDRoleLabelKey] = constants.LLMDRolePrefill

		if err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, workerLabels); err != nil {
			return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
		}
	}
	leaderLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadLeaderPrefill,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
		constants.KServeComponentLabelKey:     constants.KServeComponentWorkload,
		constants.LLMDRoleLabelKey:            constants.LLMDRolePrefill,
	}

	group := lwsapi.LeaderWorkerTemplate{
		Size: prefill.Parallelism.GetSize(),
		WorkerTemplate: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: workerLabels,
			},
		},
		RestartPolicy: lwsapi.RecreateGroupOnPodRestart,
	}

	serviceAccount, _, err := r.expectedMultiNodePrefillServiceAccount(ctx, llmSvc)
	if err != nil {
		return nil, fmt.Errorf("failed to create expected multi node service account: %w", err)
	}

	if prefill.Template != nil {
		if err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, leaderLabels); err != nil {
			return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
		}

		group.LeaderTemplate = &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: leaderLabels,
			},
			Spec: *prefill.Template.DeepCopy(),
		}
		group.LeaderTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, ptr.Deref(deployed.Leader, corev1.PodSpec{}), &group.LeaderTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to prefill leader template: %w", err)
		}
		if prefill.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&group.LeaderTemplate.Spec, prefill.KVCacheOffloading.Secondary, "main")
		}
	}
	if prefill.Worker != nil {
		group.WorkerTemplate.Spec = *prefill.Worker.DeepCopy()
		group.WorkerTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, deployed.Worker, &group.WorkerTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to prefill worker template: %w", err)
		}
		if prefill.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&group.WorkerTemplate.Spec, prefill.KVCacheOffloading.Secondary, "main")
		}
	}

	if prefill.Parallelism.IsDataParallel() && group.Size != nil {
		group.SubGroupPolicy = &lwsapi.SubGroupPolicy{
			SubGroupSize: group.Size,
		}
	}

	propagateDisaggregatedGroupMetadata(llmSvc, &group)

	if llmSvc.Spec.Tracing != nil {
		if group.LeaderTemplate != nil {
			injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-prefill", &group.LeaderTemplate.Spec)
		}
		injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-prefill", &group.WorkerTemplate.Spec)
	}

	if group.LeaderTemplate != nil {
		utils.PropagateMap(prefill.Labels, &group.LeaderTemplate.Labels)
		utils.PropagateMap(prefill.Annotations, &group.LeaderTemplate.Annotations, routingSpecAnnotations...)
	}
	utils.PropagateMap(prefill.Labels, &group.WorkerTemplate.Labels)
	utils.PropagateMap(prefill.Annotations, &group.WorkerTemplate.Annotations, routingSpecAnnotations...)

	return multiNodeRoleTemplate(llmSvc, group), nil
}

// propagateDisaggregatedGroupMetadata propagates approved top-level annotations and
// labels onto the leader and worker pod templates.
func propagateDisaggregatedGroupMetadata(llmSvc *v1alpha2.LLMInferenceService, group *lwsapi.LeaderWorkerTemplate) {
	if group.LeaderTemplate != nil {
		utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &group.LeaderTemplate.Annotations, disaggregatedMultiNodeAnnotationPrefixes...)
		utils.PropagatePrefixedMap(llmSvc.GetLabels(), &group.LeaderTemplate.Labels, disaggregatedLabelPrefixes...)
	}
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &group.WorkerTemplate.Annotations, disaggregatedMultiNodeAnnotationPrefixes...)
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &group.WorkerTemplate.Labels, disaggregatedLabelPrefixes...)
}

// multiNodeRoleTemplate completes a multi-node role. The role metadata matches what the
// LeaderWorkerSet builders put on the LeaderWorkerSet, which shares its labels with the
// worker template.
func multiNodeRoleTemplate(llmSvc *v1alpha2.LLMInferenceService, group lwsapi.LeaderWorkerTemplate) *disaggregatedRoleTemplate {
	var roleAnnotations map[string]string
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &roleAnnotations, disaggregatedMultiNodeAnnotationPrefixes...)

	return &disaggregatedRoleTemplate{
		Template:    group,
		Labels:      maps.Clone(group.WorkerTemplate.Labels),
		Annotations: roleAnnotations,
	}
}
