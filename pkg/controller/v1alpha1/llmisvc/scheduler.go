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

	"k8s.io/utils/ptr"

	"k8s.io/apimachinery/pkg/util/sets"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwv1 "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func (r *LLMISVCReconciler) reconcileScheduler(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	log.FromContext(ctx).Info("Reconciling Scheduler")

	if err := r.reconcileSchedulerServiceAccount(ctx, llmSvc); err != nil {
		return err
	}
	if err := r.reconcileSchedulerInferenceModel(ctx, llmSvc); err != nil {
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
	return nil
}

// reconcileSchedulerAuthDelegatorBinding reconciles the auth-delegator role binding associated with the scheduler's service account.
// This role binding allows the scheduler's service account to perform authentication and authorization checks.
func (r *LLMISVCReconciler) reconcileSchedulerAuthDelegatorBinding(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount) error {
	authDelegatorBinding := r.expectedSchedulerAuthDelegatorBinding(llmSvc, sa)
	if !llmSvc.DeletionTimestamp.IsZero() || llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, authDelegatorBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.ClusterRoleBinding{}, authDelegatorBinding, semanticClusterRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler clusterrolebinding %s: %w", authDelegatorBinding.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSchedulerRole(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	role := r.expectedSchedulerRole(llmSvc)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, role)
	}
	if err := Reconcile(ctx, r, llmSvc, &rbacv1.Role{}, role, semanticRoleIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler role %s/%s: %w", role.GetNamespace(), role.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSchedulerRoleBinding(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount) error {
	roleBinding := r.expectedSchedulerRoleBinding(llmSvc, sa)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, roleBinding)
	}

	if err := Reconcile(ctx, r, llmSvc, &rbacv1.RoleBinding{}, roleBinding, semanticRoleBindingIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler rolebinding %s/%s: %w", roleBinding.GetNamespace(), roleBinding.GetName(), err)
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSchedulerServiceAccount(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	serviceAccount := r.expectedSchedulerServiceAccount(llmSvc)

	if !llmSvc.DeletionTimestamp.IsZero() {
		return r.reconcileSchedulerAuthDelegatorBinding(ctx, llmSvc, serviceAccount)
	}

	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, serviceAccount)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.ServiceAccount{}, serviceAccount, semanticServiceAccountIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler service account %s/%s: %w", serviceAccount.GetNamespace(), serviceAccount.GetName(), err)
	}

	if err := r.reconcileSchedulerAuthDelegatorBinding(ctx, llmSvc, serviceAccount); err != nil {
		return err
	}

	if err := r.reconcileSchedulerRole(ctx, llmSvc); err != nil {
		return err
	}

	return r.reconcileSchedulerRoleBinding(ctx, llmSvc, serviceAccount)
}

func (r *LLMISVCReconciler) reconcileSchedulerDeployment(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	scheduler := r.expectedSchedulerDeployment(ctx, llmSvc)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return Delete(ctx, r, llmSvc, scheduler)
	}
	if err := Reconcile(ctx, r, llmSvc, &appsv1.Deployment{}, scheduler, semanticDeploymentIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile scheduler deployment %s/%s: %w", scheduler.GetNamespace(), scheduler.GetName(), err)
	}
	return r.propagateDeploymentStatus(ctx, scheduler, llmSvc.MarkSchedulerWorkloadReady, llmSvc.MarkSchedulerWorkloadNotReady)
}

// consider pool "Ready" if any Parent has Accepted=True AND ResolvedRefs=True
func isV1PoolReady(p *igwv1.InferencePool) bool {
	for _, ps := range p.Status.Parents {
		accepted, resolved := false, false
		for _, c := range ps.Conditions {
			if string(c.Type) == "Accepted" && string(c.Status) == "True" { accepted = true }
			if string(c.Type) == "ResolvedRefs" && string(c.Status) == "True" { resolved = true }
		}
		if accepted && resolved { return true }
	}
	return false
}

// alpha2 check via dynamic client
func (r *LLMISVCReconciler) isAlpha2PoolReady(ctx context.Context, ns, name string) bool {
	u, err := r.DynamicClient.Resource(gvrInferencePoolV1Alpha2).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil { return false }
	parents, _, _ := unstructured.NestedSlice(u.Object, "status", "parents")
	for _, p := range parents {
		pm, _ := p.(map[string]any)
		conds, _, _ := unstructured.NestedSlice(pm, "conditions")
		accepted, resolved := false, false
		for _, cc := range conds {
			cm, _ := cc.(map[string]any)
			if cm["type"] == "Accepted" && cm["status"] == "True" { accepted = true }
			if cm["type"] == "ResolvedRefs" && cm["status"] == "True" { resolved = true }
		}
		if accepted && resolved { return true }
	}
	return false
}

func (r *LLMISVCReconciler) reconcileSchedulerInferencePool(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	// If router/scheduler disabled or BYO pool (HasRef), delete both variants and exit.
	expected := r.expectedSchedulerInferencePool(ctx, llmSvc)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
		if err := Delete(ctx, r, llmSvc, expected); err != nil { // v1 typed
			return err
		}
		return r.deleteAlpha2PoolIfExists(ctx, llmSvc) // best-effort alpha2
	}

	// 1) Ensure v1 InferencePool (typed) exists/updated.
	if err := Reconcile(ctx, r, llmSvc, &igwv1.InferencePool{}, expected, semanticInferencePoolIsEqual); err != nil {
		return err
	}

	// 2) Ensure v1alpha2 InferencePool (dynamic) exists/updated.
	if err := r.reconcileAlpha2Pool(ctx, llmSvc, expected); err != nil {
		return err
	}

	// ---- Readiness aggregation (prefer v1; fallback to v1alpha2) ----
	// Fetch current v1 to read Status (expected has no Status).
	cur := &igwv1.InferencePool{}
	if err := r.Client.Get(ctx, crclient.ObjectKey{
		Namespace: expected.Namespace, 
		Name: expected.Name,
	}, cur); err != nil {
		// If we can't fetch v1, treat v1 as not ready and rely on alpha2 below.
	}

	v1Ready := isV1PoolReady(cur)
	alpha2Ready := r.isAlpha2PoolReady(ctx, llmSvc.GetNamespace(), expected.GetName())

	if v1Ready || alpha2Ready {
		// Prefer v1 but either is acceptable.
		llmSvc.MarkRouterReady()
	} else {
		llmSvc.MarkRouterNotReady("InferencePoolNotReady", "Neither v1 nor v1alpha2 InferencePool reports Accepted=True and ResolvedRefs=True")
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSchedulerService(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	expected := r.expectedSchedulerService(ctx, llmSvc)
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil || llmSvc.Spec.Router.Scheduler.Template == nil {
		return Delete(ctx, r, llmSvc, expected)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.Service{}, expected, semanticServiceIsEqual); err != nil {
		return err
	}

	return nil
}

func (r *LLMISVCReconciler) reconcileSchedulerInferenceModel(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	// If scheduler disabled, best-effort delete v1alpha2 InferenceModel and return.
	if llmSvc.Spec.Router == nil || llmSvc.Spec.Router.Scheduler == nil {
		return r.deleteAlpha2InferenceModelIfExists(ctx, llmSvc)
	}

	u := r.expectedAlpha2InferenceModel(llmSvc)
	res := r.DynamicClient.Resource(gvrInferenceModelV1Alpha2).Namespace(u.GetNamespace())

	// Upsert pattern using dynamic client.
	cur, err := res.Get(ctx, u.GetName(), metav1.GetOptions{})
	if err == nil {
		u.SetResourceVersion(cur.GetResourceVersion())
		_, err = res.Update(ctx, u, metav1.UpdateOptions{})
		return err
	}
	_, err = res.Create(ctx, u, metav1.CreateOptions{})
	return err
}

// Build v1alpha2 InferenceModel unstructured.
// NOTE: We avoid v1 typed IM (doesn't exist). We write the fields the scheduler expects.
func (r *LLMISVCReconciler) expectedAlpha2InferenceModel(llmSvc *v1alpha1.LLMInferenceService) *unstructured.Unstructured {
	name := v1alpha1.InferenceModelName(llmSvc)
	group := "inference.networking.k8s.io" // pool group we target - updated to v1 group
	poolName := llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc)

	// Default modelName to resource name if spec.model.name is empty.
	modelName := ptr.Deref(llmSvc.Spec.Model.Name, llmSvc.GetName())

	// Default criticality to "Critical" if not set (keeps old behavior).
	criticality := "Critical"
	if llmSvc.Spec.Model.Criticality != nil && *llmSvc.Spec.Model.Criticality != "" {
		criticality = string(*llmSvc.Spec.Model.Criticality)
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "inference.networking.x-k8s.io/v1alpha2",
			"kind":       "InferenceModel",
			"metadata": map[string]any{
				"name":      name,
				"namespace": llmSvc.GetNamespace(),
				"labels":    r.schedulerLabels(llmSvc),
				"ownerReferences": []any{
					map[string]any{
						"apiVersion": v1alpha1.LLMInferenceServiceGVK.GroupVersion().String(),
						"kind":       v1alpha1.LLMInferenceServiceGVK.Kind,
						"name":       llmSvc.GetName(),
						"uid":        string(llmSvc.GetUID()),
						"controller": true,
					},
				},
			},
			"spec": map[string]any{
				"modelName": modelName,
				"poolRef": map[string]any{
					"group": group,
					"kind":  "InferencePool",
					"name":  poolName,
				},
				"criticality": criticality,
			},
		},
	}
}

func (r *LLMISVCReconciler) deleteAlpha2InferenceModelIfExists(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	name := v1alpha1.InferenceModelName(llmSvc)
	res := r.DynamicClient.Resource(gvrInferenceModelV1Alpha2).Namespace(llmSvc.GetNamespace())
	_, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil // not found or not installed so ignore
	}
	return res.Delete(ctx, name, metav1.DeleteOptions{})
}

func (r *LLMISVCReconciler) expectedSchedulerService(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *corev1.Service {
	logger := log.FromContext(ctx)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmSvc.Spec.Router.EPPServiceName(llmSvc),
			Namespace: llmSvc.GetNamespace(),
			Labels:    r.schedulerLabels(llmSvc),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: r.schedulerLabels(llmSvc),
		},
	}

	if llmSvc.Spec.Router.HasSchedulerTemplate() {
		podSpec := llmSvc.Spec.Router.Scheduler.Template.DeepCopy()

		desiredPorts := sets.New("grpc", "grpc-health", "metrics")

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

func (r *LLMISVCReconciler) expectedSchedulerInferencePool(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *igwv1.InferencePool {
	labels := r.schedulerLabels(llmSvc)

	ip := &igwv1.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-inference-pool"),
			Namespace: llmSvc.GetNamespace(),
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
	}
	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Pool != nil && llmSvc.Spec.Router.Scheduler.Pool.Spec != nil {
		ip.Spec = *llmSvc.Spec.Router.Scheduler.Pool.Spec.DeepCopy()
	}

	log.FromContext(ctx).V(2).Info("Expected router InferencePool", "inferencepool", ip)

	return ip
}

// Leftover typed v1 InferenceModel code (it doesn’t exist in v1)
// func (r *LLMISVCReconciler) expectedSchedulerInferenceModel(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *igwv1.InferenceModel {
// 	labels := r.schedulerLabels(llmSvc)

// 	im := &igwv1.InferenceModel{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      v1alpha1.InferenceModelName(llmSvc),
// 			Namespace: llmSvc.GetNamespace(),
// 			Labels:    labels,
// 			OwnerReferences: []metav1.OwnerReference{
// 				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
// 			},
// 		},
// 		Spec: igwv1.InferenceModelSpec{
// 			ModelName: ptr.Deref(llmSvc.Spec.Model.Name, llmSvc.GetName()),
// 			PoolRef: igwv1.PoolObjectReference{
// 				Group: "inference.networking.x-k8s.io",
// 				Kind:  "InferencePool",
// 				Name:  igwv1.ObjectName(llmSvc.Spec.Router.Scheduler.InferencePoolName(llmSvc)),
// 			},
// 			Criticality: llmSvc.Spec.Model.Criticality,
// 		},
// 	}
// 	if im.Spec.Criticality == nil {
// 		im.Spec.Criticality = ptr.To(igwv1.Critical)
// 	}

// 	log.FromContext(ctx).V(2).Info("Expected InferenceModel", "inferencemodel", im)

// 	return im
// }

func (r *LLMISVCReconciler) expectedSchedulerDeployment(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *appsv1.Deployment {
	labels := r.schedulerLabels(llmSvc)
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-router-scheduler"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
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

	if llmSvc.Spec.Router.HasSchedulerTemplate() {
		d.Spec.Template.Spec = *llmSvc.Spec.Router.Scheduler.Template.DeepCopy()
		for i := range d.Spec.Template.Spec.Containers {
			if d.Spec.Template.Spec.Containers[i].Name != "main" {
				continue
			}

			if slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "--configText") ||
				slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "-configText") ||
				slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "--configFile") ||
				slices.Contains(d.Spec.Template.Spec.Containers[i].Args, "-configFile") {
				// When the configuration is overridden, don't add/override it.
				break
			}

			d.Spec.Template.Spec.Containers[i].Args = append(d.Spec.Template.Spec.Containers[i].Args,
				"--configText",
				schedulerConfigText(llmSvc),
			)
		}
	}

	log.FromContext(ctx).V(2).Info("Expected router scheduler deployment", "deployment", d)

	return d
}

func schedulerConfigText(llmSvc *v1alpha1.LLMInferenceService) string {
	switch {
	case llmSvc.Spec.Prefill != nil:
		return `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: pd-profile-handler
  parameters:
    threshold: 100
- type: prefill-header-handler
- type: prefill-filter
- type: decode-filter
- type: prefix-cache-scorer
- type: load-aware-scorer
- type: max-score-picker
schedulingProfiles:
- name: prefill
  plugins:
  - pluginRef: prefill-filter
  - pluginRef: prefix-cache-scorer
    weight: 2.0
  - pluginRef: load-aware-scorer
    weight: 1.0
  - pluginRef: max-score-picker
- name: decode
  plugins:
  - pluginRef: decode-filter
  - pluginRef: prefix-cache-scorer
    weight: 2.0
  - pluginRef: load-aware-scorer
    weight: 1.0
  - pluginRef: max-score-picker
`
	default:
		return `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: prefix-cache-scorer
- type: load-aware-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: prefix-cache-scorer
    weight: 2.0
  - pluginRef: load-aware-scorer
    weight: 1.0
  - pluginRef: max-score-picker
`
	}
}

func (r *LLMISVCReconciler) expectedSchedulerServiceAccount(llmSvc *v1alpha1.LLMInferenceService) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-epp-sa"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.schedulerLabels(llmSvc),
		},
	}

	if llmSvc.Spec.Router != nil &&
		llmSvc.Spec.Router.Scheduler != nil &&
		llmSvc.Spec.Router.Scheduler.Template != nil &&
		llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName != "" {
		sa.Name = llmSvc.Spec.Router.Scheduler.Template.ServiceAccountName
	}

	return sa
}

func (r *LLMISVCReconciler) expectedSchedulerAuthDelegatorBinding(llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.ClusterRoleBinding {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   kmeta.ChildName(llmSvc.GetNamespace(), "-"+llmSvc.GetName()+"-epp-auth-rb"),
			Labels: r.schedulerLabels(llmSvc),
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

func (r *LLMISVCReconciler) expectedSchedulerRole(llmSvc *v1alpha1.LLMInferenceService) *rbacv1.Role {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-epp-role"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.schedulerLabels(llmSvc),
		},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"inference.networking.x-k8s.io"}, Resources: []string{"inferencepools", "inferencemodels"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"inference.networking.k8s.io"}, Resources: []string{"inferencepools", "inferencemodels"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"discovery.k8s.io"}, Resources: []string{"endpointslices"}, Verbs: []string{"get", "list", "watch"}},
		},
	}
	return role
}

func (r *LLMISVCReconciler) expectedSchedulerRoleBinding(llmSvc *v1alpha1.LLMInferenceService, sa *corev1.ServiceAccount) *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-epp-rb"),
			Namespace: llmSvc.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
			Labels: r.schedulerLabels(llmSvc),
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

// func semanticInferenceModelIsEqual(expected *igwv1.InferenceModel, current *igwv1.InferenceModel) bool {
// 	return equality.Semantic.DeepDerivative(expected.Spec, current.Spec) &&
// 		equality.Semantic.DeepDerivative(expected.Labels, current.Labels) &&
// 		equality.Semantic.DeepDerivative(expected.Annotations, current.Annotations)
// }

func semanticInferencePoolIsEqual(expected *igwv1.InferencePool, curr *igwv1.InferencePool) bool {
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

func (r *LLMISVCReconciler) schedulerLabels(llmSvc *v1alpha1.LLMInferenceService) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component": "llminferenceservice-router-scheduler",
		"app.kubernetes.io/name":      llmSvc.GetName(),
		"app.kubernetes.io/part-of":   "llminferenceservice",
	}
}

var gvrInferencePoolV1Alpha2 = schema.GroupVersionResource{
	Group:    "inference.networking.x-k8s.io",
	Version:  "v1alpha2",
	Resource: "inferencepools",
}

var gvrInferenceModelV1Alpha2 = schema.GroupVersionResource{
	Group:    "inference.networking.x-k8s.io",
	Version:  "v1alpha2",
	Resource: "inferencemodels",
}

func (r *LLMISVCReconciler) reconcileAlpha2Pool(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService, v1pool *igwv1.InferencePool) error {
	u, err := v1ToAlpha2Unstructured(v1pool)
	if err != nil {
		return err
	}
	res := r.DynamicClient.Resource(gvrInferencePoolV1Alpha2).Namespace(u.GetNamespace())
	cur, err := res.Get(ctx, u.GetName(), metav1.GetOptions{})
	if err == nil {
		u.SetResourceVersion(cur.GetResourceVersion())
		_, err = res.Update(ctx, u, metav1.UpdateOptions{})
		return err
	}
	_, err = res.Create(ctx, u, metav1.CreateOptions{})
	return err
}

func (r *LLMISVCReconciler) deleteAlpha2PoolIfExists(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	name := kmeta.ChildName(llmSvc.GetName(), "-inference-pool")
	res := r.DynamicClient.Resource(gvrInferencePoolV1Alpha2).Namespace(llmSvc.GetNamespace())
	_, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil // nothing to delete or not found
	}
	return res.Delete(ctx, name, metav1.DeleteOptions{})
}

// Convert the typed v1 pool to a v1alpha2 unstructured object.
// NOTE: v1 uses typed LabelKey/LabelValue and non-pointer Kind/Number/FailureMode.
// We convert keys/values to strings and use "" / >0 checks instead of nil checks.
func v1ToAlpha2Unstructured(v1p *igwv1.InferencePool) (*unstructured.Unstructured, error) {
	if v1p == nil {
		return nil, fmt.Errorf("nil v1 pool")
	}

	// selector: v1 -> v1alpha2 (string map)
	selector := map[string]any{}
	if v1p.Spec.Selector.MatchLabels != nil {
		for k, v := range v1p.Spec.Selector.MatchLabels {
			selector[string(k)] = string(v) // v1 uses typed keys/values; alpha2 wants plain strings
		}
	}

	// target port: v1 TargetPorts[0].Number -> alpha2 targetPortNumber (int)
	if len(v1p.Spec.TargetPorts) == 0 {
		return nil, fmt.Errorf("spec.targetPorts[0] required")
	}
	tp := int(v1p.Spec.TargetPorts[0].Number) // Number is a non-pointer alias (int32)

	// endpointPickerRef -> extensionRef
	// IMPORTANT: Kind/Group/FailureMode are value types in v1, not pointers.
	ext := map[string]any{
		"name": string(v1p.Spec.EndpointPickerRef.Name),
	}
	if v1p.Spec.EndpointPickerRef.Group != nil && *v1p.Spec.EndpointPickerRef.Group != "" {
	ext["group"] = string(*v1p.Spec.EndpointPickerRef.Group) // ✅ deref the *Group
	}
	if s := string(v1p.Spec.EndpointPickerRef.Kind); s != "" {
		ext["kind"] = s
	}
	if v1p.Spec.EndpointPickerRef.Port != nil && v1p.Spec.EndpointPickerRef.Port.Number > 0 {
		ext["portNumber"] = int(v1p.Spec.EndpointPickerRef.Port.Number)
	}
	if s := string(v1p.Spec.EndpointPickerRef.FailureMode); s != "" {
		ext["failureMode"] = s
	}

	u := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "inference.networking.x-k8s.io/v1alpha2",
			"kind":       "InferencePool",
			"metadata": map[string]any{
				"name":        v1p.Name,
				"namespace":   v1p.Namespace,
				"labels":      v1p.Labels,
				"annotations": v1p.Annotations,
			},
			"spec": map[string]any{
				"selector":         selector,
				"targetPortNumber": tp,
				"extensionRef":     ext,
			},
		},
	}
	return u, nil
}