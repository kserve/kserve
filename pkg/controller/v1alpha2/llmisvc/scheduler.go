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
	"sort"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	igwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/apix/v1alpha2"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// reconcileScheduler manages the scheduler component and its related resources
// The scheduler handles load balancing for inference pods
func (r *LLMISVCReconciler) reconcileScheduler(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	log.FromContext(ctx).Info("Reconciling Scheduler")

	if err := r.reconcileSchedulerServiceAccount(ctx, llmSvc); err != nil {
		return err
	}
	if err := r.reconcileSchedulerDeployment(ctx, llmSvc); err != nil {
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

func (r *LLMISVCReconciler) reconcileSchedulerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	scheduler := r.expectedSchedulerDeployment(ctx, llmSvc)
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
	return r.propagateDeploymentStatus(ctx, scheduler, llmSvc.MarkSchedulerWorkloadReady, llmSvc.MarkSchedulerWorkloadNotReady)
}

func (r *LLMISVCReconciler) reconcileSchedulerInferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	logger := log.FromContext(ctx)
	shouldDelete := utils.GetForceStopRuntime(llmSvc) || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef()

	if err := r.reconcileV1InferencePool(ctx, llmSvc, shouldDelete); err != nil {
		return err
	}

	if err := r.reconcileV1Alpha2InferencePool(ctx, llmSvc, shouldDelete); err != nil {
		return err
	}

	logger.V(2).Info("InferencePool reconciliation complete",
		"v1Available", r.InferencePoolV1Available,
		"v1alpha2Available", r.InferencePoolV1Alpha2Available)

	return nil
}

// reconcileV1InferencePool reconciles the v1 InferencePool if the CRD is available.
func (r *LLMISVCReconciler) reconcileV1InferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shouldDelete bool) error {
	if !r.InferencePoolV1Available {
		return nil
	}

	expected := r.expectedSchedulerInferencePool(ctx, llmSvc)
	if shouldDelete {
		return Delete(ctx, r, llmSvc, expected)
	}
	return Reconcile(ctx, r, llmSvc, &igwapi.InferencePool{}, expected, semanticInferencePoolIsEqual)
}

// reconcileV1Alpha2InferencePool reconciles the v1alpha2 InferencePool if the CRD is available.
func (r *LLMISVCReconciler) reconcileV1Alpha2InferencePool(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, shouldDelete bool) error {
	if !r.InferencePoolV1Alpha2Available {
		return nil
	}

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

		var servicePorts []corev1.ServicePort
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

func (r *LLMISVCReconciler) expectedSchedulerDeployment(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *appsv1.Deployment {
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

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Template != nil {
		d.Spec.Template.Spec = *llmSvc.Spec.Router.Scheduler.Template.DeepCopy()
		for i := range d.Spec.Template.Spec.Containers {
			if d.Spec.Template.Spec.Containers[i].Name != "main" {
				continue
			}

			if slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "--config-text") ||
				slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "-config-text") ||
				slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "--config-file") ||
				slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "-config-file") {
				// When the configuration is overridden, don't add/override it.
				break
			}

			d.Spec.Template.Spec.Containers[i].Args = append(d.Spec.Template.Spec.Containers[i].Args,
				"--config-text",
				schedulerConfigText(llmSvc),
			)
		}
	}

	log.FromContext(ctx).V(2).Info("Expected router scheduler deployment", "deployment", d)

	return d
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
- type: pd-profile-handler
  parameters:
    threshold: 0
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
		err := r.Client.Get(ctx, types.NamespacedName{Name: existingServiceAccountName, Namespace: llmSvc.Namespace}, existingServiceAccount)
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
			{APIGroups: []string{"inference.networking.k8s.io", "inference.networking.x-k8s.io"}, Resources: []string{"inferencepools", "inferenceobjectives"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"discovery.k8s.io"}, Resources: []string{"endpointslices"}, Verbs: []string{"get", "list", "watch"}},
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
