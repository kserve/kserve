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
	"maps"
	"path"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	pvcEvictorImage          = "quay.io/wenzhou/llm-d-kv-cache-pvc-evictor:latest" // TODO: replace once we have the new image
	pvcEvictorCacheDirectory = "kv/model-cache/models"
	pvcEvictorMountPath      = "/mnt/kv-cache-evictor"
	pvcEvictorServiceAccount = "llm-d-kv-cache-pvc-evictor"
	pvcEvictorLabel          = "serving.kserve.io/pvc-evictor"
	pvcEvictorKeyLabel       = "serving.kserve.io/pvc-evictor-key"
	pvcEvictorConsumerPrefix = "serving.kserve.io/pvc-evictor-consumer-"
	pvcEvictorPVCName        = "serving.kserve.io/pvc-name"
	pvcEvictorPVCPath        = "serving.kserve.io/pvc-path"
	pvcEvictorSecurityHash   = "serving.kserve.io/pvc-security-context-hash"
)

func pvcEvictorContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{corev1.Capability("ALL")},
		},
		RunAsNonRoot: ptr.To(true),
	}
}

func pvcEvictorResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
}

func pvcEvictorDeploymentPredicate() predicate.Predicate {
	isEvictor := func(object client.Object) bool {
		return object != nil && object.GetLabels()[pvcEvictorLabel] == "true"
	}
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isEvictor(e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isEvictor(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isEvictor(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !isEvictor(e.ObjectOld) && !isEvictor(e.ObjectNew) {
				return false
			}
			oldDeployment, oldOK := e.ObjectOld.(*appsv1.Deployment)
			newDeployment, newOK := e.ObjectNew.(*appsv1.Deployment)
			if !oldOK || !newOK {
				return true
			}
			return !equality.Semantic.DeepEqual(oldDeployment.Spec, newDeployment.Spec) ||
				!equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels()) ||
				!equality.Semantic.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations()) ||
				!equality.Semantic.DeepEqual(e.ObjectOld.GetOwnerReferences(), e.ObjectNew.GetOwnerReferences())
		},
	}
}

func (r *LLMISVCReconciler) enqueueOnPVCEvictorDeploymentChange() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		services := &v1alpha2.LLMInferenceServiceList{}
		if err := r.List(ctx, services, client.InNamespace(object.GetNamespace())); err != nil {
			return nil
		}
		requests := make([]reconcile.Request, 0, len(services.Items))
		for i := range services.Items {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&services.Items[i])})
		}
		return requests
	})
}

type pvcEvictorTarget struct {
	claimName            string
	subPath              string
	securityContext      *corev1.PodSecurityContext
	requiresNodeAffinity bool
}

func (r *LLMISVCReconciler) finalizePVCEvictors(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	config, err := r.loadConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load configuration for PVC evictor cleanup: %w", err)
	}
	return r.cleanupUnusedPVCEvictors(ctx, llmSvc.Namespace, config, nil, llmSvc.Name)
}

func (r *LLMISVCReconciler) reconcilePVCEvictors(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) error {
	expected, err := r.expectedPVCEvictorDeployments(ctx, llmSvc)
	if err != nil {
		return err
	}
	if len(expected) > 0 {
		if err := r.ensurePVCEvictorServiceAccount(ctx, llmSvc.Namespace); err != nil {
			return err
		}
	}

	current := &appsv1.DeploymentList{}
	if err := r.List(ctx, current,
		client.InNamespace(llmSvc.Namespace),
		client.MatchingLabels{pvcEvictorLabel: "true"},
	); err != nil {
		return fmt.Errorf("failed to list PVC evictor deployments: %w", err)
	}

	logger := log.FromContext(ctx)
	for _, deployment := range expected {
		matches := findPVCEvictorDeployments(current.Items, deployment.Name, deployment.Annotations[pvcEvictorPVCName], deployment.Annotations[pvcEvictorPVCPath])
		if len(matches) > 0 {
			existing := matches[0]
			if err := r.reconcileExistingPVCEvictorDeployment(ctx, existing, deployment); err != nil {
				return err
			}
			for _, duplicate := range matches[1:] {
				if err := r.Delete(ctx, duplicate); err != nil && !apierrors.IsNotFound(err) {
					return fmt.Errorf("failed to delete duplicate PVC evictor Deployment %s/%s: %w", duplicate.Namespace, duplicate.Name, err)
				}
				logger.Info("Deleted duplicate shared PVC evictor Deployment", "deployment", duplicate.Name,
					"pvcName", deployment.Annotations[pvcEvictorPVCName], "pvcPath", deployment.Annotations[pvcEvictorPVCPath])
			}
			logger.Info("Reusing existing shared PVC evictor Deployment", "deployment", existing.Name,
				"pvcName", deployment.Annotations[pvcEvictorPVCName], "pvcPath", deployment.Annotations[pvcEvictorPVCPath])
			continue
		}
		createErr := r.Create(ctx, deployment)
		if createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			return fmt.Errorf("failed to create shared PVC evictor Deployment %s: %w", deployment.Name, createErr)
		}
		if apierrors.IsAlreadyExists(createErr) {
			existing, getErr := r.getPVCEvictorDeployment(ctx, deployment)
			if getErr != nil {
				return fmt.Errorf("failed to inspect existing PVC evictor Deployment %s/%s: %w", deployment.Namespace, deployment.Name, getErr)
			}
			if !samePVCEvictorIdentity(existing, deployment) {
				return fmt.Errorf("Deployment %s/%s already exists but does not identify the expected PVC evictor", deployment.Namespace, deployment.Name)
			}
			if err := r.reconcileExistingPVCEvictorDeployment(ctx, existing, deployment); err != nil {
				return err
			}
			logger.Info("Shared PVC evictor Deployment was created concurrently; reusing it", "deployment", deployment.Name)
		}
	}
	return r.cleanupUnusedPVCEvictors(ctx, llmSvc.Namespace, config, llmSvc, "")
}

func (r *LLMISVCReconciler) ensurePVCEvictorServiceAccount(ctx context.Context, namespace string) error {
	serviceAccount := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcEvictorServiceAccount, Namespace: namespace}, serviceAccount)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get PVC evictor ServiceAccount %s/%s: %w", namespace, pvcEvictorServiceAccount, err)
	}
	if err := r.Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcEvictorServiceAccount,
			Namespace: namespace,
			Labels:    map[string]string{pvcEvictorLabel: "true"},
		},
		AutomountServiceAccountToken: ptr.To(false),
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create PVC evictor ServiceAccount %s/%s: %w", namespace, pvcEvictorServiceAccount, err)
	}
	return nil
}

func (r *LLMISVCReconciler) getPVCEvictorDeployment(ctx context.Context, expected *appsv1.Deployment) (*appsv1.Deployment, error) {
	if r.Clientset != nil {
		return r.Clientset.AppsV1().Deployments(expected.Namespace).Get(ctx, expected.Name, metav1.GetOptions{})
	}
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(expected), deployment)
	return deployment, err
}

func findPVCEvictorDeployments(deployments []appsv1.Deployment, expectedName, pvcName, pvcPath string) []*appsv1.Deployment {
	matches := make([]*appsv1.Deployment, 0)
	for i := range deployments {
		deployment := &deployments[i]
		if deployment.Annotations[pvcEvictorPVCName] == pvcName && deployment.Annotations[pvcEvictorPVCPath] == pvcPath {
			matches = append(matches, deployment)
		}
	}
	for i := range deployments {
		deployment := &deployments[i]
		if deployment.Name == expectedName && samePVCEvictorAnnotations(deployment, pvcName, pvcPath) {
			for _, match := range matches {
				if match.Name == deployment.Name {
					return append([]*appsv1.Deployment{deployment}, removePVCEvictorDeployment(matches, deployment.Name)...)
				}
			}
			matches = append([]*appsv1.Deployment{deployment}, matches...)
			break
		}
	}
	return matches
}

func removePVCEvictorDeployment(deployments []*appsv1.Deployment, name string) []*appsv1.Deployment {
	remaining := make([]*appsv1.Deployment, 0, len(deployments))
	for _, deployment := range deployments {
		if deployment.Name != name {
			remaining = append(remaining, deployment)
		}
	}
	return remaining
}

func samePVCEvictorAnnotations(deployment *appsv1.Deployment, pvcName, pvcPath string) bool {
	return deployment.Annotations[pvcEvictorPVCName] == pvcName && deployment.Annotations[pvcEvictorPVCPath] == pvcPath
}

func samePVCEvictorIdentity(current, expected *appsv1.Deployment) bool {
	return samePVCEvictorAnnotations(current, expected.Annotations[pvcEvictorPVCName], expected.Annotations[pvcEvictorPVCPath])
}

func (r *LLMISVCReconciler) reconcileExistingPVCEvictorDeployment(ctx context.Context, current, expected *appsv1.Deployment) error {
	if !current.DeletionTimestamp.IsZero() {
		return nil
	}
	if current.Annotations[pvcEvictorSecurityHash] != expected.Annotations[pvcEvictorSecurityHash] &&
		(current.Annotations[pvcEvictorSecurityHash] != "" || expected.Annotations[pvcEvictorSecurityHash] != "") {
		return fmt.Errorf("shared PVC evictor Deployment %s/%s has different security settings; all LLMInferenceServices using PVC %q and path %q must use the same evictorSecurityContext", current.Namespace, current.Name, expected.Annotations[pvcEvictorPVCName], expected.Annotations[pvcEvictorPVCPath])
	}
	if !equality.Semantic.DeepEqual(current.Spec.Selector, expected.Spec.Selector) {
		if err := r.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete shared PVC evictor Deployment with changed selector %s/%s: %w", current.Namespace, current.Name, err)
		}
		return nil
	}
	if equality.Semantic.DeepEqual(current.Spec, expected.Spec) &&
		managedPVCEvictorMetadataEqual(current.Labels, expected.Labels) &&
		managedPVCEvictorMetadataEqual(current.Annotations, expected.Annotations) &&
		equality.Semantic.DeepEqual(current.OwnerReferences, retainExternalOwnerReferences(current.OwnerReferences)) {
		return nil
	}

	updated := current.DeepCopy()
	updated.Spec = *expected.Spec.DeepCopy()
	updated.Labels = maps.Clone(current.Labels)
	maps.Copy(updated.Labels, expected.Labels)
	updated.Annotations = maps.Clone(current.Annotations)
	maps.Copy(updated.Annotations, expected.Annotations)
	updated.OwnerReferences = retainExternalOwnerReferences(current.OwnerReferences)
	if err := r.Update(ctx, updated); err != nil {
		return fmt.Errorf("failed to update shared PVC evictor Deployment %s/%s: %w", current.Namespace, current.Name, err)
	}
	return nil
}

func managedPVCEvictorMetadataEqual(current, expected map[string]string) bool {
	for key, value := range expected {
		if current[key] != value {
			return false
		}
	}
	return true
}

func retainExternalOwnerReferences(ownerReferences []metav1.OwnerReference) []metav1.OwnerReference {
	retained := make([]metav1.OwnerReference, 0, len(ownerReferences))
	for _, ownerReference := range ownerReferences {
		if ownerReference.Kind == "LLMInferenceService" && strings.HasPrefix(ownerReference.APIVersion, "serving.kserve.io/") {
			continue
		}
		retained = append(retained, ownerReference)
	}
	return retained
}

func (r *LLMISVCReconciler) expectedPVCEvictorDeployments(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) (map[string]*appsv1.Deployment, error) {
	if utils.GetForceStopRuntime(llmSvc) {
		return map[string]*appsv1.Deployment{}, nil
	}
	targets := collectPVCEvictorTargets(llmSvc)
	expected := make(map[string]*appsv1.Deployment, len(targets))
	for i := range targets {
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, types.NamespacedName{Name: targets[i].claimName, Namespace: llmSvc.Namespace}, pvc); err != nil {
			return nil, fmt.Errorf("failed to get PVC %s/%s for evictor: %w", llmSvc.Namespace, targets[i].claimName, err)
		}
		if pvcHasReadWriteOncePod(pvc) || pvcHasReadOnlyMany(pvc) {
			return nil, fmt.Errorf("PVC %s/%s is not writable for a pvc.ref evictor; use ReadWriteOnce or ReadWriteMany", llmSvc.Namespace, targets[i].claimName)
		}
		targets[i].requiresNodeAffinity = pvcRequiresNodeAffinity(pvc)
		target := targets[i]
		deployment := expectedPVCEvictorDeployment(llmSvc, target)
		expected[deployment.Name] = deployment
	}
	return expected, nil
}

func pvcRequiresNodeAffinity(pvc *corev1.PersistentVolumeClaim) bool {
	for _, accessMode := range pvc.Spec.AccessModes {
		if accessMode == corev1.ReadWriteOnce {
			return true
		}
	}
	return false
}

func pvcHasReadWriteOncePod(pvc *corev1.PersistentVolumeClaim) bool {
	for _, accessMode := range pvc.Spec.AccessModes {
		if accessMode == corev1.ReadWriteOncePod {
			return true
		}
	}
	return false
}

func pvcHasReadOnlyMany(pvc *corev1.PersistentVolumeClaim) bool {
	for _, accessMode := range pvc.Spec.AccessModes {
		if accessMode == corev1.ReadOnlyMany {
			return true
		}
	}
	return false
}

func pvcEvictorConsumerLabelKey(claimName, subPath string) string {
	return pvcEvictorConsumerPrefix + pvcEvictorKey(claimName, subPath)
}

func addPVCEvictorConsumerLabels(labels map[string]string, secondary []v1alpha2.SecondaryTierSpec) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}
	for _, tier := range secondary {
		if tier.FileSystem == nil || tier.FileSystem.PVC == nil ||
			tier.FileSystem.PVC.Evictor == nil || !*tier.FileSystem.PVC.Evictor ||
			tier.FileSystem.PVC.Ref == nil {
			continue
		}
		ref := tier.FileSystem.PVC.Ref
		labels[pvcEvictorConsumerLabelKey(ref.Name, normalizePVCEvictorPath(ref.Path))] = "true"
	}
	return labels
}

func (r *LLMISVCReconciler) cleanupUnusedPVCEvictors(ctx context.Context, namespace string, config *Config, currentService *v1alpha2.LLMInferenceService, excludedService string) error {
	referenced, cleanupSafe, err := r.referencedPVCEvictorIdentities(ctx, namespace, config, currentService, excludedService)
	if err != nil {
		return err
	}
	if !cleanupSafe {
		log.FromContext(ctx).Info("Skipping PVC evictor cleanup because one or more LLMInferenceServices could not be resolved")
		return nil
	}

	current := &appsv1.DeploymentList{}
	if err := r.List(ctx, current, client.InNamespace(namespace), client.MatchingLabels{pvcEvictorLabel: "true"}); err != nil {
		return fmt.Errorf("failed to list PVC evictor Deployments for cleanup: %w", err)
	}
	deleted := make(map[string]struct{})
	for i := range current.Items {
		deployment := &current.Items[i]
		pvcName := deployment.Annotations[pvcEvictorPVCName]
		pvcPath := deployment.Annotations[pvcEvictorPVCPath]
		if pvcName == "" {
			continue
		}
		if _, ok := referenced[pvcEvictorIdentity(pvcName, pvcPath)]; ok {
			continue
		}
		if err := r.Delete(ctx, deployment); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete unused PVC evictor Deployment %s/%s: %w", namespace, deployment.Name, err)
		}
		deleted[deployment.Name] = struct{}{}
	}
	return r.cleanupPVCEvictorServiceAccount(ctx, namespace, referenced, deleted)
}

func (r *LLMISVCReconciler) cleanupPVCEvictorServiceAccount(ctx context.Context, namespace string, referenced map[string]struct{}, deleted map[string]struct{}) error {
	if len(referenced) > 0 {
		return nil
	}

	current := &appsv1.DeploymentList{}
	if err := r.List(ctx, current, client.InNamespace(namespace), client.MatchingLabels{pvcEvictorLabel: "true"}); err != nil {
		return fmt.Errorf("failed to list PVC evictor Deployments before ServiceAccount cleanup: %w", err)
	}
	if len(current.Items) > 0 {
		for i := range current.Items {
			if current.Items[i].DeletionTimestamp.IsZero() {
				if _, ok := deleted[current.Items[i].Name]; !ok {
					return nil
				}
			}
		}
	}

	serviceAccount := &corev1.ServiceAccount{}
	if err := r.Get(ctx, types.NamespacedName{Name: pvcEvictorServiceAccount, Namespace: namespace}, serviceAccount); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get PVC evictor ServiceAccount %s/%s for cleanup: %w", namespace, pvcEvictorServiceAccount, err)
	}
	if serviceAccount.Labels[pvcEvictorLabel] != "true" {
		return nil
	}
	if err := r.Delete(ctx, serviceAccount); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete unused PVC evictor ServiceAccount %s/%s: %w", namespace, pvcEvictorServiceAccount, err)
	}
	return nil
}

func (r *LLMISVCReconciler) referencedPVCEvictorIdentities(ctx context.Context, namespace string, config *Config, currentService *v1alpha2.LLMInferenceService, excludedService string) (map[string]struct{}, bool, error) {
	services := &v1alpha2.LLMInferenceServiceList{}
	if err := r.List(ctx, services, client.InNamespace(namespace)); err != nil {
		return nil, false, fmt.Errorf("failed to list LLMInferenceServices for PVC evictor ownership: %w", err)
	}

	referenced := make(map[string]struct{})
	cleanupSafe := true
	logger := log.FromContext(ctx)
	for i := range services.Items {
		service := services.Items[i].DeepCopy()
		if service.Name == excludedService || utils.GetForceStopRuntime(service) {
			continue
		}
		if currentService != nil && service.Name == currentService.Name {
			service = currentService.DeepCopy()
		} else {
			combined, err := r.combineBaseRefsConfig(ctx, service, config)
			if err != nil {
				cleanupSafe = false
				logger.Error(err, "Could not resolve PVC evictor references; preserving existing evictors", "llmSvc", client.ObjectKeyFromObject(service))
				continue
			}
			service.Spec = combined.Config.Spec
		}
		if utils.GetForceStopRuntime(service) {
			continue
		}
		for _, target := range collectPVCEvictorTargets(service) {
			referenced[pvcEvictorIdentity(target.claimName, target.subPath)] = struct{}{}
		}
	}
	return referenced, cleanupSafe, nil
}

func collectPVCEvictorTargets(llmSvc *v1alpha2.LLMInferenceService) []pvcEvictorTarget {
	targets := make([]pvcEvictorTarget, 0)
	seen := make(map[string]struct{})
	add := func(workload *v1alpha2.WorkloadSpec) {
		if workload == nil || workload.KVCacheOffloading == nil {
			return
		}
		for _, tier := range workload.KVCacheOffloading.Secondary {
			if tier.FileSystem == nil || tier.FileSystem.PVC == nil ||
				tier.FileSystem.PVC.Evictor == nil || !*tier.FileSystem.PVC.Evictor ||
				tier.FileSystem.PVC.Ref == nil {
				continue
			}
			ref := tier.FileSystem.PVC.Ref
			subPath := normalizePVCEvictorPath(ref.Path)
			key := pvcEvictorIdentity(ref.Name, subPath)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, pvcEvictorTarget{
				claimName:       ref.Name,
				subPath:         subPath,
				securityContext: tier.FileSystem.PVC.EvictorSecurityContext.DeepCopy(),
			})
		}
	}

	add(&llmSvc.Spec.WorkloadSpec)
	add(llmSvc.Spec.Prefill)
	return targets
}

func normalizePVCEvictorPath(value string) string {
	if value == "" {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func pvcEvictorIdentity(claimName, subPath string) string {
	return claimName + "\x00" + normalizePVCEvictorPath(subPath)
}

func pvcEvictorKey(claimName, subPath string) string {
	hash := sha256.Sum256([]byte(pvcEvictorIdentity(claimName, subPath)))
	return hex.EncodeToString(hash[:])[:16]
}

func pvcEvictorSecurityContextHash(securityContext *corev1.PodSecurityContext) string {
	if securityContext == nil {
		return ""
	}
	data, err := json.Marshal(securityContext)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func expectedPVCEvictorDeployment(llmSvc *v1alpha2.LLMInferenceService, target pvcEvictorTarget) *appsv1.Deployment {
	key := pvcEvictorKey(target.claimName, target.subPath)
	name := kmeta.ChildName("pvc-evictor", "-"+key)
	labels := map[string]string{
		pvcEvictorLabel:                    "true",
		pvcEvictorKeyLabel:                 key,
		constants.KubernetesPartOfLabelKey: constants.LLMInferenceServicePartOfValue,
	}

	podSecurityContext := &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	if target.securityContext != nil {
		podSecurityContext = target.securityContext.DeepCopy()
		podSecurityContext.RunAsNonRoot = ptr.To(true)
		if podSecurityContext.SeccompProfile == nil {
			podSecurityContext.SeccompProfile = &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			}
		}
	}

	podSpec := corev1.PodSpec{
		ServiceAccountName:           pvcEvictorServiceAccount,
		AutomountServiceAccountToken: ptr.To(false),
		SecurityContext:              podSecurityContext,
		Containers: []corev1.Container{{
			Name:            "pvc-evictor",
			Image:           pvcEvictorImage,
			ImagePullPolicy: corev1.PullAlways,
			SecurityContext: pvcEvictorContainerSecurityContext(),
			Env: []corev1.EnvVar{
				{Name: "PVC_MOUNT_PATH", Value: pvcEvictorMountPath},
				{Name: "CACHE_DIRECTORY", Value: pvcEvictorCacheDirectory},
			},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "kv-cache-storage",
				MountPath: pvcEvictorMountPath,
				SubPath:   target.subPath,
			}},
			Resources: pvcEvictorResources(),
		}},
		RestartPolicy: corev1.RestartPolicyAlways,
		Volumes: []corev1.Volume{{
			Name: "kv-cache-storage",
			VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: target.claimName,
			}},
		}},
	}
	if target.requiresNodeAffinity {
		podSpec.Affinity = &corev1.Affinity{PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
					pvcEvictorConsumerLabelKey(target.claimName, target.subPath): "true",
				}},
				TopologyKey: "kubernetes.io/hostname",
			}},
		}}
	}

	annotations := map[string]string{
		pvcEvictorPVCName: target.claimName,
		pvcEvictorPVCPath: target.subPath,
	}
	if securityHash := pvcEvictorSecurityContextHash(target.securityContext); securityHash != "" {
		annotations[pvcEvictorSecurityHash] = securityHash
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   llmSvc.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       podSpec,
			},
		},
	}
}

// attachPVCEvictor adds a sidecar that shares the PVC and subPath with the vLLM
// container. As a sidecar, it always runs on the same node as that vLLM pod.
func attachPVCEvictor(podSpec *corev1.PodSpec, volumeName, mountPath, subPath string) {
	name := "pvc-evictor-" + volumeName
	evictor := corev1.Container{
		Name:            name,
		Image:           pvcEvictorImage,
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: pvcEvictorContainerSecurityContext(),
		Resources:       pvcEvictorResources(),
		Env: []corev1.EnvVar{
			{Name: "PVC_MOUNT_PATH", Value: mountPath},
			{Name: "CACHE_DIRECTORY", Value: pvcEvictorCacheDirectory},
		},
		VolumeMounts: []corev1.VolumeMount{{
			Name:      volumeName,
			MountPath: mountPath,
			SubPath:   subPath,
		}},
	}

	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			podSpec.Containers[i] = evictor
			return
		}
	}
	podSpec.Containers = append(podSpec.Containers, evictor)
}

func kvCacheSecondaryRootDir(_ int, tier v1alpha2.SecondaryTierSpec, rootDir string) string {
	if tier.FileSystem != nil && tier.FileSystem.PVC != nil && tier.FileSystem.PVC.Evictor != nil && *tier.FileSystem.PVC.Evictor {
		return path.Join(rootDir, pvcEvictorCacheDirectory)
	}
	return rootDir
}
