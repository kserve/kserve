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
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
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
	pvcEvictorImage          = "quay.io/opendatahub/odh-llm-d-kv-pvc-evictor:odh-stable"
	pvcEvictorCacheDirectory = "kv/model-cache/models"
	pvcEvictorMountPath      = "/mnt/kv-cache-evictor"
	pvcEvictorTemplateName   = "pvc-evictor"
	pvcEvictorNamePrefix     = "kserve-llm-isvc-kv-pvc-evictor"
	pvcEvictorServiceAccount = pvcEvictorNamePrefix
	pvcEvictorLabel          = "serving.kserve.io/pvc-evictor"
	pvcEvictorKeyLabel       = "serving.kserve.io/pvc-evictor-key"
	pvcEvictorConsumerPrefix = "serving.kserve.io/pvc-evictor-consumer-"
	pvcEvictorPVCName        = "serving.kserve.io/pvc-name"
	pvcEvictorPVCPath        = "serving.kserve.io/pvc-path"
	pvcEvictorSecurityHash   = "serving.kserve.io/pvc-security-context-hash"
	pvcEvictorContainerHash  = "serving.kserve.io/pvc-evictor-container-hash"
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

func findPVCEvictorContainerTemplate(podSpec *corev1.PodSpec) *corev1.Container {
	index := findPVCEvictorContainerTemplateIndex(podSpec)
	if index < 0 {
		return nil
	}
	return podSpec.Containers[index].DeepCopy()
}

func findPVCEvictorContainerTemplateIndex(podSpec *corev1.PodSpec) int {
	if podSpec == nil {
		return -1
	}
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == pvcEvictorTemplateName {
			return i
		}
	}
	return -1
}

func takePVCEvictorContainerTemplate(podSpec *corev1.PodSpec) *corev1.Container {
	index := findPVCEvictorContainerTemplateIndex(podSpec)
	if index < 0 {
		return nil
	}
	template := podSpec.Containers[index].DeepCopy()
	podSpec.Containers = append(podSpec.Containers[:index], podSpec.Containers[index+1:]...)
	return template
}

func workloadPVCEvictorContainerTemplate(workload *v1alpha2.WorkloadSpec) *corev1.Container {
	if workload == nil {
		return nil
	}
	if template := findPVCEvictorContainerTemplate(workload.Template); template != nil {
		return template
	}
	return findPVCEvictorContainerTemplate(workload.Worker)
}

func inheritedPVCEvictorContainerTemplate(workload *v1alpha2.WorkloadSpec, fallback *corev1.Container) *corev1.Container {
	if template := workloadPVCEvictorContainerTemplate(workload); template != nil {
		return template
	}
	if fallback == nil {
		return nil
	}
	return fallback.DeepCopy()
}

func validatePVCEvictorContainerTemplates(llmSvc *v1alpha2.LLMInferenceService) error {
	type pvcEvictorReference struct {
		name string
		path string
	}
	type workloadTemplates struct {
		template   *corev1.Container
		identities map[string]pvcEvictorReference
	}

	workloads := make([]workloadTemplates, 0, 2)
	mainTemplate := workloadPVCEvictorContainerTemplate(&llmSvc.Spec.WorkloadSpec)
	addWorkload := func(workload *v1alpha2.WorkloadSpec, template *corev1.Container) {
		if workload == nil || workload.KVCacheOffloading == nil || !workloadHasPVCTier(workload) {
			return
		}
		identities := make(map[string]pvcEvictorReference)
		for _, tier := range workload.KVCacheOffloading.Secondary {
			if tier.FileSystem == nil || tier.FileSystem.PVC == nil || tier.FileSystem.PVC.Ref == nil {
				continue
			}
			ref := tier.FileSystem.PVC.Ref
			path := normalizePVCEvictorPath(ref.Path)
			identities[pvcEvictorIdentity(ref.Name, path)] = pvcEvictorReference{name: ref.Name, path: path}
		}
		workloads = append(workloads, workloadTemplates{template: template, identities: identities})
	}
	addWorkload(&llmSvc.Spec.WorkloadSpec, mainTemplate)
	addWorkload(llmSvc.Spec.Prefill, inheritedPVCEvictorContainerTemplate(llmSvc.Spec.Prefill, mainTemplate))
	if len(workloads) == 0 {
		return nil
	}

	templatesByIdentity := make(map[string][]*corev1.Container)
	referencesByIdentity := make(map[string]pvcEvictorReference)
	for _, workload := range workloads {
		for identity, reference := range workload.identities {
			templatesByIdentity[identity] = append(templatesByIdentity[identity], workload.template)
			referencesByIdentity[identity] = reference
		}
	}
	for identity, templates := range templatesByIdentity {
		if len(templates) < 2 {
			continue
		}
		expectedHash := pvcEvictorContainerTemplateHash(templates[0])
		for _, template := range templates[1:] {
			if pvcEvictorContainerTemplateHash(template) != expectedHash {
				reference := referencesByIdentity[identity]
				return fmt.Errorf("all workloads using PVC %q and path %q must use the same pvc-evictor container template", reference.name, reference.path)
			}
		}
	}
	return nil
}

func workloadHasPVCTier(workload *v1alpha2.WorkloadSpec) bool {
	if workload == nil || workload.KVCacheOffloading == nil {
		return false
	}
	for _, tier := range workload.KVCacheOffloading.Secondary {
		if tier.FileSystem != nil && tier.FileSystem.PVC != nil {
			return true
		}
	}
	return false
}

func newPVCEvictorContainer(template *corev1.Container, name, volumeName, mountPath, subPath string, preserveTemplateMounts bool) corev1.Container {
	evictor := normalizedPVCEvictorContainer(template)
	evictor.Name = name
	evictor.Env = upsertPVCEvictorEnv(evictor.Env, mountPath)
	cacheMount := corev1.VolumeMount{Name: volumeName, MountPath: mountPath, SubPath: subPath}
	if preserveTemplateMounts {
		evictor.VolumeMounts = upsertPVCEvictorVolumeMount(evictor.VolumeMounts, cacheMount)
	} else {
		evictor.VolumeMounts = []corev1.VolumeMount{cacheMount}
	}
	return evictor
}

func normalizedPVCEvictorContainer(template *corev1.Container) corev1.Container {
	if template == nil {
		return corev1.Container{
			Image:           pvcEvictorImage,
			ImagePullPolicy: corev1.PullAlways,
			SecurityContext: pvcEvictorContainerSecurityContext(),
			Resources:       pvcEvictorResources(),
		}
	}

	evictor := *template.DeepCopy()
	if evictor.Image == "" {
		evictor.Image = pvcEvictorImage
	}
	if evictor.ImagePullPolicy == "" {
		evictor.ImagePullPolicy = corev1.PullAlways
	}
	if evictor.SecurityContext == nil {
		evictor.SecurityContext = pvcEvictorContainerSecurityContext()
	} else {
		evictor.SecurityContext = defaultPVCEvictorSecurityContext(evictor.SecurityContext)
	}
	evictor.Resources = defaultPVCEvictorResources(evictor.Resources)
	return evictor
}

func defaultPVCEvictorResources(resources corev1.ResourceRequirements) corev1.ResourceRequirements {
	if len(resources.Requests) == 0 && len(resources.Limits) == 0 && len(resources.Claims) == 0 {
		return pvcEvictorResources()
	}
	return resources
}

func defaultPVCEvictorSecurityContext(securityContext *corev1.SecurityContext) *corev1.SecurityContext {
	result := securityContext.DeepCopy()
	defaults := pvcEvictorContainerSecurityContext()
	if result.AllowPrivilegeEscalation == nil {
		result.AllowPrivilegeEscalation = defaults.AllowPrivilegeEscalation
	}
	if result.Capabilities == nil {
		result.Capabilities = defaults.Capabilities
	} else if result.Capabilities.Drop == nil {
		result.Capabilities = result.Capabilities.DeepCopy()
		result.Capabilities.Drop = defaults.Capabilities.Drop
	}
	if result.RunAsNonRoot == nil {
		result.RunAsNonRoot = defaults.RunAsNonRoot
	}
	return result
}

func upsertPVCEvictorEnv(env []corev1.EnvVar, mountPath string) []corev1.EnvVar {
	values := []struct {
		name  string
		value string
	}{
		{name: "PVC_MOUNT_PATH", value: mountPath},
		{name: "CACHE_DIRECTORY", value: pvcEvictorCacheDirectory},
	}
	for _, value := range values {
		found := false
		for i := range env {
			if env[i].Name == value.name {
				env[i].Value = value.value
				env[i].ValueFrom = nil
				found = true
				break
			}
		}
		if !found {
			env = append(env, corev1.EnvVar{Name: value.name, Value: value.value})
		}
	}
	return env
}

func upsertPVCEvictorVolumeMount(mounts []corev1.VolumeMount, mount corev1.VolumeMount) []corev1.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == mount.Name {
			mounts[i] = mount
			return mounts
		}
	}
	return append(mounts, mount)
}

func pvcEvictorContainerTemplateHash(template *corev1.Container) string {
	canonical := normalizedPVCEvictorContainer(template)
	canonical.Name = ""
	canonical.Env = slices.DeleteFunc(canonical.Env, func(variable corev1.EnvVar) bool {
		return variable.Name == "PVC_MOUNT_PATH" || variable.Name == "CACHE_DIRECTORY"
	})
	canonical.VolumeMounts = slices.DeleteFunc(canonical.VolumeMounts, func(mount corev1.VolumeMount) bool {
		return mount.Name == "kv-cache-storage"
	})

	data, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
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
				!equality.Semantic.DeepEqual(oldDeployment.Status, newDeployment.Status) ||
				!equality.Semantic.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels()) ||
				!equality.Semantic.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations()) ||
				!equality.Semantic.DeepEqual(e.ObjectOld.GetOwnerReferences(), e.ObjectNew.GetOwnerReferences())
		},
	}
}

func pvcEvictorServiceAccountPredicate() predicate.Predicate {
	isEvictor := func(object client.Object) bool {
		return object != nil && object.GetLabels()[pvcEvictorLabel] == "true"
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return isEvictor(e.Object) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return isEvictor(e.Object) },
		GenericFunc: func(e event.GenericEvent) bool { return isEvictor(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isEvictor(e.ObjectOld) || isEvictor(e.ObjectNew)
		},
	}
}

func (r *LLMISVCReconciler) enqueueOnPVCEvictorResourceChange() handler.EventHandler {
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
	containerTemplate    *corev1.Container
	requiresNodeAffinity bool
}

type pvcEvictorTemplateAgreement struct {
	agreed     bool
	unresolved bool
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
	var agreements map[string]pvcEvictorTemplateAgreement
	if len(expected) > 0 {
		agreements, err = r.desiredPVCEvictorTemplateAgreements(ctx, llmSvc, config)
		if err != nil {
			return err
		}
	}

	logger := log.FromContext(ctx)
	for _, deployment := range expected {
		matches := findPVCEvictorDeployments(current.Items, deployment.Name, deployment.Annotations[pvcEvictorPVCName], deployment.Annotations[pvcEvictorPVCPath])
		if len(matches) > 0 {
			existing := matches[0]
			if err := r.reconcileExistingPVCEvictorDeployment(ctx, agreements, existing, deployment); err != nil {
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
			if err := r.reconcileExistingPVCEvictorDeployment(ctx, agreements, existing, deployment); err != nil {
				return err
			}
			logger.Info("Shared PVC evictor Deployment was created concurrently; reusing it", "deployment", deployment.Name)
		}
	}
	markPVCEvictorReadiness(llmSvc, expected, current.Items)
	return r.cleanupUnusedPVCEvictors(ctx, llmSvc.Namespace, config, llmSvc, "")
}

func markPVCEvictorReadiness(llmSvc *v1alpha2.LLMInferenceService, expected map[string]*appsv1.Deployment, current []appsv1.Deployment) {
	if len(expected) == 0 {
		llmSvc.MarkPVCEvictorUnset()
		return
	}
	for _, deployment := range expected {
		matches := findPVCEvictorDeployments(current, deployment.Name, deployment.Annotations[pvcEvictorPVCName], deployment.Annotations[pvcEvictorPVCPath])
		if len(matches) == 0 || matches[0].Status.AvailableReplicas == 0 {
			message := fmt.Sprintf("PVC evictor Deployment %s/%s has no available replicas", deployment.Namespace, deployment.Name)
			if len(matches) > 0 {
				for _, condition := range matches[0].Status.Conditions {
					if condition.Status == corev1.ConditionFalse && condition.Message != "" {
						message = fmt.Sprintf("PVC evictor Deployment %s/%s is not ready: %s", deployment.Namespace, deployment.Name, condition.Message)
						break
					}
				}
			}
			llmSvc.MarkPVCEvictorNotReady("PVCEvictorNotReady", "%s", message)
			return
		}
	}
	llmSvc.MarkPVCEvictorReady()
}

func (r *LLMISVCReconciler) ensurePVCEvictorServiceAccount(ctx context.Context, namespace string) error {
	serviceAccount := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcEvictorServiceAccount, Namespace: namespace}, serviceAccount)
	if err == nil {
		updated := serviceAccount.DeepCopy()
		r.injectImagePullSecretsFromDefaultServiceAccount(ctx, updated)
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}
		updated.Labels[pvcEvictorLabel] = "true"
		updated.AutomountServiceAccountToken = ptr.To(false)
		if !equality.Semantic.DeepEqual(serviceAccount, updated) {
			if err := r.Update(ctx, updated); err != nil {
				return fmt.Errorf("failed to repair PVC evictor ServiceAccount %s/%s: %w", namespace, pvcEvictorServiceAccount, err)
			}
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get PVC evictor ServiceAccount %s/%s: %w", namespace, pvcEvictorServiceAccount, err)
	}
	expected := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcEvictorServiceAccount,
			Namespace: namespace,
			Labels:    map[string]string{pvcEvictorLabel: "true"},
		},
		AutomountServiceAccountToken: ptr.To(false),
	}
	r.injectImagePullSecretsFromDefaultServiceAccount(ctx, expected)
	if err := r.Create(ctx, expected); err != nil && !apierrors.IsAlreadyExists(err) {
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

func (r *LLMISVCReconciler) reconcileExistingPVCEvictorDeployment(ctx context.Context, agreements map[string]pvcEvictorTemplateAgreement, current, expected *appsv1.Deployment) error {
	if !current.DeletionTimestamp.IsZero() {
		return nil
	}
	if (current.Annotations[pvcEvictorSecurityHash] != expected.Annotations[pvcEvictorSecurityHash] &&
		(current.Annotations[pvcEvictorSecurityHash] != "" || expected.Annotations[pvcEvictorSecurityHash] != "")) ||
		(current.Annotations[pvcEvictorContainerHash] != expected.Annotations[pvcEvictorContainerHash] &&
			(current.Annotations[pvcEvictorContainerHash] != "" || expected.Annotations[pvcEvictorContainerHash] != "")) {
		identity := pvcEvictorIdentity(expected.Annotations[pvcEvictorPVCName], expected.Annotations[pvcEvictorPVCPath])
		agreement := agreements[identity]
		if !agreement.agreed {
			return fmt.Errorf("shared PVC evictor Deployment %s/%s has different security or container settings; all LLMInferenceServices using PVC %q and path %q must resolve the same vLLM PodSecurityContext and pvc-evictor container template", current.Namespace, current.Name, expected.Annotations[pvcEvictorPVCName], expected.Annotations[pvcEvictorPVCPath])
		}
		if agreement.unresolved {
			log.FromContext(ctx).Info(
				"Preserving shared PVC evictor Deployment because another LLMInferenceService's desired template could not be resolved",
				"deployment", client.ObjectKeyFromObject(current),
				"pvcName", expected.Annotations[pvcEvictorPVCName],
				"pvcPath", expected.Annotations[pvcEvictorPVCPath],
			)
			return nil
		}
	}
	if !equality.Semantic.DeepEqual(current.Spec.Selector, expected.Spec.Selector) {
		if err := r.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete shared PVC evictor Deployment with changed selector %s/%s: %w", current.Namespace, current.Name, err)
		}
		return nil
	}
	normalizedExpected := expected.DeepCopy()
	normalizedExpected.SetResourceVersion(current.GetResourceVersion())
	if err := r.Update(ctx, normalizedExpected, client.DryRunAll); err != nil {
		return fmt.Errorf("failed to get defaults for shared PVC evictor Deployment %s/%s: %w", current.Namespace, current.Name, err)
	}
	expectedOwnerReferences := retainExternalOwnerReferences(current.OwnerReferences)
	if equality.Semantic.DeepEqual(current.Spec, normalizedExpected.Spec) &&
		managedPVCEvictorMetadataEqual(current.Labels, expected.Labels) &&
		managedPVCEvictorAnnotationsEqual(current.Annotations, expected.Annotations) &&
		ownerReferencesEqual(current.OwnerReferences, expectedOwnerReferences) {
		return nil
	}

	updated := current.DeepCopy()
	updated.Spec = *normalizedExpected.Spec.DeepCopy()
	updated.Labels = maps.Clone(current.Labels)
	maps.Copy(updated.Labels, expected.Labels)
	updated.Annotations = syncedPVCEvictorAnnotations(current.Annotations, expected.Annotations)
	updated.OwnerReferences = expectedOwnerReferences
	if err := r.Update(ctx, updated); err != nil {
		return fmt.Errorf("failed to update shared PVC evictor Deployment %s/%s: %w", current.Namespace, current.Name, err)
	}
	return nil
}

func (r *LLMISVCReconciler) desiredPVCEvictorTemplateAgreements(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, config *Config) (map[string]pvcEvictorTemplateAgreement, error) {
	services := &v1alpha2.LLMInferenceServiceList{}
	if err := r.List(ctx, services, client.InNamespace(llmSvc.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list LLMInferenceServices for PVC evictor security settings: %w", err)
	}

	type templateHashes struct {
		securityHash  string
		containerHash string
	}
	hashesByIdentity := make(map[string]map[templateHashes]struct{})
	logger := log.FromContext(ctx)
	unresolved := false
	for i := range services.Items {
		service := services.Items[i].DeepCopy()
		if !service.DeletionTimestamp.IsZero() || utils.GetForceStopRuntime(service) {
			continue
		}
		if service.Name == llmSvc.Name {
			service = llmSvc.DeepCopy()
		} else {
			combined, err := r.combineBaseRefsConfig(ctx, service, config)
			if err != nil {
				logger.Error(err, "Could not resolve PVC evictor templates; preserving existing shared evictors", "llmSvc", client.ObjectKeyFromObject(service))
				unresolved = true
				continue
			}
			service.Spec = combined.Config.Spec
		}
		for _, target := range collectPVCEvictorTargets(service) {
			identity := pvcEvictorIdentity(target.claimName, target.subPath)
			if hashesByIdentity[identity] == nil {
				hashesByIdentity[identity] = make(map[templateHashes]struct{})
			}
			hashesByIdentity[identity][templateHashes{
				securityHash:  pvcEvictorSecurityContextHash(target.securityContext),
				containerHash: pvcEvictorContainerTemplateHash(target.containerTemplate),
			}] = struct{}{}
		}
	}
	agreements := make(map[string]pvcEvictorTemplateAgreement, len(hashesByIdentity))
	for identity, hashes := range hashesByIdentity {
		agreements[identity] = pvcEvictorTemplateAgreement{
			agreed:     len(hashes) == 1,
			unresolved: unresolved,
		}
	}
	return agreements, nil
}

func ownerReferencesEqual(current, expected []metav1.OwnerReference) bool {
	if len(current) == 0 && len(expected) == 0 {
		return true
	}
	return equality.Semantic.DeepEqual(current, expected)
}

func managedPVCEvictorMetadataEqual(current, expected map[string]string) bool {
	for key, value := range expected {
		if current[key] != value {
			return false
		}
	}
	return true
}

func managedPVCEvictorAnnotationsEqual(current, expected map[string]string) bool {
	if !managedPVCEvictorMetadataEqual(current, expected) {
		return false
	}
	for key := range current {
		if !isManagedPVCEvictorAnnotation(key) {
			continue
		}
		if _, exists := expected[key]; !exists {
			return false
		}
	}
	return true
}

func syncedPVCEvictorAnnotations(current, expected map[string]string) map[string]string {
	result := maps.Clone(current)
	if result == nil {
		result = make(map[string]string)
	}
	for key := range result {
		if isManagedPVCEvictorAnnotation(key) {
			if _, exists := expected[key]; !exists {
				delete(result, key)
			}
		}
	}
	maps.Copy(result, expected)
	return result
}

func isManagedPVCEvictorAnnotation(key string) bool {
	return key == pvcEvictorPVCName ||
		key == pvcEvictorPVCPath ||
		key == pvcEvictorSecurityHash ||
		key == pvcEvictorContainerHash
}

func retainExternalOwnerReferences(ownerReferences []metav1.OwnerReference) []metav1.OwnerReference {
	var retained []metav1.OwnerReference
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
		if r.APIReader == nil {
			return nil, errors.New("PVC API reader is not configured")
		}
		if err := r.APIReader.Get(ctx, types.NamespacedName{Name: targets[i].claimName, Namespace: llmSvc.Namespace}, pvc); err != nil {
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
		if tier.FileSystem == nil || tier.FileSystem.PVC == nil || tier.FileSystem.PVC.Ref == nil {
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
		if !service.DeletionTimestamp.IsZero() || service.Name == excludedService || utils.GetForceStopRuntime(service) {
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
	mainTemplate := workloadPVCEvictorContainerTemplate(&llmSvc.Spec.WorkloadSpec)
	add := func(workload *v1alpha2.WorkloadSpec, template *corev1.Container) {
		if workload == nil || workload.KVCacheOffloading == nil {
			return
		}
		for _, tier := range workload.KVCacheOffloading.Secondary {
			if tier.FileSystem == nil || tier.FileSystem.PVC == nil || tier.FileSystem.PVC.Ref == nil {
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
				claimName:         ref.Name,
				subPath:           subPath,
				securityContext:   workloadPVCEvictorSecurityContext(workload),
				containerTemplate: template,
			})
		}
	}

	add(&llmSvc.Spec.WorkloadSpec, mainTemplate)
	add(llmSvc.Spec.Prefill, inheritedPVCEvictorContainerTemplate(llmSvc.Spec.Prefill, mainTemplate))
	return targets
}

func workloadPVCEvictorSecurityContext(workload *v1alpha2.WorkloadSpec) *corev1.PodSecurityContext {
	if workload == nil {
		return nil
	}
	if workload.Template != nil && workload.Template.SecurityContext != nil {
		return workload.Template.SecurityContext.DeepCopy()
	}
	if workload.Worker != nil && workload.Worker.SecurityContext != nil {
		return workload.Worker.SecurityContext.DeepCopy()
	}
	return nil
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
	name := kmeta.ChildName(pvcEvictorNamePrefix, "-"+key)
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
		Containers: []corev1.Container{
			newPVCEvictorContainer(target.containerTemplate, "pvc-evictor", "kv-cache-storage", pvcEvictorMountPath, target.subPath, false),
		},
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
	if containerHash := pvcEvictorContainerTemplateHash(target.containerTemplate); containerHash != "" {
		annotations[pvcEvictorContainerHash] = containerHash
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
func attachPVCEvictor(podSpec *corev1.PodSpec, volumeName, mountPath, subPath string, template *corev1.Container) {
	name := "pvc-evictor-" + volumeName
	evictor := newPVCEvictorContainer(template, name, volumeName, mountPath, subPath, true)

	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			podSpec.Containers[i] = evictor
			return
		}
	}
	podSpec.Containers = append(podSpec.Containers, evictor)
}

func kvCacheSecondaryRootDir(_ int, tier v1alpha2.SecondaryTierSpec, rootDir string) string {
	if tier.FileSystem != nil && tier.FileSystem.PVC != nil {
		return path.Join(rootDir, pvcEvictorCacheDirectory)
	}
	return rootDir
}
