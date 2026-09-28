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
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

func (r *LLMISVCReconciler) reconcileMultiNodeWorkload(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	log.FromContext(ctx).Info("Reconciling multi-node workload")

	if err := r.reconcileManagedDRA(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile managed DRA: %w", err)
	}

	if err := r.reconcileMultiNodeMainServiceAccount(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile multi-node service account: %w", err)
	}
	if err := r.reconcileMultiNodePrefillServiceAccount(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile multi-node service account: %w", err)
	}
	if err := r.reconcileMultiNodeMainWorkload(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile multi-node main workload: %w", err)
	}
	if err := r.reconcileMultiNodePrefillWorkload(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile multi-node prefill workload: %w", err)
	}
	return nil
}

func (r *LLMISVCReconciler) reconcileMultiNodeMainWorkload(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	if isStopped := utils.GetForceStopRuntime(llmSvc); isStopped || llmSvc.Spec.Worker == nil {
		if isStopped {
			llmSvc.MarkWorkerWorkloadNotReady("Stopped", "Service is stopped")
		} else {
			llmSvc.MarkWorkerWorkloadUnset()
		}
		return Delete(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mainLWSName(llmSvc),
				Namespace: llmSvc.GetNamespace(),
			},
		})
	}

	expected, err := r.expectedMainMultiNodeLWS(ctx, llmSvc, config)
	if err != nil {
		return fmt.Errorf("failed to build the expected main LWS: %w", err)
	}
	if err := Reconcile(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{}, expected, semanticLWSIsEqual, PreserveLWSReplicas()); err != nil {
		return err
	}
	return r.propagateLeaderWorkerSetStatus(ctx, expected, llmSvc.MarkWorkerWorkloadReady, llmSvc.MarkWorkerWorkloadNotReady)
}

func (r *LLMISVCReconciler) reconcileMultiNodePrefillWorkload(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	if isStopped := utils.GetForceStopRuntime(llmSvc); isStopped || llmSvc.Spec.Prefill == nil || llmSvc.Spec.Prefill.Worker == nil {
		if isStopped {
			llmSvc.MarkPrefillWorkerWorkloadNotReady("Stopped", "Service is stopped")
		} else {
			llmSvc.MarkPrefillWorkerWorkloadUnset()
		}
		return Delete(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefillLWSName(llmSvc),
				Namespace: llmSvc.GetNamespace(),
			},
		})
	}

	expected, err := r.expectedPrefillMultiNodeLWS(ctx, llmSvc, config)
	if err != nil {
		return fmt.Errorf("failed to build the expected prefill LWS: %w", err)
	}
	if err := Reconcile(ctx, r, llmSvc, &lwsapi.LeaderWorkerSet{}, expected, semanticLWSIsEqual, PreserveLWSReplicas()); err != nil {
		return err
	}
	return r.propagateLeaderWorkerSetStatus(ctx, expected, llmSvc.MarkPrefillWorkerWorkloadReady, llmSvc.MarkPrefillWorkerWorkloadNotReady)
}

func (r *LLMISVCReconciler) propagateLeaderWorkerSetStatus(ctx context.Context, expected *lwsapi.LeaderWorkerSet, ready func(), notReady func(reason string, messageFormat string, messageA ...interface{})) error {
	logger := log.FromContext(ctx)

	curr := &lwsapi.LeaderWorkerSet{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Get(ctx, client.ObjectKeyFromObject(expected), curr)
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
	notReady(string(lwsapi.LeaderWorkerSetProgressing), "LWS is progressing")
	return nil
}

func (r *LLMISVCReconciler) expectedMainMultiNodeLWS(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) (*lwsapi.LeaderWorkerSet, error) {
	key := types.NamespacedName{Name: mainLWSName(llmSvc), Namespace: llmSvc.GetNamespace()}

	// Fetch the deployed templates once to preserve storage-init images across upgrades.
	deployedLeader, deployedWorker, err := r.deployedLeaderWorkerPodSpecs(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get current leader worker set %s/%s: %w", key.Namespace, key.Name, err)
	}

	group, err := r.expectedMultiNodeMainLeaderWorkerTemplate(ctx, llmSvc, config, deployedLeader, deployedWorker)
	if err != nil {
		return nil, err
	}

	expected := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: group.ObjectLabels,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			Replicas:             llmSvc.Spec.Replicas,
			LeaderWorkerTemplate: group.Template,
			RolloutStrategy: lwsapi.RolloutStrategy{
				Type:                       lwsapi.RollingUpdateStrategyType,
				RollingUpdateConfiguration: rollingUpdateConfigFromWorkloadSpec(&llmSvc.Spec.WorkloadSpec),
			},
			StartupPolicy: lwsapi.LeaderCreatedStartupPolicy,
		},
	}

	propagateLeaderWorkerSetObjectAnnotations(llmSvc, expected)

	log.FromContext(ctx).V(2).Info("Expected main LWS", "leaderworkerset", expected)

	return expected, nil
}

// multiNodeTemplate is a rendered multi-node group template. ObjectLabels are the
// labels of the LeaderWorkerSet object: the worker pod labels as they stand before
// the workload revision is applied, because the object has always shared its label
// map with the worker template.
type multiNodeTemplate struct {
	Template     lwsapi.LeaderWorkerTemplate
	ObjectLabels map[string]string
}

// expectedMultiNodeMainLeaderWorkerTemplate renders the decode group template of a
// multi-node workload. deployedLeader (nil when there is none) and deployedWorker are
// the pod specs currently deployed for decode, used to keep storage-initializer
// settings stable across controller upgrades.
func (r *LLMISVCReconciler) expectedMultiNodeMainLeaderWorkerTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployedLeader *corev1.PodSpec, deployedWorker corev1.PodSpec) (*multiNodeTemplate, error) {
	workerLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadWorker,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
	if llmSvc.Spec.Template == nil {
		// When there is no leader template, workers become part of the InferencePool selector.
		workerLabels[constants.KServeComponentLabelKey] = constants.KServeComponentWorkload
		workerLabels[constants.LLMDRoleLabelKey] = constants.LLMDRoleDecode

		err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, workerLabels)
		if err != nil {
			return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
		}
	}
	role := constants.LLMDRoleDecode
	if llmSvc.Spec.Prefill == nil {
		role = constants.LLMDRoleBoth
	}
	leaderLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadLeader,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
		constants.KServeComponentLabelKey:     constants.KServeComponentWorkload,
		constants.LLMDRoleLabelKey:            role,
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

	if llmSvc.Spec.Template != nil && !utils.GetForceStopRuntime(llmSvc) {
		err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, leaderLabels)
		if err != nil {
			return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
		}

		group.LeaderTemplate = &corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: leaderLabels,
			},
			Spec: *llmSvc.Spec.Template.DeepCopy(),
		}

		serviceAccount, _, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
		if err != nil {
			return nil, fmt.Errorf("failed to create expected multi node service account: %w", err)
		}
		group.LeaderTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, ptr.Deref(deployedLeader, corev1.PodSpec{}), &group.LeaderTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to leader template: %w", err)
		}
		if llmSvc.Spec.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&group.LeaderTemplate.Spec, llmSvc.Spec.KVCacheOffloading.Secondary, "main")
		}

		if hasRoutingSidecar(group.LeaderTemplate.Spec) {
			log.FromContext(ctx).V(2).Info("Main container has a routing sidecar")

			s := routingSidecar(&group.LeaderTemplate.Spec)
			if llmSvc.Spec.Router != nil {
				s.Env = append(s.Env, corev1.EnvVar{
					Name:  "INFERENCE_POOL_NAME",
					Value: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
				})
			}
		}
	}
	if llmSvc.Spec.Worker != nil && !utils.GetForceStopRuntime(llmSvc) {
		group.WorkerTemplate.Spec = *llmSvc.Spec.Worker.DeepCopy()

		serviceAccount, _, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
		if err != nil {
			return nil, fmt.Errorf("failed to create expected multi node service account: %w", err)
		}
		group.WorkerTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, deployedWorker, &group.WorkerTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to worker template: %w", err)
		}
		if llmSvc.Spec.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&group.WorkerTemplate.Spec, llmSvc.Spec.KVCacheOffloading.Secondary, "main")
		}

		if hasRoutingSidecar(group.WorkerTemplate.Spec) {
			log.FromContext(ctx).V(2).Info("Main (worker) container has a routing sidecar")

			s := routingSidecar(&group.WorkerTemplate.Spec)
			if llmSvc.Spec.Router != nil {
				s.Env = append(s.Env, corev1.EnvVar{
					Name:  "INFERENCE_POOL_NAME",
					Value: llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
				})
			}
		}
	}

	propagateLeaderWorkerTemplateMetadata(llmSvc, &group)

	if group.LeaderTemplate != nil {
		utils.PropagateMap(llmSvc.Spec.Labels, &group.LeaderTemplate.Labels)
		utils.PropagateMap(llmSvc.Spec.Annotations, &group.LeaderTemplate.Annotations, AnnotationModelBasedRoutingEnabled)
	}
	utils.PropagateMap(llmSvc.Spec.Labels, &group.WorkerTemplate.Labels)
	utils.PropagateMap(llmSvc.Spec.Annotations, &group.WorkerTemplate.Annotations, AnnotationModelBasedRoutingEnabled)

	// Inject tracing instrumentation when spec.tracing is set
	if llmSvc.Spec.Tracing != nil {
		if group.LeaderTemplate != nil {
			injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-decode", &group.LeaderTemplate.Spec)
		}
		injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-decode", &group.WorkerTemplate.Spec)
	}

	applyLeaderWorkerSetWorkloadRevision(&group, config)

	return &multiNodeTemplate{Template: group, ObjectLabels: workerLabels}, nil
}

func (r *LLMISVCReconciler) expectedPrefillMultiNodeLWS(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) (*lwsapi.LeaderWorkerSet, error) {
	key := types.NamespacedName{Name: prefillLWSName(llmSvc), Namespace: llmSvc.GetNamespace()}
	active := llmSvc.Spec.Prefill != nil && !utils.GetForceStopRuntime(llmSvc)

	var deployedLeader *corev1.PodSpec
	var deployedWorker corev1.PodSpec
	if active {
		var err error
		if deployedLeader, deployedWorker, err = r.deployedLeaderWorkerPodSpecs(ctx, key); err != nil {
			return nil, fmt.Errorf("failed to get current prefill leader worker set %s/%s: %w", key.Namespace, key.Name, err)
		}
	}

	group, err := r.expectedMultiNodePrefillLeaderWorkerTemplate(ctx, llmSvc, config, deployedLeader, deployedWorker)
	if err != nil {
		return nil, err
	}

	expected := &lwsapi.LeaderWorkerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: group.ObjectLabels,
		},
		Spec: lwsapi.LeaderWorkerSetSpec{
			LeaderWorkerTemplate: group.Template,
			RolloutStrategy: lwsapi.RolloutStrategy{
				Type:                       lwsapi.RollingUpdateStrategyType,
				RollingUpdateConfiguration: rollingUpdateConfigFromPrefill(llmSvc.Spec.Prefill),
			},
			StartupPolicy: lwsapi.LeaderCreatedStartupPolicy,
		},
	}
	if active {
		expected.Spec.Replicas = llmSvc.Spec.Prefill.Replicas
	}

	propagateLeaderWorkerSetObjectAnnotations(llmSvc, expected)

	log.FromContext(ctx).V(2).Info("Expected prefill LWS", "leaderworkerset", expected)

	return expected, nil
}

// expectedMultiNodePrefillLeaderWorkerTemplate renders the prefill group template of a
// multi-node workload. deployedLeader (nil when there is none) and deployedWorker are
// the pod specs currently deployed for prefill, used to keep storage-initializer
// settings stable across controller upgrades.
func (r *LLMISVCReconciler) expectedMultiNodePrefillLeaderWorkerTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployedLeader *corev1.PodSpec, deployedWorker corev1.PodSpec) (*multiNodeTemplate, error) {
	workerLabels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadWorkerPrefill,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Template == nil {
		// When there is no leader template, workers become part of the InferencePool selector.
		workerLabels[constants.KServeComponentLabelKey] = constants.KServeComponentWorkload
		workerLabels[constants.LLMDRoleLabelKey] = constants.LLMDRolePrefill

		err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, workerLabels)
		if err != nil {
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
		WorkerTemplate: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: workerLabels,
			},
		},
		RestartPolicy: lwsapi.RecreateGroupOnPodRestart,
	}

	if llmSvc.Spec.Prefill != nil && !utils.GetForceStopRuntime(llmSvc) {
		group.Size = llmSvc.Spec.Prefill.Parallelism.GetSize()

		serviceAccount, _, err := r.expectedMultiNodePrefillServiceAccount(ctx, llmSvc)
		if err != nil {
			return nil, fmt.Errorf("failed to create expected multi node service account: %w", err)
		}

		if llmSvc.Spec.Prefill.Template != nil {
			err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, leaderLabels)
			if err != nil {
				return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
			}

			group.LeaderTemplate = &corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: leaderLabels,
				},
				Spec: *llmSvc.Spec.Prefill.Template.DeepCopy(),
			}

			group.LeaderTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

			if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, ptr.Deref(deployedLeader, corev1.PodSpec{}), &group.LeaderTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
				return nil, fmt.Errorf("failed to attach model artifacts to prefill leader template: %w", err)
			}
			if llmSvc.Spec.Prefill.KVCacheOffloading != nil {
				attachKVCacheSecondaryTiers(&group.LeaderTemplate.Spec, llmSvc.Spec.Prefill.KVCacheOffloading.Secondary, "main")
			}
		}
		if llmSvc.Spec.Prefill.Worker != nil {
			group.WorkerTemplate.Spec = *llmSvc.Spec.Prefill.Worker.DeepCopy()

			group.WorkerTemplate.Spec.ServiceAccountName = serviceAccount.GetName()

			if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, deployedWorker, &group.WorkerTemplate.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
				return nil, fmt.Errorf("failed to attach model artifacts to prefill worker template: %w", err)
			}
			if llmSvc.Spec.Prefill.KVCacheOffloading != nil {
				attachKVCacheSecondaryTiers(&group.WorkerTemplate.Spec, llmSvc.Spec.Prefill.KVCacheOffloading.Secondary, "main")
			}
		}

		if llmSvc.Spec.Prefill.Parallelism.IsDataParallel() && group.Size != nil {
			group.SubGroupPolicy = &lwsapi.SubGroupPolicy{
				SubGroupSize: group.Size,
			}
		}
	}

	propagateLeaderWorkerTemplateMetadata(llmSvc, &group)

	// Inject tracing instrumentation when spec.tracing is set
	if llmSvc.Spec.Tracing != nil {
		if group.LeaderTemplate != nil {
			injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-prefill", &group.LeaderTemplate.Spec)
		}
		injectServerTracingIntoPodSpec(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-prefill", &group.WorkerTemplate.Spec)
	}

	if llmSvc.Spec.Prefill != nil {
		if group.LeaderTemplate != nil {
			utils.PropagateMap(llmSvc.Spec.Prefill.Labels, &group.LeaderTemplate.Labels)
			utils.PropagateMap(llmSvc.Spec.Prefill.Annotations, &group.LeaderTemplate.Annotations, AnnotationModelBasedRoutingEnabled)
		}
		utils.PropagateMap(llmSvc.Spec.Prefill.Labels, &group.WorkerTemplate.Labels)
		utils.PropagateMap(llmSvc.Spec.Prefill.Annotations, &group.WorkerTemplate.Annotations, AnnotationModelBasedRoutingEnabled)
	}

	applyLeaderWorkerSetWorkloadRevision(&group, config)

	return &multiNodeTemplate{Template: group, ObjectLabels: workerLabels}, nil
}

func (r *LLMISVCReconciler) reconcileMultiNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	serviceAccount, useExistingServiceAccount, err := r.expectedMultiNodeMainServiceAccount(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to create expected multi node service account: %w", err)
	}
	if !useExistingServiceAccount {
		if utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Worker == nil {
			return Delete(ctx, r, llmSvc, serviceAccount)
		}

		if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile multi node service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
		}
	}
	if err := r.reconcileMultiNodeMainRole(ctx, llmSvc, config); err != nil {
		return err
	}

	return r.reconcileMultiNodeMainRoleBinding(ctx, llmSvc, serviceAccount, config)
}

func (r *LLMISVCReconciler) reconcileMultiNodePrefillServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	serviceAccount, useExistingServiceAccount, err := r.expectedMultiNodePrefillServiceAccount(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to create expected multi node service account: %w", err)
	}
	if !useExistingServiceAccount {
		if utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Prefill == nil || llmSvc.Spec.Prefill.Worker == nil {
			return Delete(ctx, r, llmSvc, serviceAccount)
		}

		if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile multi node service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
		}
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileMultiNodeMainRole(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	lws, err := r.expectedMainMultiNodeLWS(ctx, llmSvc, config)
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

func (r *LLMISVCReconciler) reconcileMultiNodeMainRoleBinding(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount, config *Config) error {
	lws, err := r.expectedMainMultiNodeLWS(ctx, llmSvc, config)
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

func (r *LLMISVCReconciler) expectedMultiNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*corev1.ServiceAccount, bool, error) {
	useExistingServiceAccount := false
	expectedServiceAccountName := mainLWSName(llmSvc)

	// An existing service account attached to the main leader template takes precedence over any attached to the prefill worker template.
	var existingServiceAccountName string
	if llmSvc.Spec.Template != nil && llmSvc.Spec.Template.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Template.ServiceAccountName
	} else if llmSvc.Spec.Worker != nil && llmSvc.Spec.Worker.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Worker.ServiceAccountName
	}

	if existingServiceAccountName != "" && existingServiceAccountName != expectedServiceAccountName {
		useExistingServiceAccount = true
		log.FromContext(ctx).V(2).Info("Using existing service account for multi node main workload", "serviceAccountName", existingServiceAccountName)
		existingServiceAccount := &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{Name: existingServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
		if err != nil {
			return nil, useExistingServiceAccount, fmt.Errorf("failed to fetch existing multi node main service account %s/%s: %w", llmSvc.Namespace, existingServiceAccountName, err)
		}
		return existingServiceAccount, useExistingServiceAccount, nil
	}

	expectedServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedServiceAccountName,
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}

	r.injectSecretsFromDefaultServiceAccount(ctx, expectedServiceAccount)

	// Add required labels to the created service account
	if expectedServiceAccount.Labels == nil {
		expectedServiceAccount.Labels = make(map[string]string)
	}
	expectedServiceAccount.Labels[constants.KubernetesAppNameLabelKey] = llmSvc.GetName()
	expectedServiceAccount.Labels[constants.KubernetesPartOfLabelKey] = constants.LLMInferenceServicePartOfValue

	return expectedServiceAccount, useExistingServiceAccount, nil
}

func (r *LLMISVCReconciler) expectedMultiNodePrefillServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*corev1.ServiceAccount, bool, error) {
	useExistingServiceAccount := false
	expectedServiceAccountName := prefillLWSName(llmSvc)

	// An existing service account attached to the prefill leader template takes precedence over any attached to the prefill worker template.
	var existingServiceAccountName string
	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Template != nil && llmSvc.Spec.Prefill.Template.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Prefill.Template.ServiceAccountName
	} else if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Worker != nil && llmSvc.Spec.Prefill.Worker.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Prefill.Worker.ServiceAccountName
	}

	if existingServiceAccountName != "" && existingServiceAccountName != expectedServiceAccountName {
		useExistingServiceAccount = true
		log.FromContext(ctx).V(2).Info("Using existing service account for multi node prefill workload", "serviceAccountName", existingServiceAccountName)
		existingServiceAccount := &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{Name: existingServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
		if err != nil {
			return nil, useExistingServiceAccount, fmt.Errorf("failed to fetch existing multi node prefill service account %s/%s: %w", llmSvc.Namespace, existingServiceAccountName, err)
		}
		return existingServiceAccount, useExistingServiceAccount, nil
	}

	expectedServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedServiceAccountName,
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}

	r.injectSecretsFromDefaultServiceAccount(ctx, expectedServiceAccount)

	// Add required labels to the created service account
	if expectedServiceAccount.Labels == nil {
		expectedServiceAccount.Labels = make(map[string]string)
	}
	expectedServiceAccount.Labels[constants.KubernetesAppNameLabelKey] = llmSvc.GetName()
	expectedServiceAccount.Labels[constants.KubernetesPartOfLabelKey] = constants.LLMInferenceServicePartOfValue

	return expectedServiceAccount, useExistingServiceAccount, nil
}

func (r *LLMISVCReconciler) expectedMultiNodeMainRole(llmSvc *v1alpha2.LLMInferenceService) *rbacv1.Role {
	ro := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-role"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: map[string]string{
				constants.KubernetesAppNameLabelKey: llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
			},
		},
	}
	ro.Rules = append(ro.Rules, sidecarSSRFProtectionRules...)
	return ro
}

func (r *LLMISVCReconciler) expectedMultiNodeRoleBinding(llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-rb"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: map[string]string{
				constants.KubernetesAppNameLabelKey: llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
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

// Top-level metadata keys propagated to LeaderWorkerSets and their pod templates.
var (
	leaderWorkerSetApprovedAnnotationPrefixes = []string{
		"leaderworkerset.sigs.k8s.io",
		"k8s.v1.cni.cncf.io",
		constants.KueueAPIGroupName,
		"prometheus.io",
		constants.LocalModelLabel,
	}
	leaderWorkerSetApprovedLabelPrefixes = []string{
		constants.KueueAPIGroupName,
		constants.LocalModelLabel,
	}
)

// propagateLeaderWorkerSetObjectAnnotations propagates approved top-level annotations
// to the LeaderWorkerSet object. The object's labels are the worker template's labels,
// which propagateLeaderWorkerTemplateMetadata already covers.
func propagateLeaderWorkerSetObjectAnnotations(llmSvc *v1alpha2.LLMInferenceService, expected *lwsapi.LeaderWorkerSet) {
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &expected.Annotations, leaderWorkerSetApprovedAnnotationPrefixes...)
}

// propagateLeaderWorkerTemplateMetadata propagates approved top-level annotations and
// labels to the leader and worker pod templates.
func propagateLeaderWorkerTemplateMetadata(llmSvc *v1alpha2.LLMInferenceService, group *lwsapi.LeaderWorkerTemplate) {
	if group.LeaderTemplate != nil {
		utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &group.LeaderTemplate.Annotations, leaderWorkerSetApprovedAnnotationPrefixes...)
	}
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &group.WorkerTemplate.Annotations, leaderWorkerSetApprovedAnnotationPrefixes...)

	if group.LeaderTemplate != nil {
		utils.PropagatePrefixedMap(llmSvc.GetLabels(), &group.LeaderTemplate.Labels, leaderWorkerSetApprovedLabelPrefixes...)
	}
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &group.WorkerTemplate.Labels, leaderWorkerSetApprovedLabelPrefixes...)
}

// deployedLeaderWorkerPodSpecs returns the leader (nil when there is none) and worker
// pod specs of the LeaderWorkerSet named key, or empty specs when it or its CRD does
// not exist.
func (r *LLMISVCReconciler) deployedLeaderWorkerPodSpecs(ctx context.Context, key types.NamespacedName) (*corev1.PodSpec, corev1.PodSpec, error) {
	curr := &lwsapi.LeaderWorkerSet{}
	if err := r.Get(ctx, key, curr); err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return nil, corev1.PodSpec{}, err
	}
	var leader *corev1.PodSpec
	if curr.Spec.LeaderWorkerTemplate.LeaderTemplate != nil {
		leader = &curr.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec
	}
	return leader, curr.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec, nil
}

func mainLWSName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-mn")
}

func prefillLWSName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-mn-prefill")
}

func rollingUpdateConfigFromWorkloadSpec(workload *v1alpha2.WorkloadSpec) *lwsapi.RollingUpdateConfiguration {
	if workload == nil || workload.RolloutStrategy == nil {
		return nil
	}
	rs := workload.RolloutStrategy
	if rs.MaxUnavailable == nil && rs.MaxSurge == nil {
		return nil
	}
	config := &lwsapi.RollingUpdateConfiguration{
		MaxUnavailable: intstr.FromInt32(1),
		MaxSurge:       intstr.FromInt32(0),
	}
	if rs.MaxUnavailable != nil {
		config.MaxUnavailable = *rs.MaxUnavailable
	}
	if rs.MaxSurge != nil {
		config.MaxSurge = *rs.MaxSurge
	}
	return config
}

func rollingUpdateConfigFromPrefill(prefill *v1alpha2.WorkloadSpec) *lwsapi.RollingUpdateConfiguration {
	return rollingUpdateConfigFromWorkloadSpec(prefill)
}

func semanticLWSIsEqual(expected *lwsapi.LeaderWorkerSet, curr *lwsapi.LeaderWorkerSet) bool {
	isLeaderEqual := (expected.Spec.LeaderWorkerTemplate.LeaderTemplate != nil) == (curr.Spec.LeaderWorkerTemplate.LeaderTemplate != nil)

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
