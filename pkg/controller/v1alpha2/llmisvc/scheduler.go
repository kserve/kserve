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
	"path"
	"slices"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	tokenizerContainerName = "tokenizer"

	udsTokenizerBaseModelName = "base"
	udsTokenizerSocketFile    = "/tmp/tokenizer/tokenizer-uds.socket" //nolint:gosec // G101: not a credential, UDS socket path
)

// reconcileScheduler manages the scheduler component and its related resources
// The scheduler handles load balancing for inference pods
func (r *LLMISVCReconciler) reconcileScheduler(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, schedulerConfig *SchedulerConfig) error {
	log.FromContext(ctx).Info("Reconciling Scheduler")

	if err := r.reconcileSchedulerServiceAccount(ctx, llmSvc); err != nil {
		return err
	}
	if err := r.reconcileSchedulerDeployment(ctx, llmSvc, schedulerConfig); err != nil {
		return err
	}
	if err := r.reconcileSchedulerService(ctx, llmSvc); err != nil {
		return err
	}
	if err := r.reconcileSchedulerInferencePool(ctx, llmSvc); err != nil {
		return err
	}

	if utils.GetForceStopRuntime(llmSvc) {
		llmSvc.MarkInferencePoolNotReady("Stopped", "Service is stopped")
	}

	return nil
}

// reconcileSchedulerAuthDelegatorBinding manages RBAC for authentication delegation
// This allows the scheduler to authenticate requests to `/metrics`
func (r *LLMISVCReconciler) reconcileSchedulerAuthDelegatorBinding(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount) error {
	authDelegatorBinding := r.expectedSchedulerAuthDelegatorBinding(llmSvc, sa)
	// Clean up binding if scheduler is not configured or uses external pool
	if utils.GetForceStopRuntime(llmSvc) || !llmSvc.DeletionTimestamp.IsZero() || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		return Delete(ctx, r, llmSvc, authDelegatorBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.ClusterRoleBinding{}, authDelegatorBinding, semanticClusterRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler clusterrolebinding %s: %w", authDelegatorBinding.GetName(), err)
	}

	return nil
}

// reconcileSchedulerRole manages the RBAC role for scheduler permissions
// The scheduler needs permissions to manage inference pools and related resources
func (r *LLMISVCReconciler) reconcileSchedulerRole(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	role := r.expectedSchedulerRole(llmSvc)
	// Clean up role if scheduler is not configured or uses external pool
	if utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		return Delete(ctx, r, llmSvc, role)
	}
	if err := Reconcile(ctx, r, llmSvc, &rbacv1.Role{}, role, semanticRoleIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler role %s/%s: %w", role.GetNamespace(), role.GetName(), err)
	}

	return nil
}

// reconcileSchedulerRoleBinding binds the scheduler role to its service account
// This grants the scheduler the necessary permissions to operate
func (r *LLMISVCReconciler) reconcileSchedulerRoleBinding(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount) error {
	roleBinding := r.expectedSchedulerRoleBinding(llmSvc, sa)
	// Clean up binding if scheduler is not configured or uses external pool
	if utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		return Delete(ctx, r, llmSvc, roleBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.RoleBinding{}, roleBinding, semanticRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler rolebinding %s/%s: %w", roleBinding.GetNamespace(), roleBinding.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSchedulerServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	serviceAccount, useExistingServiceAccount, err := r.expectedSchedulerServiceAccount(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to create expected scheduler service account: %w", err)
	}

	if !llmSvc.DeletionTimestamp.IsZero() {
		return r.reconcileSchedulerAuthDelegatorBinding(ctx, llmSvc, serviceAccount)
	}

	if !useExistingServiceAccount {
		if utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
			return Delete(ctx, r, llmSvc, serviceAccount)
		}

		if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
			return fmt.Errorf("failed to reconcile scheduler service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
		}
	}
	if err := r.reconcileSchedulerAuthDelegatorBinding(ctx, llmSvc, serviceAccount); err != nil {
		return err
	}

	if err := r.reconcileSchedulerRole(ctx, llmSvc); err != nil {
		return err
	}

	return r.reconcileSchedulerRoleBinding(ctx, llmSvc, serviceAccount)
}

func (r *LLMISVCReconciler) reconcileSchedulerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, schedulerConfig *SchedulerConfig) error {
	scheduler, err := r.expectedSchedulerDeployment(ctx, llmSvc, schedulerConfig)
	if err != nil {
		return fmt.Errorf("failed to build expected scheduler deployment: %w", err)
	}
	if isStopped := utils.GetForceStopRuntime(llmSvc); isStopped || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		if isStopped {
			llmSvc.MarkSchedulerWorkloadNotReady("Stopped", "Service is stopped")
		} else {
			llmSvc.MarkSchedulerWorkloadUnset()
		}
		return Delete(ctx, r, llmSvc, scheduler)
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, scheduler, semanticDeploymentIsEqual, PreserveDeploymentReplicas()); err != nil {
		return fmt.Errorf("failed to reconcile scheduler deployment %s/%s: %w", scheduler.GetNamespace(), scheduler.GetName(), err)
	}
	return r.propagateSchedulerDeploymentStatus(ctx, scheduler, llmSvc.MarkSchedulerWorkloadReady, llmSvc.MarkSchedulerWorkloadNotReady)
}

func (r *LLMISVCReconciler) reconcileSchedulerInferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	shouldDelete := utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef()

	if err := r.reconcileV1InferencePool(ctx, llmSvc, shouldDelete); err != nil {
		return err
	}

	if err := r.reconcileV1Alpha2InferencePool(ctx, llmSvc, shouldDelete); err != nil {
		return err
	}

	return nil
}

// reconcileV1InferencePool reconciles the v1 InferencePool if the CRD is available.
func (r *LLMISVCReconciler) reconcileV1InferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shouldDelete bool) error {
	expected := r.expectedSchedulerInferencePool(ctx, llmSvc)
	if shouldDelete {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &igwapi.InferencePool{}, expected, semanticInferencePoolIsEqual)
}

// reconcileV1Alpha2InferencePool reconciles the v1alpha2 InferencePool if the CRD is available.
func (r *LLMISVCReconciler) reconcileV1Alpha2InferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shouldDelete bool) error {
	expected := r.expectedSchedulerInferencePoolV1Alpha2(ctx, llmSvc)
	if shouldDelete {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &igwapiv1alpha2.InferencePool{}, expected, semanticInferencePoolV1Alpha2IsEqual)
}

func (r *LLMISVCReconciler) reconcileSchedulerService(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	expected := r.expectedSchedulerService(ctx, llmSvc)
	if utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		return Delete(ctx, r, llmSvc, expected)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.Service{}, expected, semanticServiceIsEqual); err != nil {
		return err
	}

	return nil
}

func (r *LLMISVCReconciler) expectedSchedulerService(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *corev1.Service {
	logger := log.FromContext(ctx)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Spec.Router.EPPServiceName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
			Labels:    SchedulerLabels(llmSvc),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: SchedulerLabels(llmSvc),
		},
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Template != nil {
		podSpec := llmSvc.Spec.Router.Scheduler.Template.DeepCopy()

		desiredPorts := sets.New("grpc", "grpc-health", "metrics", "zmq")

		actualPorts := make(map[string]*corev1.ContainerPort)
		for _, container := range podSpec.Containers {
			for _, port := range container.Ports {
				if desiredPorts.Has(port.Name) {
					actualPorts[port.Name] = port.DeepCopy()
				}
			}
		}

		if len(desiredPorts) != len(actualPorts) {
			// TODO should this be raised as failing condition? + check if grpc port matches what's defined in the inferencepool
			logger.Info("some ports are not matching", "desired", desiredPorts, "actual", maps.Keys(actualPorts))
		}

		servicePorts := make([]corev1.ServicePort, 0, len(actualPorts))
		for name, port := range actualPorts {
			servicePorts = append(servicePorts, corev1.ServicePort{
				Name:       name,
				Port:       port.ContainerPort,
				TargetPort: intstr.FromString(name),
				Protocol:   port.Protocol,
			})
		}

		sort.Slice(servicePorts, func(i, j int) bool {
			return servicePorts[i].Name < servicePorts[j].Name
		})

		svc.Spec.Ports = servicePorts
	}

	log.FromContext(ctx).V(2).Info("Expected router EPP service", "service", svc)

	return svc
}

func (r *LLMISVCReconciler) expectedSchedulerInferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *igwapi.InferencePool {
	labels := SchedulerLabels(llmSvc)

	ip := &igwapi.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-inference-pool"),
			Namespace: llmSvc.GetNamespace(),
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
		},
	}
	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Pool != nil && llmSvc.Spec.Router.Scheduler.Pool.Spec != nil {
		ip.Spec = *llmSvc.Spec.Router.Scheduler.Pool.Spec.DeepCopy()
	}

	log.FromContext(ctx).V(2).Info("Expected router InferencePool", "inferencepool", ip)

	return ip
}

func (r *LLMISVCReconciler) expectedSchedulerInferencePoolV1Alpha2(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *igwapiv1alpha2.InferencePool {
	labels := SchedulerLabels(llmSvc)

	// Define the desired ObjectMeta first
	// Use the same name as v1 pool - they can coexist because they're different CRDs (different API groups)
	desiredMeta := metav1.ObjectMeta{
		Name:      kmeta.ChildName(llmSvc.GetName(), "-inference-pool"),
		Namespace: llmSvc.GetNamespace(),
		Labels:    labels,
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
		},
	}

	ip := &igwapiv1alpha2.InferencePool{
		ObjectMeta: desiredMeta,
	}

	// Convert v1 spec to v1alpha2 using the built-in GIE conversion
	// Note: ConvertFrom overwrites ObjectMeta, so we must restore it after conversion
	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Pool != nil && llmSvc.Spec.Router.Scheduler.Pool.Spec != nil {
		srcPool := &igwapi.InferencePool{Spec: *llmSvc.Spec.Router.Scheduler.Pool.Spec.DeepCopy()}
		if err := ip.ConvertFrom(srcPool); err != nil {
			log.FromContext(ctx).Error(err, "Failed to convert InferencePool spec to v1alpha2")
		}
		// Restore the desired ObjectMeta after conversion (ConvertFrom overwrites it)
		ip.ObjectMeta = desiredMeta
	}

	log.FromContext(ctx).V(2).Info("Expected router InferencePool v1alpha2", "inferencepool", ip)

	return ip
}

func (r *LLMISVCReconciler) expectedSchedulerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, schedulerConfig *SchedulerConfig) (*appsv1.Deployment, error) {
	labels := SchedulerLabels(llmSvc)
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-router-scheduler"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Strategy: appsv1.DeploymentStrategy{
				// The current recommended EPP deployment pattern is to have a single active replica. This ensures
				// optimal performance of the stateful operations such prefix cache aware scorer.
				// The `Recreate` strategy ensures the old replica is killed immediately, and allow the new replica(s) to
				// quickly take over. This is particularly important in the high availability set up with leader
				// election, as the rolling update strategy would prevent the old leader being killed because
				// otherwise the maxUnavailable would be 100%.
				Type: appsv1.RecreateDeploymentStrategyType,
			},
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

	mainIdx := -1
	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Template != nil {
		curr := &appsv1.Deployment{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(d), curr); err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get current scheduler deployment %s/%s: %w", d.GetNamespace(), d.GetName(), err)
		}

		d.Spec.Replicas = llmSvc.Spec.Router.Scheduler.Replicas
		d.Spec.Template.Spec = *llmSvc.Spec.Router.Scheduler.Template.DeepCopy()

		mainIdx = slices.IndexFunc(d.Spec.Template.Spec.Containers, func(c corev1.Container) bool {
			return c.Name == "main"
		})
		if mainIdx < 0 {
			log.FromContext(ctx).Info("Scheduler template does not have a container named \"main\", skipping arg injection")
		}

		if mainIdx >= 0 {
			mainContainer := &d.Spec.Template.Spec.Containers[mainIdx]

			if d.Spec.Replicas != nil && *d.Spec.Replicas > 1 &&
				!slices.Contains(mainContainer.Args, "--ha-enable-leader-election") &&
				!slices.Contains(mainContainer.Args, "-ha-enable-leader-election") {
				mainContainer.Args = append(mainContainer.Args,
					"--ha-enable-leader-election",
				)
			}

			mainContainer.Args = append(mainContainer.Args,
				preserveSchedulerConfig(llmSvc, curr)...,
			)
		}

		if isUsingTokenizerSidecar(llmSvc.Spec) {
			var existingServiceAccount *corev1.ServiceAccount
			if llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName != "" {
				existingServiceAccount = &corev1.ServiceAccount{}
				err := r.Get(ctx, types.NamespacedName{Name: llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
				if err != nil {
					if !apierrors.IsNotFound(err) {
						return d, fmt.Errorf("failed to fetch existing scheduler service account %s/%s: %w", llmSvc.Namespace, llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName, err)
					}
					// The service account may not exist yet (first reconciliation, cache lag)
					// or may have already been deleted (stop flow). Let attachModelArtifacts
					// handle credential injection with the default service account fallback.
					existingServiceAccount = nil
				}
			} else {
				// Use the generated scheduler SA which has credentials propagated from the main workload SA.
				sa, _, saErr := r.expectedSchedulerServiceAccount(ctx, llmSvc)
				if saErr != nil {
					return d, fmt.Errorf("failed to get expected scheduler service account: %w", saErr)
				}
				existingServiceAccount = sa
			}

			curr := &appsv1.Deployment{}
			if err := r.Get(ctx, client.ObjectKeyFromObject(d), curr); err != nil && !apierrors.IsNotFound(err) {
				return d, fmt.Errorf("failed to get current scheduler deployment %s/%s: %w", d.GetNamespace(), d.GetName(), err)
			}

			config, err := LoadConfig(ctx, r.Clientset)
			if err != nil {
				return d, fmt.Errorf("failed to load config for scheduler deployment %s/%s: %w", d.GetNamespace(), d.GetName(), err)
			}

			modelPath := path.Join(constants.DefaultModelLocalMountPath, "base")

			if err := r.attachModelArtifacts(ctx, existingServiceAccount, llmSvc, curr.Spec.Template.Spec, &d.Spec.Template.Spec, config, tokenizerContainerName, modelPath); err != nil {
				return d, fmt.Errorf("failed to attach model artifacts to scheduler deployment: %w", err)
			}

			if err := mutateSchedulerConfig(d, WithUdsTokenizerConfig); err != nil {
				return d, fmt.Errorf("failed to mutate scheduler config for tokenizer: %w", err)
			}
		}
	}

	r.propagateSchedulerMetadata(llmSvc, d)

	// Set a hash of the current certificate data on the pod template so that
	// when certificates are renewed the pod template changes and the scheduler
	// is restarted to pick up the new certificate.
	// Skip if the main container supports automatic cert reload.
	if mainIdx >= 0 && !slices.Contains(d.Spec.Template.Spec.Containers[mainIdx].Args, "--enable-cert-reload") {
		if h := r.getSelfSignedCertHash(ctx, llmSvc); h != "" {
			if d.Spec.Template.Annotations == nil {
				d.Spec.Template.Annotations = map[string]string{}
			}
			d.Spec.Template.Annotations[schedulerConfig.RestartAnnotation] = h
		}
	}

	log.FromContext(ctx).V(2).Info("Expected router scheduler deployment", "deployment", d)

	return d, nil
}

func (r *LLMISVCReconciler) propagateSchedulerMetadata(llmSvc *v1alpha2.LLMInferenceService, expected *appsv1.Deployment) {
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return
	}
	utils.PropagateMap(llmSvc.Spec.Router.Scheduler.Labels, &expected.Spec.Template.Labels)
	utils.PropagateMap(llmSvc.Spec.Router.Scheduler.Annotations, &expected.Spec.Template.Annotations)
}

func schedulerConfigText(llmSvc *v1alpha2.LLMInferenceService) string {
	if llmSvc.Spec.Router != nil &&
		llmSvc.Spec.Router.Scheduler != nil &&
		llmSvc.Spec.Router.Scheduler.Config != nil &&
		llmSvc.Spec.Router.Scheduler.Config.Inline != nil {
		// We don't need to handle Ref as it's done as part of the config merge step.
		return string(llmSvc.Spec.Router.Scheduler.Config.Inline.Raw)
	}

	switch {
	case llmSvc.Spec.Prefill != nil:
		// Always do P/D by default (threshold 0)
		return `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: prefill-header-handler
- type: prefill-filter
- type: decode-filter
- type: queue-scorer
- type: prefix-cache-scorer
- type: max-score-picker
- type: always-disagg-pd-decider
- type: pd-profile-handler
  parameters:
    deciderPluginName: always-disagg-pd-decider
schedulingProfiles:
- name: prefill
  plugins:
  - pluginRef: prefill-filter
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
- name: decode
  plugins:
  - pluginRef: decode-filter
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`
	default:
		return `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: queue-scorer
- type: prefix-cache-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`
	}
}

// schedulerConfigFlags lists both kebab-case and camelCase variants because
// Go's flag package accepts either form.
var schedulerConfigFlags = map[string]struct{}{
	"--config-text": {}, "-config-text": {}, "--configText": {}, "-configText": {},
	"--config-file": {}, "-config-file": {}, "--configFile": {}, "-configFile": {},
}

// preserveSchedulerConfig returns the config args for the scheduler container.
//
// Priority:
//  1. Explicit inline config (including resolved ConfigMap refs) - always wins.
//  2. Config flag already present in the template args - kept as-is (return nil).
//  3. Config flag found in the current deployment - preserved across upgrades.
//  4. No config anywhere - a fresh default is generated.
func preserveSchedulerConfig(llmSvc *v1alpha2.LLMInferenceService, curr *appsv1.Deployment) []string {
	if llmSvc.Spec.Router != nil &&
		llmSvc.Spec.Router.Scheduler != nil &&
		llmSvc.Spec.Router.Scheduler.Config != nil &&
		llmSvc.Spec.Router.Scheduler.Config.Inline != nil {
		return []string{"--config-text", string(llmSvc.Spec.Router.Scheduler.Config.Inline.Raw)}
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Template != nil {
		if configFlagFromContainers(llmSvc.Spec.Router.Scheduler.Template.Containers) != nil {
			return nil
		}
	}

	if pair := configFlagFromContainers(curr.Spec.Template.Spec.Containers); pair != nil {
		return pair
	}

	return []string{"--config-text", schedulerConfigText(llmSvc)}
}

// configFlagFromContainers scans the "main" container for a config flag and
// returns {flag, value} if found, nil otherwise.
func configFlagFromContainers(containers []corev1.Container) []string {
	for i := range containers {
		c := &containers[i]
		if c.Name != "main" {
			continue
		}
		for j := 0; j+1 < len(c.Args); j++ {
			if _, ok := schedulerConfigFlags[c.Args[j]]; ok {
				return []string{c.Args[j], c.Args[j+1]}
			}
		}
		break // done with main
	}
	return nil
}

func (r *LLMISVCReconciler) expectedSchedulerServiceAccount(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (*corev1.ServiceAccount, bool, error) {
	useExistingServiceAccount := false
	expectedServiceAccountName := kmeta.ChildName(llmSvc.GetName(), "-epp-sa")

	var existingServiceAccountName string
	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Template != nil && llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName != "" {
		existingServiceAccountName = llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName
	}

	if existingServiceAccountName != "" && existingServiceAccountName != expectedServiceAccountName {
		useExistingServiceAccount = true
		log.FromContext(ctx).V(2).Info("Using existing service account for scheduler", "serviceAccountName", existingServiceAccountName)
		existingServiceAccount := &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{Name: existingServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
		if err != nil {
			return nil, true, fmt.Errorf("failed to fetch existing scheduler service account %s/%s: %w", llmSvc.Namespace, existingServiceAccountName, err)
		}
		return existingServiceAccount, useExistingServiceAccount, nil
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      expectedServiceAccountName,
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: SchedulerLabels(llmSvc),
		},
	}

	// Propagate credential-related fields from the main workload's service account
	// so the tokenizer sidecar can download the model using the same credentials.
	if llmSvc.Spec.Template != nil && llmSvc.Spec.Template.ServiceAccountName != "" {
		mainSA := &corev1.ServiceAccount{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      llmSvc.Spec.Template.ServiceAccountName,
			Namespace: llmSvc.GetNamespace(),
		}, mainSA)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, false, fmt.Errorf("failed to fetch main workload service account %s/%s: %w",
					llmSvc.GetNamespace(), llmSvc.Spec.Template.ServiceAccountName, err)
			}
			// SA may not exist yet on first reconciliation; next reconcile will pick it up.
			log.FromContext(ctx).V(2).Info("Main workload service account not found, skipping credential propagation",
				"serviceAccountName", llmSvc.Spec.Template.ServiceAccountName)
		} else {
			if mainSA.Annotations != nil {
				if sa.Annotations == nil {
					sa.Annotations = make(map[string]string)
				}
				maps.Copy(sa.Annotations, mainSA.Annotations)
			}
			sa.Secrets = mainSA.Secrets
			sa.ImagePullSecrets = mainSA.ImagePullSecrets
		}
	}

	return sa, useExistingServiceAccount, nil
}

func (r *LLMISVCReconciler) expectedSchedulerAuthDelegatorBinding(llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.ClusterRoleBinding {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   kmeta.ChildName(llmSvc.GetNamespace(), "-"+llmSvc.GetName()+"-epp-auth-rb"),
			Labels: SchedulerLabels(llmSvc),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      sa.GetName(),
			Namespace: sa.GetNamespace(),
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:auth-delegator",
		},
	}
	return crb
}

func (r *LLMISVCReconciler) expectedSchedulerRole(llmSvc *v1alpha2.LLMInferenceService) *rbacv1.Role {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-epp-role"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: SchedulerLabels(llmSvc),
		},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"inference.networking.k8s.io", "inference.networking.x-k8s.io"}, Resources: []string{"inferencepools", "inferenceobjectives", "inferencemodels"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"discovery.k8s.io"}, Resources: []string{"endpointslices"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"coordination.k8s.io"}, Resources: []string{"leases"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		},
	}
	return role
}

func (r *LLMISVCReconciler) expectedSchedulerRoleBinding(llmSvc *v1alpha2.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-epp-rb"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
			},
			Labels: SchedulerLabels(llmSvc),
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      sa.GetName(),
			Namespace: sa.GetNamespace(),
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     kmeta.ChildName(llmSvc.GetName(), "-epp-role"),
		},
	}
	return rb
}

func (r *LLMISVCReconciler) propagateSchedulerDeploymentStatus(ctx context.Context, expected *appsv1.Deployment, ready func(), notReady func(reason, messageFormat string, messageA ...interface{})) error {
	curr := &appsv1.Deployment{}
	err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Get(ctx, client.ObjectKeyFromObject(expected), curr)
	})
	if err != nil {
		return fmt.Errorf("failed to get current deployment %s/%s: %w", expected.GetNamespace(), expected.GetName(), err)
	}
	// HA mode is an active-passive setup, passive replicas will remain unavailable.
	if curr.Status.AvailableReplicas > 0 {
		ready()
		return nil
	}

	for _, cond := range curr.Status.Conditions {
		if cond.Type == appsv1.DeploymentProgressing {
			if cond.Status == corev1.ConditionFalse && cond.Reason == "ProgressDeadlineExceeded" {
				notReady(cond.Reason, cond.Message)
				return nil
			}
		}
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

	notReady(string(appsv1.DeploymentProgressing), "Deployment rollout in progress")
	return nil
}

type mutateSchedulerConfigFunc func(u *unstructured.Unstructured) error

func mutateSchedulerConfig(d *appsv1.Deployment, opts ...mutateSchedulerConfigFunc) error {
	schedulerContainer := utils.GetContainerWithName(&d.Spec.Template.Spec, "main")
	if schedulerContainer == nil {
		return nil
	}

	for i := range len(schedulerContainer.Args) - 1 {
		if schedulerContainer.Args[i] == "--config-text" || schedulerContainer.Args[i] == "-config-text" {
			u := unstructured.Unstructured{}
			if err := yaml.Unmarshal([]byte(schedulerContainer.Args[i+1]), &u); err != nil {
				// Config text is not a valid YAML object (e.g. a plain string from a user-provided template),
				// skip mutation as there's no structured config to modify.
				return nil //nolint:nilerr // unmarshal error is intentionally discarded for non-YAML config values
			}
			for _, opt := range opts {
				if err := opt(&u); err != nil {
					return fmt.Errorf("failed to mutate config for scheduler deployment %s/%s: %w", d.GetNamespace(), d.GetName(), err)
				}
			}
			out, err := yaml.Marshal(u.Object)
			if err != nil {
				return fmt.Errorf("failed to marshal mutated config for scheduler deployment %s/%s: %w", d.GetNamespace(), d.GetName(), err)
			}
			schedulerContainer.Args[i+1] = string(out)
		}
	}
	return nil
}

func WithUdsTokenizerConfig(u *unstructured.Unstructured) error {
	var (
		precisePrefixCacheScorerPlugin = "precise-prefix-cache-scorer"
		modelNameField                 = []string{"parameters", "indexerConfig", "tokenizersPoolConfig", "modelName"}
		udsSocketFileField             = []string{"parameters", "indexerConfig", "tokenizersPoolConfig", "uds", "socketFile"}
	)

	val, found, err := unstructured.NestedFieldNoCopy(u.Object, "plugins")
	if err != nil || !found {
		return err
	}
	plugins, ok := val.([]interface{})
	if !ok {
		return nil
	}

	for _, plugin := range plugins {
		pluginMap, ok := plugin.(map[string]interface{})
		if !ok || pluginMap["type"] != precisePrefixCacheScorerPlugin {
			continue
		}

		if err := unstructured.SetNestedField(pluginMap, udsTokenizerBaseModelName, modelNameField...); err != nil {
			return err
		}
		if err := unstructured.SetNestedField(pluginMap, udsTokenizerSocketFile, udsSocketFileField...); err != nil {
			return err
		}
	}

	return nil
}

func semanticServiceIsEqual(expected *corev1.Service, current *corev1.Service) bool {
	return equality.Semantic.DeepDerivative(expected.Spec, current.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, current.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, current.Annotations)
}

func semanticInferencePoolIsEqual(expected *igwapi.InferencePool, curr *igwapi.InferencePool) bool {
	return equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func semanticInferencePoolV1Alpha2IsEqual(expected *igwapiv1alpha2.InferencePool, curr *igwapiv1alpha2.InferencePool) bool {
	return equality.Semantic.DeepDerivative(expected.Spec, curr.Spec) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func semanticServiceAccountIsEqual(expected *corev1.ServiceAccount, current *corev1.ServiceAccount) bool {
	return equality.Semantic.DeepDerivative(expected.Secrets, current.Secrets) &&
		equality.Semantic.DeepDerivative(expected.ImagePullSecrets, current.ImagePullSecrets) &&
		equality.Semantic.DeepDerivative(expected.Labels, current.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, current.Annotations)
}

func semanticRoleIsEqual(expected *rbacv1.Role, curr *rbacv1.Role) bool {
	return equality.Semantic.DeepDerivative(expected.Rules, curr.Rules) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func semanticClusterRoleBindingIsEqual(expected *rbacv1.ClusterRoleBinding, curr *rbacv1.ClusterRoleBinding) bool {
	return equality.Semantic.DeepDerivative(expected.Subjects, curr.Subjects) &&
		equality.Semantic.DeepDerivative(expected.RoleRef, curr.RoleRef) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func semanticRoleBindingIsEqual(expected *rbacv1.RoleBinding, curr *rbacv1.RoleBinding) bool {
	return equality.Semantic.DeepDerivative(expected.Subjects, curr.Subjects) &&
		equality.Semantic.DeepDerivative(expected.RoleRef, curr.RoleRef) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expected.Annotations, curr.Annotations)
}

func SchedulerLabels(llmSvc *v1alpha2.LLMInferenceService) map[string]string {
	return map[string]string{
		constants.KubernetesComponentLabelKey: constants.LLMComponentRouterScheduler,
		constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
		constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
	}
}
