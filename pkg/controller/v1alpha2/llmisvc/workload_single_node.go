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
	"maps"
	"slices"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func (r *LLMISVCReconciler) reconcileSingleNodeWorkload(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, useDisaggregatedSet bool) error {
	log.FromContext(ctx).Info("Reconciling single-node workload")

	if err := r.reconcileManagedDRA(ctx, llmSvc); err != nil {
		return fmt.Errorf("failed to reconcile managed DRA: %w", err)
	}

	if err := r.reconcileSingleNodeMainServiceAccount(ctx, llmSvc, config); err != nil {
		return fmt.Errorf("failed to reconcile service account: %w", err)
	}

	if err := r.reconcileSingleNodeMainWorkload(ctx, llmSvc, config, useDisaggregatedSet); err != nil {
		return fmt.Errorf("failed to reconcile main workload: %w", err)
	}

	if err := r.reconcileSingleNodePrefill(ctx, llmSvc, config, useDisaggregatedSet); err != nil {
		return fmt.Errorf("failed to reconcile prefill workload: %w", err)
	}
	return nil
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainWorkload(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, useDisaggregatedSet bool) error {
	if isStopped := utils.GetForceStopRuntime(llmSvc); isStopped || llmSvc.Spec.Worker != nil || useDisaggregatedSet {
		if isStopped {
			llmSvc.MarkMainWorkloadNotReady("Stopped", "Service is stopped")
		} else {
			llmSvc.MarkMainWorkloadUnset()
		}
		return Delete(ctx, r, llmSvc, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mainDeploymentName(llmSvc),
				Namespace: llmSvc.GetNamespace(),
			},
		})
	}

	expected, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, config)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, expected, semanticDeploymentIsEqual, PreserveDeploymentReplicas(), PreserveDeploymentSelector()); err != nil {
		return err
	}
	return r.propagateWorkloadDeploymentStatus(ctx, expected, llmSvc.MarkMainWorkloadReady, llmSvc.MarkMainWorkloadNotReady)
}

func (r *LLMISVCReconciler) expectedSingleNodeMainDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) (*appsv1.Deployment, error) {
	key := types.NamespacedName{Name: mainDeploymentName(llmSvc), Namespace: llmSvc.GetNamespace()}

	var deployed corev1.PodSpec
	if llmSvc.Spec.Template != nil && !utils.GetForceStopRuntime(llmSvc) {
		var err error
		if deployed, err = r.deployedDeploymentPodSpec(ctx, key); err != nil {
			return nil, fmt.Errorf("failed to get current deployment %s/%s: %w", key.Namespace, key.Name, err)
		}
	}

	pod, err := r.expectedSingleNodeMainPodTemplate(ctx, llmSvc, config, deployed)
	if err != nil {
		return nil, err
	}

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: pod.IdentityLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: llmSvc.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: deploymentSelectorLabels(pod.IdentityLabels, llmSvc.Spec.Labels),
			},
			Template: pod.Template,
		},
	}

	applyDeploymentRolloutStrategy(d, &llmSvc.Spec.WorkloadSpec)
	propagateDeploymentObjectMetadata(llmSvc, d)

	log.FromContext(ctx).V(2).Info("Expected main deployment", "deployment", d)

	return d, nil
}

// singleNodePodTemplate is a rendered single-node pod template. IdentityLabels are
// the workload identity labels, before any propagated metadata, which a Deployment
// uses for its own labels and its selector.
type singleNodePodTemplate struct {
	Template       corev1.PodTemplateSpec
	IdentityLabels map[string]string
}

// expectedSingleNodeMainPodTemplate renders the decode pod template of a single-node
// workload. deployed is the pod spec currently deployed for decode, used to keep
// storage-initializer settings stable across controller upgrades.
func (r *LLMISVCReconciler) expectedSingleNodeMainPodTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed corev1.PodSpec) (*singleNodePodTemplate, error) {
	role := constants.LLMDRoleDecode
	if llmSvc.Spec.Prefill == nil {
		role = constants.LLMDRoleBoth
	}

	labels := r.singleNodeLabels(llmSvc)
	labels[constants.KServeComponentLabelKey] = constants.KServeComponentWorkload
	labels[constants.LLMDRoleLabelKey] = role

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

	err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
	}

	template := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      maps.Clone(labels),
			Annotations: podAnnotations,
		},
	}

	if llmSvc.Spec.Template != nil && !utils.GetForceStopRuntime(llmSvc) {
		template.Spec = *llmSvc.Spec.Template.DeepCopy()

		var serviceAccount *corev1.ServiceAccount = nil
		if hasRoutingSidecar(template.Spec) || config.ModelExpress != nil {
			var err error
			serviceAccount, _, err = r.expectedSingleNodeMainServiceAccount(ctx, llmSvc)
			if err != nil {
				return nil, fmt.Errorf("failed to created expected single node service account: %w", err)
			}
			template.Spec.ServiceAccountName = serviceAccount.GetName()
			if hasRoutingSidecar(template.Spec) {
				log.FromContext(ctx).Info("Main container has a routing sidecar")
				s := routingSidecar(&template.Spec)
				if llmSvc.Spec.Router != nil {
					s.Env = append(s.Env, corev1.EnvVar{
						Name:      "INFERENCE_POOL_NAME",
						Value:     llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc),
						ValueFrom: nil,
					})
				}
			}
		} else if llmSvc.Spec.Template.ServiceAccountName != "" {
			serviceAccount = &corev1.ServiceAccount{}
			err := r.Get(ctx, types.NamespacedName{Name: llmSvc.Spec.Template.ServiceAccountName, Namespace: llmSvc.Namespace}, serviceAccount)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch existing single node main service account %s/%s: %w", llmSvc.Namespace, llmSvc.Spec.Template.ServiceAccountName, err)
			}
		}

		if err := r.attachModelArtifacts(ctx, serviceAccount, llmSvc, deployed, &template.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to main pod template: %w", err)
		}
		if llmSvc.Spec.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&template.Spec, llmSvc.Spec.KVCacheOffloading.Secondary, "main")
		}
		if err := attachModelExpress(llmSvc, &template.Spec, config.ModelExpress); err != nil {
			return nil, fmt.Errorf("failed to attach ModelExpress: %w", err)
		}
	}

	propagatePodTemplateMetadata(llmSvc, &template)

	utils.PropagateMap(llmSvc.Spec.Labels, &template.Labels)
	utils.PropagateMap(llmSvc.Spec.Annotations, &template.Annotations, routingSpecAnnotations...)

	// Inject tracing instrumentation when spec.tracing is set
	if llmSvc.Spec.Tracing != nil {
		mainIdx := slices.IndexFunc(template.Spec.Containers, func(c corev1.Container) bool {
			return c.Name == "main"
		})
		if mainIdx >= 0 {
			injectServerTracing(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-decode", &template.Spec.Containers[mainIdx])
		}
	}

	applyWorkloadRevision(&template, config)

	return &singleNodePodTemplate{Template: template, IdentityLabels: labels}, nil
}

func (r *LLMISVCReconciler) reconcileSingleNodePrefill(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, useDisaggregatedSet bool) error {
	if isStopped := utils.GetForceStopRuntime(llmSvc); isStopped || llmSvc.Spec.Prefill == nil || llmSvc.Spec.Prefill.Worker != nil || useDisaggregatedSet {
		if isStopped {
			llmSvc.MarkPrefillWorkloadNotReady("Stopped", "Service is stopped")
		} else {
			llmSvc.MarkPrefillWorkloadUnset()
		}
		return Delete(ctx, r, llmSvc, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefillDeploymentName(llmSvc),
				Namespace: llmSvc.GetNamespace(),
			},
		})
	}

	prefill, err := r.expectedPrefillMainDeployment(ctx, llmSvc, config)
	if err != nil {
		return fmt.Errorf("failed to get expected prefill deployment: %w", err)
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, prefill, semanticDeploymentIsEqual, PreserveDeploymentReplicas(), PreserveDeploymentSelector()); err != nil {
		return fmt.Errorf("failed to reconcile prefill deployment %s/%s: %w", prefill.GetNamespace(), prefill.GetName(), err)
	}
	return r.propagateWorkloadDeploymentStatus(ctx, prefill, llmSvc.MarkPrefillWorkloadReady, llmSvc.MarkPrefillWorkloadNotReady)
}

func (r *LLMISVCReconciler) expectedPrefillMainDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) (*appsv1.Deployment, error) {
	key := types.NamespacedName{Name: prefillDeploymentName(llmSvc), Namespace: llmSvc.GetNamespace()}

	var deployed corev1.PodSpec
	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Template != nil && !utils.GetForceStopRuntime(llmSvc) {
		var err error
		if deployed, err = r.deployedDeploymentPodSpec(ctx, key); err != nil {
			return nil, fmt.Errorf("failed to get current prefill deployment %s/%s: %w", key.Namespace, key.Name, err)
		}
	}

	pod, err := r.expectedSingleNodePrefillPodTemplate(ctx, llmSvc, config, deployed)
	if err != nil {
		return nil, err
	}

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: pod.IdentityLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Template: pod.Template,
		},
	}

	if llmSvc.Spec.Prefill != nil {
		d.Spec.Replicas = llmSvc.Spec.Prefill.Replicas
		d.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: deploymentSelectorLabels(pod.IdentityLabels, llmSvc.Spec.Prefill.Labels),
		}
		applyDeploymentRolloutStrategy(d, llmSvc.Spec.Prefill)
	}

	propagateDeploymentObjectMetadata(llmSvc, d)

	log.FromContext(ctx).V(2).Info("Expected prefill deployment", "deployment", d)

	return d, nil
}

// expectedSingleNodePrefillPodTemplate renders the prefill pod template of a
// single-node workload. deployed is the pod spec currently deployed for prefill, used
// to keep storage-initializer settings stable across controller upgrades.
func (r *LLMISVCReconciler) expectedSingleNodePrefillPodTemplate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config, deployed corev1.PodSpec) (*singleNodePodTemplate, error) {
	labels := map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkloadPrefill,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
		constants.KServeComponentLabelKey:     constants.KServeComponentWorkload,
		constants.LLMDRoleLabelKey:            constants.LLMDRolePrefill,
	}

	var template corev1.PodTemplateSpec
	if llmSvc.Spec.Prefill != nil {
		err := r.propagateInferencePoolRefLabelSelector(ctx, llmSvc, labels)
		if err != nil {
			return nil, fmt.Errorf("failed to propagate InferencePool reference labels: %w", err)
		}

		template.Labels = maps.Clone(labels)
	}

	if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.Template != nil && !utils.GetForceStopRuntime(llmSvc) {
		template.Spec = *llmSvc.Spec.Prefill.Template.DeepCopy()

		var existingServiceAccount *corev1.ServiceAccount = nil
		if llmSvc.Spec.Prefill.Template.ServiceAccountName != "" {
			existingServiceAccount = &corev1.ServiceAccount{}
			err := r.Get(ctx, types.NamespacedName{Name: llmSvc.Spec.Prefill.Template.ServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch existing single node prefill service account %s/%s: %w", llmSvc.Namespace, llmSvc.Spec.Prefill.Template.ServiceAccountName, err)
			}
		} else if config.ModelExpress != nil {
			var err error
			existingServiceAccount, _, err = r.expectedSingleNodeMainServiceAccount(ctx, llmSvc)
			if err != nil {
				return nil, fmt.Errorf("failed to created expected single node service account: %w", err)
			}
			template.Spec.ServiceAccountName = existingServiceAccount.GetName()
		}

		if err := r.attachModelArtifacts(ctx, existingServiceAccount, llmSvc, deployed, &template.Spec, config, "main", constants.DefaultModelLocalMountPath, len(config.ResolvedLoRAAdapters) > 0); err != nil {
			return nil, fmt.Errorf("failed to attach model artifacts to prefill pod template: %w", err)
		}
		if llmSvc.Spec.Prefill != nil && llmSvc.Spec.Prefill.KVCacheOffloading != nil {
			attachKVCacheSecondaryTiers(&template.Spec, llmSvc.Spec.Prefill.KVCacheOffloading.Secondary, "main")
		}
		if err := attachModelExpress(llmSvc, &template.Spec, config.ModelExpress); err != nil {
			return nil, fmt.Errorf("failed to attach ModelExpress: %w", err)
		}
	}

	propagatePodTemplateMetadata(llmSvc, &template)

	if llmSvc.Spec.Prefill != nil {
		utils.PropagateMap(llmSvc.Spec.Prefill.Labels, &template.Labels)
		utils.PropagateMap(llmSvc.Spec.Prefill.Annotations, &template.Annotations, routingSpecAnnotations...)
	}

	// Inject tracing instrumentation when spec.tracing is set
	if llmSvc.Spec.Tracing != nil {
		mainIdx := slices.IndexFunc(template.Spec.Containers, func(c corev1.Container) bool {
			return c.Name == "main"
		})
		if mainIdx >= 0 {
			injectServerTracing(llmSvc.Spec.Tracing, llmSvc.GetNamespace(), llmSvc.GetName(), "-prefill", &template.Spec.Containers[mainIdx])
		}
	}

	applyWorkloadRevision(&template, config)

	return &singleNodePodTemplate{Template: template, IdentityLabels: labels}, nil
}

// Top-level metadata keys propagated to Deployments and their pod templates.
var (
	deploymentApprovedAnnotationPrefixes = []string{
		"k8s.v1.cni.cncf.io",
		constants.KueueAPIGroupName,
		"prometheus.io",
		constants.LocalModelLabel,
	}
	deploymentApprovedLabelPrefixes = []string{
		constants.KueueAPIGroupName,
		constants.LocalModelLabel,
	}
)

func (r *LLMISVCReconciler) propagateDeploymentMetadata(llmSvc *v1alpha2.LLMInferenceService, expected *appsv1.Deployment) {
	propagateDeploymentObjectMetadata(llmSvc, expected)
	propagatePodTemplateMetadata(llmSvc, &expected.Spec.Template)
}

// propagateDeploymentObjectMetadata propagates approved top-level annotations and
// labels to the Deployment object.
func propagateDeploymentObjectMetadata(llmSvc *v1alpha2.LLMInferenceService, expected *appsv1.Deployment) {
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &expected.Annotations, deploymentApprovedAnnotationPrefixes...)
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &expected.Labels, deploymentApprovedLabelPrefixes...)
}

// propagatePodTemplateMetadata propagates approved top-level annotations and labels to
// a single-node pod template.
func propagatePodTemplateMetadata(llmSvc *v1alpha2.LLMInferenceService, template *corev1.PodTemplateSpec) {
	utils.PropagatePrefixedMap(llmSvc.GetAnnotations(), &template.Annotations, deploymentApprovedAnnotationPrefixes...)
	utils.PropagatePrefixedMap(llmSvc.GetLabels(), &template.Labels, deploymentApprovedLabelPrefixes...)
}

// deployedDeploymentPodSpec returns the pod spec of the Deployment named key, or an
// empty spec when it does not exist.
func (r *LLMISVCReconciler) deployedDeploymentPodSpec(ctx context.Context, key types.NamespacedName) (corev1.PodSpec, error) {
	curr := &appsv1.Deployment{}
	if err := r.Get(ctx, key, curr); err != nil && !apierrors.IsNotFound(err) {
		return corev1.PodSpec{}, err
	}
	return curr.Spec.Template.Spec, nil
}

func (r *LLMISVCReconciler) propagateWorkloadDeploymentStatus(ctx context.Context, expected *appsv1.Deployment, ready func(), notReady func(reason, messageFormat string, messageA ...interface{})) error {
	curr := &appsv1.Deployment{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Get(ctx, client.ObjectKeyFromObject(expected), curr)
	})
	if err != nil {
		return fmt.Errorf("failed to get current deployment %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	for _, cond := range curr.Status.Conditions {
		if cond.Type == appsv1.DeploymentProgressing {
			if cond.Status == corev1.ConditionFalse {
				notReady(cond.Reason, cond.Message)
			}
		}

		if cond.Type == appsv1.DeploymentAvailable {
			if cond.Status == corev1.ConditionTrue {
				ready()
			} else {
				notReady(cond.Reason, cond.Message)
			}
			return nil
		}
	}
	notReady(string(appsv1.DeploymentProgressing), "")
	return nil
}

func semanticDeploymentIsEqual(expected *appsv1.Deployment, curr *appsv1.Deployment) bool {
	// Use DeepEqual for the Pod Spec so that when fields are removed (like resource requirements, we push them down to the
	// child resource)
	return equality.Semantic.DeepEqual(expected.Spec.Template.Spec, curr.Spec.Template.Spec) &&
		equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, config)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	serviceAccount, useExistingServiceAccount, err := r.expectedSingleNodeMainServiceAccount(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to created expected single node service account: %w", err)
	}

	if !useExistingServiceAccount {
		if utils.GetForceStopRuntime(llmSvc) || (!hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) && config.ModelExpress == nil) {
			return Delete(ctx, r, llmSvc, serviceAccount)
		}

		if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile single node service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
		}
	}

	if err := r.reconcileSingleNodeMainRole(ctx, llmSvc, config); err != nil {
		return err
	}

	return r.reconcileSingleNodeMainRoleBinding(ctx, llmSvc, serviceAccount, config)
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainRole(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, config)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	role := r.expectedSingleNodeRole(llmSvc)
	if utils.GetForceStopRuntime(llmSvc) || !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, role)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.Role{}, role, semanticRoleIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node role %s/%s: %w", role.GetNamespace(), role.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSingleNodeMainRoleBinding(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount, config *Config) error {
	expectedDeployment, err := r.expectedSingleNodeMainDeployment(ctx, llmSvc, config)
	if err != nil {
		return fmt.Errorf("failed to get expected main deployment: %w", err)
	}

	roleBinding := r.expectedSingleNodeRoleBinding(llmSvc, sa)
	if utils.GetForceStopRuntime(llmSvc) || !hasRoutingSidecar(expectedDeployment.Spec.Template.Spec) {
		return Delete(ctx, r, llmSvc, roleBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.RoleBinding{}, roleBinding, semanticRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile single node rolebinding %s/%s: %w", roleBinding.GetNamespace(), roleBinding.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) expectedSingleNodeMainServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*corev1.ServiceAccount, bool, error) {
	useExistingServiceAccount := false
	expectedServiceAccountName := kmeta.ChildName(llmSvc.GetName(), "-kserve")

	var existingServiceAccountName string
	if llmSvc.Spec.Template != nil && llmSvc.Spec.Template.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Template.ServiceAccountName
	}

	if existingServiceAccountName != "" && existingServiceAccountName != expectedServiceAccountName {
		useExistingServiceAccount = true
		log.FromContext(ctx).V(2).Info("Using existing service account for single node main workload", "serviceAccountName", existingServiceAccountName)
		existingServiceAccount := &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{Name: existingServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
		if err != nil {
			return nil, useExistingServiceAccount, fmt.Errorf("failed to fetch existing single node main service account %s/%s: %w", llmSvc.Namespace, existingServiceAccountName, err)
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

	if expectedServiceAccount.Labels == nil {
		expectedServiceAccount.Labels = make(map[string]string)
	}
	maps.Copy(expectedServiceAccount.Labels, r.singleNodeLabels(llmSvc))

	return expectedServiceAccount, useExistingServiceAccount, nil
}

func (r *LLMISVCReconciler) expectedSingleNodeRole(llmSvc *v1alpha2.LLMInferenceService) *rbacv1.Role {
	ro := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-role"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: r.singleNodeLabels(llmSvc),
		},
	}
	ro.Rules = append(ro.Rules, sidecarSSRFProtectionRules...)
	return ro
}

func (r *LLMISVCReconciler) expectedSingleNodeRoleBinding(llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-rb"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
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

func (r *LLMISVCReconciler) singleNodeLabels(llmSvc *v1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
}

func mainDeploymentName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve")
}

func prefillDeploymentName(llmSvc *v1alpha2.LLMInferenceService) string {
	return kmeta.ChildName(llmSvc.GetName(), "-kserve-prefill")
}

func applyDeploymentRolloutStrategy(d *appsv1.Deployment, workload *v1alpha2.WorkloadSpec) {
	if workload == nil || workload.RolloutStrategy == nil {
		return
	}
	rs := workload.RolloutStrategy
	if rs.MaxUnavailable == nil && rs.MaxSurge == nil {
		return
	}
	d.Spec.Strategy = appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: rs.MaxUnavailable,
			MaxSurge:       rs.MaxSurge,
		},
	}
}
