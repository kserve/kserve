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

package reconcilers

import (
	"context"
	stderrors "errors"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachecommon"
)

// KernelCacheReconciler reconciles KernelCache resources (namespace-scoped)
type KernelCacheReconciler struct {
	client.Client
	Clientset kubernetes.Interface
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// Reconcile
// Step 1 - Handle deletion with finalizer
// Step 2 - Create Download PVC (RWX)
// Step 3 - Create ONE extraction Job per cache (RWX pattern)
// Step 4 - Get nodes where agent pods are running
// Step 5 - Aggregate status from KernelCacheNodes
// Step 6 - Create Serving PVC when extraction complete
func (r *KernelCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	kc := &v1alpha1.KernelCache{}
	if err := r.Get(ctx, req.NamespacedName, kc); err != nil {
		if errors.IsNotFound(err) {
			// Resource deleted between watch trigger and reconcile - normal during deletion
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	r.Log.Info("Reconciling KernelCache", "name", req.Name, "namespace", req.Namespace)

	// Load config from inferenceservice-config ConfigMap (once at top)
	kernelCacheConfig, err := kernelcachecommon.LoadKernelCacheConfig(ctx, r.Clientset)
	if err != nil {
		r.Log.Error(err, "unable to load kernel cache config", "name", constants.InferenceServiceConfigMapName)
		return ctrl.Result{}, err
	}

	// Early return if KernelCache feature disabled
	if !kernelCacheConfig.Enabled {
		r.Log.Info("KernelCache feature disabled in config, skipping reconciliation")
		return reconcile.Result{}, nil
	}

	// Step 1: Handle deletion
	if !kc.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, kc, kernelCacheConfig)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(kc, kernelcachecommon.KernelCacheFinalizerName) {
		controllerutil.AddFinalizer(kc, kernelcachecommon.KernelCacheFinalizerName)
		if err := r.Update(ctx, kc); err != nil {
			if errors.IsConflict(err) {
				r.Log.Info("Finalizer add conflict (will retry)",
					"cache", kc.Name, "namespace", kc.Namespace)
			} else {
				r.Log.Error(err, "Failed to add finalizer",
					"cache", kc.Name, "namespace", kc.Namespace)
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Ensure ResolvedDigest is copied from annotation to Status (immutability)
	// This prevents annotation tampering attacks - Status is RBAC-protected subresource
	if err := r.ensureResolvedDigest(ctx, kc); err != nil {
		return ctrl.Result{}, err
	}

	jobNamespace := kernelCacheConfig.JobNamespace

	// Step 2: Create Download PVC (RWX)
	if err := r.ensureDownloadPVC(ctx, kc, jobNamespace); err != nil {
		r.Log.Error(err, "failed to create download PVC")
		return ctrl.Result{}, err
	}

	// Step 3: Create ONE extraction Job per cache (RWX pattern)
	// Operator creates the Job, agent monitors availability
	if err := r.ensureExtractionJob(ctx, kc, kernelCacheConfig); err != nil {
		r.Log.Error(err, "failed to ensure extraction job")
		return ctrl.Result{}, err
	}

	// Step 4: Get nodes where agent pods are running
	// This automatically respects DaemonSet scheduling (taints, labels, node selectors)
	agentPods := &corev1.PodList{}
	if err := r.List(ctx, agentPods,
		client.InNamespace(jobNamespace),
		client.MatchingLabels{"app": "kserve-kernelcachenode-agent"},
	); err != nil {
		return ctrl.Result{}, err
	}

	// Extract unique node names from running agent pods
	agentNodes := make(map[string]bool)
	for _, pod := range agentPods.Items {
		if pod.Spec.NodeName != "" {
			agentNodes[pod.Spec.NodeName] = true
		}
	}

	// Step 5: Aggregate status from KernelCacheNodes
	if err := r.updateAggregateStatus(ctx, kc); err != nil {
		return ctrl.Result{}, err
	}

	// Step 6: Create Serving PVC when extraction complete
	if r.extractionComplete(ctx, kc, jobNamespace) {
		if err := r.ensureServingPVC(ctx, kc); err != nil {
			r.Log.Error(err, "failed to create serving PVC")
			return ctrl.Result{}, err
		}
	}

	// No periodic requeue needed - all operations synchronous
	// Watches on Job status changes trigger reconciliation
	return ctrl.Result{}, nil
}

// handleDeletion: Finalizer cleanup after webhook validation
// ValidateDelete webhook blocks deletion if pods still using cache
// Finalizer handles cleanup of extraction resources (Job, PVC)
func (r *KernelCacheReconciler) handleDeletion(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	kernelCacheConfig *v1beta1.KernelCacheConfig,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(kc, kernelcachecommon.KernelCacheFinalizerName) {
		return ctrl.Result{}, nil
	}

	r.Log.Info("Deleting KernelCache", "name", kc.Name, "namespace", kc.Namespace)

	jobNamespace := kernelCacheConfig.JobNamespace

	// Agent owns KernelCacheNode writes and Agent will automatically remove
	// this cache when it discovers KernelCache deletion

	// Delete extraction Jobs, then Download and Serving PVCs and PVs
	downloadPVCName := kc.Namespace + "-" + kc.Name + "-download"
	downloadPVName := kc.Namespace + "-" + kc.Name + "-download-pv"
	servingPVCName := kc.Name // Same name as KernelCache
	servingPVName := kc.Namespace + "-" + kc.Name + "-serving-pv"

	// Delete extraction Jobs first (PVC can't delete while Pods are using it)
	// Job deletion automatically deletes Pods via propagation policy
	jobLabels := map[string]string{
		"cache":           kc.Name,
		"cache-namespace": kc.Namespace,
	}

	// Delete Jobs (Pods auto-delete via propagation policy)
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(jobNamespace),
		client.MatchingLabels(jobLabels),
	); err == nil {
		propagationPolicy := metav1.DeletePropagationBackground
		for i := range jobList.Items {
			job := &jobList.Items[i]
			if err := r.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &propagationPolicy}); err != nil && !errors.IsNotFound(err) {
				r.Log.Error(err, "Failed to delete extraction Job", "job", job.Name)
			} else {
				r.Log.Info("Deleted extraction Job (Pods will auto-delete)", "job", job.Name)
			}
		}
	}

	// Delete Download PVC
	downloadPVC := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      downloadPVCName,
		Namespace: jobNamespace,
	}, downloadPVC); err == nil {
		if err := r.Delete(ctx, downloadPVC); err != nil && !errors.IsNotFound(err) {
			r.Log.Error(err, "Failed to delete Download PVC", "pvc", downloadPVCName)
			return ctrl.Result{}, err
		}
		r.Log.Info("Deleted Download PVC", "pvc", downloadPVCName)
	}

	// Delete Download PV
	downloadPV := &corev1.PersistentVolume{}
	if err := r.Get(ctx, types.NamespacedName{
		Name: downloadPVName,
	}, downloadPV); err == nil {
		if err := r.Delete(ctx, downloadPV); err != nil && !errors.IsNotFound(err) {
			r.Log.Error(err, "Failed to delete Download PV", "pv", downloadPVName)
			return ctrl.Result{}, err
		}
		r.Log.Info("Deleted Download PV", "pv", downloadPVName)
	}

	// Delete Serving PVC (has owner reference, should auto-delete, but clean up manually to be safe)
	servingPVC := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      servingPVCName,
		Namespace: kc.Namespace,
	}, servingPVC); err == nil {
		if err := r.Delete(ctx, servingPVC); err != nil && !errors.IsNotFound(err) {
			r.Log.Error(err, "Failed to delete Serving PVC", "pvc", servingPVCName)
			return ctrl.Result{}, err
		}
		r.Log.Info("Deleted Serving PVC", "pvc", servingPVCName)
	}

	// Delete Serving PV
	servingPV := &corev1.PersistentVolume{}
	if err := r.Get(ctx, types.NamespacedName{
		Name: servingPVName,
	}, servingPV); err == nil {
		if err := r.Delete(ctx, servingPV); err != nil && !errors.IsNotFound(err) {
			r.Log.Error(err, "Failed to delete Serving PV", "pv", servingPVName)
			return ctrl.Result{}, err
		}
		r.Log.Info("Deleted Serving PV", "pv", servingPVName)
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(kc, kernelcachecommon.KernelCacheFinalizerName)
	if err := r.Update(ctx, kc); err != nil {
		if errors.IsConflict(err) {
			r.Log.Info("Finalizer removal conflict (will retry)",
				"cache", kc.Name, "namespace", kc.Namespace)
		} else {
			r.Log.Error(err, "Failed to remove finalizer",
				"cache", kc.Name, "namespace", kc.Namespace)
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// ensureResolvedDigest copies digest from annotation to Status field (one-time, immutable)
// This prevents annotation tampering - Status is RBAC-protected subresource
func (r *KernelCacheReconciler) ensureResolvedDigest(ctx context.Context, kc *v1alpha1.KernelCache) error {
	annotationDigest := kc.Annotations[v1alpha1.AnnotationResolvedDigest]

	// First reconcile: copy annotation → Status (one-way, one-time)
	if kc.Status.ResolvedDigest == "" {
		if annotationDigest == "" {
			return fmt.Errorf("webhook must set %s annotation before reconcile", v1alpha1.AnnotationResolvedDigest)
		}
		kc.Status.ResolvedDigest = annotationDigest
		if err := r.Status().Update(ctx, kc); err != nil {
			if errors.IsConflict(err) {
				r.Log.Info("Digest status update conflict (will retry)",
					"cache", kc.Name, "namespace", kc.Namespace)
			} else {
				r.Log.Error(err, "Failed to update KernelCache digest",
					"cache", kc.Name, "namespace", kc.Namespace)
			}
			return err
		}
		return nil
	}

	// Detect tampering: annotation changed but Status is immutable
	if kc.Status.ResolvedDigest != annotationDigest {
		r.Log.Info("Annotation digest differs from Status (ignoring annotation - possible tampering)",
			"status", kc.Status.ResolvedDigest,
			"annotation", annotationDigest)
	}

	return nil
}

// ensureDownloadPVC creates Download PV and PVC in job namespace (kserve)
// Pattern from GKM pkg/common/k8s.go CreatePv/CreatePvc and LocalModel reconciler
func (r *KernelCacheReconciler) ensureDownloadPVC(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	jobNamespace string,
) error {
	// Create Download PV and PVC in job namespace (from config), NOT in KernelCache namespace
	// Job namespace is where agents run and create extraction Jobs

	// Include namespace to avoid conflicts when same name in different namespaces
	pvName := kc.Namespace + "-" + kc.Name + "-download-pv"
	pvcName := kc.Namespace + "-" + kc.Name + "-download"

	// Storage size from CRD or default
	storageSize := resource.MustParse("10Gi") // Default
	if kc.Spec.StorageSize != nil {
		storageSize = *kc.Spec.StorageSize
	}

	// Default access mode: ReadWriteMany for Phase 1 (multi-node sharing)
	accessModes := kc.Spec.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	}

	// Step 1: Create PV (for KIND - no dynamic provisioner)
	// Use labels for tracking instead of owner references (PV is cluster-scoped, KernelCache is namespace-scoped)
	pv := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				"app.kubernetes.io/name":          "kernelcache",
				"app.kubernetes.io/component":     "download",
				"kernelcache.kserve.io/cache":     kc.Name,
				"kernelcache.kserve.io/namespace": kc.Namespace,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: storageSize,
			},
			AccessModes:                   accessModes,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			VolumeMode:                    func() *corev1.PersistentVolumeMode { m := corev1.PersistentVolumeFilesystem; return &m }(),
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/kernel-caches",
					Type: func() *corev1.HostPathType { t := corev1.HostPathDirectoryOrCreate; return &t }(),
				},
			},
		},
	}

	// Set storage class if specified
	var storageClass string
	if kc.Spec.StorageClassName != nil {
		storageClass = *kc.Spec.StorageClassName
		pv.Spec.StorageClassName = storageClass
	}

	// Create PV without owner reference (pass nil - PV is cluster-scoped, can't have namespace-scoped owner)
	if err := CreatePV(ctx, r.Clientset, r.Scheme, r.Log, pv, nil); err != nil {
		return fmt.Errorf("failed to create download PV: %w", err)
	}

	// Step 2: Create PVC bound to PV
	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: jobNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":          "kernelcache",
				"app.kubernetes.io/component":     "download",
				"kernelcache.kserve.io/cache":     kc.Name,
				"kernelcache.kserve.io/namespace": kc.Namespace,
				"kernelcache.kserve.io/pv":        pvName,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
			VolumeName: pvName, // Bind to specific PV
		},
	}

	// StorageClass must match PV, or PVC won't bind
	// Empty string prevents Kubernetes from auto-filling default StorageClass
	if storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	} else {
		// Manual PV binding requires explicit empty storageClassName
		emptyClass := ""
		pvc.Spec.StorageClassName = &emptyClass
	}

	// Cannot set owner reference - PVC in kserve namespace, KernelCache in different namespace
	// Cleanup will be manual or via labels/finalizers
	if err := CreatePVC(ctx, r.Clientset, r.Scheme, r.Log, pvc, jobNamespace, nil); err != nil {
		return fmt.Errorf("failed to create download PVC: %w", err)
	}

	return nil
}

// ensureExtractionJob creates ONE extraction Job per cache (RWX pattern)
// Job runs on any node and writes to RWX Download PVC
// Storage backend distributes data to all nodes
func (r *KernelCacheReconciler) ensureExtractionJob(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	config *v1beta1.KernelCacheConfig,
) error {
	jobNamespace := config.JobNamespace
	pvcName := kc.Namespace + "-" + kc.Name + "-download"

	// Check if Job already exists
	job, err := r.getExtractionJob(ctx, kc, jobNamespace)
	if err != nil {
		return err
	}

	// Only create if no Job exists AND extraction not yet complete
	// (TTL controller may delete completed Jobs, so check state)
	if job == nil {
		if kc.Status.State == v1alpha1.CacheStateExtracted {
			// Extraction already succeeded, Job was deleted by TTL controller
			return nil
		}
		return r.createExtractionJob(ctx, kc, pvcName, config)
	} else if r.jobFailed(job) {
		// Only recreate if failed job is old enough
		age := time.Since(job.CreationTimestamp.Time)
		if age > 5*time.Minute {
			return r.createExtractionJob(ctx, kc, pvcName, config)
		}
	}

	return nil
}

// getExtractionJob retrieves the extraction Job for this cache
func (r *KernelCacheReconciler) getExtractionJob(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	jobNamespace string,
) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	labels := map[string]string{
		"cache":           kc.Name,
		"cache-namespace": kc.Namespace,
		"app":             "kernel-cache-extract",
	}

	if err := r.List(ctx, jobList,
		client.InNamespace(jobNamespace),
		client.MatchingLabels(labels),
	); err != nil {
		return nil, err
	}

	if len(jobList.Items) == 0 {
		return nil, nil
	}

	// Return most recent
	latest := &jobList.Items[0]
	for i := range jobList.Items {
		if jobList.Items[i].CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = &jobList.Items[i]
		}
	}

	return latest, nil
}

// createExtractionJob creates extraction Job
func (r *KernelCacheReconciler) createExtractionJob(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	pvcName string,
	config *v1beta1.KernelCacheConfig,
) error {
	jobNamespace := config.JobNamespace

	// Double-check no job exists
	existingJob, err := r.getExtractionJob(ctx, kc, jobNamespace)
	if err != nil {
		return err
	}
	if existingJob != nil && !r.jobFailed(existingJob) {
		r.Log.Info("Job already exists, skipping creation", "cache", kc.Name)
		return nil
	}

	// Deterministic name
	jobName := fmt.Sprintf("%s-%s-extract", kc.Namespace, kc.Name)

	// Use Status field (immutable, RBAC-protected) instead of annotation
	// This prevents annotation tampering attacks
	resolvedDigest := kc.Status.ResolvedDigest
	imageWithDigest := kernelcachecommon.ReplaceUrlTag(kc.Spec.Image, resolvedDigest)
	if imageWithDigest == "" {
		err := stderrors.New("unable to update image tag with digest")
		r.Log.Error(err, "invalid image or digest", "image", kc.Spec.Image, "digest", resolvedDigest)
		return err
	}

	// Hash-based storage key for deduplication
	// Use image with digest to ensure same content = same storage key
	storageKey := v1alpha1.GetKernelCacheStorageKey(imageWithDigest)

	noGPU := "false"
	if config.NoGPU {
		noGPU = "true"
	}

	container := &corev1.Container{
		Name:  kernelcachecommon.ExtractContainerName,
		Image: config.ExtractImage,
		Env: []corev1.EnvVar{
			{Name: "GKM_CACHE_DIR", Value: kernelcachecommon.MountPath},
			{Name: "GKM_IMAGE_URL", Value: imageWithDigest}, // Use image with digest, not tag
			{Name: "GO_LOG", Value: "info"},
			{Name: "NO_GPU", Value: noGPU},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      kernelcachecommon.CachePVCMountName,
				MountPath: kernelcachecommon.MountPath,
				ReadOnly:  false,
				SubPath:   filepath.Join("kernel-cache", storageKey),
			},
		},
	}

	// Use FSGroup from config, default to 1000 if not set
	fsGroup := int64(1000)
	if config.FSGroup != nil {
		fsGroup = *config.FSGroup
	}

	var initContainers []corev1.Container
	if config.EnablePermissionInitContainer {
		var rootUser int64 = 0
		commandString := fmt.Sprintf("mkdir -p %s && chown -R %d:%d %s && chmod -R 775 %s",
			kernelcachecommon.MountPath, fsGroup, fsGroup, kernelcachecommon.MountPath, kernelcachecommon.MountPath)

		initContainer := corev1.Container{
			Name:  "fix-permissions",
			Image: "registry.access.redhat.com/ubi9/ubi-micro:latest",
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &rootUser,
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      kernelcachecommon.CachePVCMountName,
					MountPath: kernelcachecommon.MountPath,
					ReadOnly:  false,
					SubPath:   filepath.Join("kernel-cache", storageKey), // Same SubPath as extraction container
				},
			},
			Command: []string{"/bin/sh"},
			Args:    []string{"-c", commandString},
		}
		initContainers = []corev1.Container{initContainer}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: jobNamespace,
			Labels: map[string]string{
				"app":             "kernel-cache-extract",
				"cache":           kc.Name,
				"cache-namespace": kc.Namespace,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: config.JobTTLSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":             "kernel-cache-extract",
						"cache":           kc.Name,
						"cache-namespace": kc.Namespace,
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: initContainers,
					Containers:     []corev1.Container{*container},
					RestartPolicy:  corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: kernelcachecommon.CachePVCMountName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup:    &fsGroup,
						RunAsUser:  &fsGroup,
						RunAsGroup: &fsGroup,
					},
				},
			},
		},
	}

	r.Log.Info("Creating extraction job", "job", jobName, "cache", kc.Name)
	return r.Create(ctx, job)
}

// jobFailed checks if Job has failed
func (r *KernelCacheReconciler) jobFailed(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// jobCompleted checks if Job has completed
func (r *KernelCacheReconciler) jobCompleted(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// extractionComplete checks if the single extraction Job has completed
func (r *KernelCacheReconciler) extractionComplete(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	jobNamespace string,
) bool {
	job, err := r.getExtractionJob(ctx, kc, jobNamespace)
	if err != nil {
		r.Log.Error(err, "failed to get extraction job")
		return false
	}

	if job == nil {
		return false
	}

	return r.jobCompleted(job)
}

// ensureServingPVC creates Serving PV and PVC in KernelCache's namespace
// Serving PV points to same HostPath as Download PV (shares disk space)
// But they are separate Kubernetes resources (PV can only bind to one PVC)
func (r *KernelCacheReconciler) ensureServingPVC(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
) error {
	// Create Serving PV and PVC in KernelCache's namespace
	// This allows ISVC pods in same namespace to mount the cache
	servingPVName := kc.Namespace + "-" + kc.Name + "-serving-pv"
	servingPVCName := kc.Name // Same name as KernelCache

	// Storage size from CRD or default
	storageSize := resource.MustParse("10Gi") // Default
	if kc.Spec.StorageSize != nil {
		storageSize = *kc.Spec.StorageSize
	}

	// Step 1: Create Serving PV (separate from Download PV)
	// Uses same HostPath but different PV resource (PV binds to one PVC only)
	servingPV := corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: servingPVName,
			Labels: map[string]string{
				"app.kubernetes.io/name":          "kernelcache",
				"app.kubernetes.io/component":     "serving",
				"kernelcache.kserve.io/cache":     kc.Name,
				"kernelcache.kserve.io/namespace": kc.Namespace,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: storageSize,
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadOnlyMany, // ReadOnly for serving pods
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			VolumeMode:                    func() *corev1.PersistentVolumeMode { m := corev1.PersistentVolumeFilesystem; return &m }(),
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/kernel-caches", // Same path as Download PV
					Type: func() *corev1.HostPathType { t := corev1.HostPathDirectoryOrCreate; return &t }(),
				},
			},
		},
	}

	// Set storage class if specified
	var storageClass string
	if kc.Spec.StorageClassName != nil {
		storageClass = *kc.Spec.StorageClassName
		servingPV.Spec.StorageClassName = storageClass
	}

	// Create Serving PV (no owner reference - cluster-scoped)
	if err := CreatePV(ctx, r.Clientset, r.Scheme, r.Log, servingPV, nil); err != nil {
		return fmt.Errorf("failed to create serving PV: %w", err)
	}

	// Step 2: Create Serving PVC in KernelCache's namespace
	servingPVC := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      servingPVCName,
			Namespace: kc.Namespace, // SAME namespace as KernelCache
			Labels: map[string]string{
				"app.kubernetes.io/name":          "kernelcache",
				"app.kubernetes.io/component":     "serving",
				"kernelcache.kserve.io/cache":     kc.Name,
				"kernelcache.kserve.io/namespace": kc.Namespace,
				"kernelcache.kserve.io/pv":        servingPVName,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadOnlyMany, // ReadOnly for serving
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
			VolumeName: servingPVName, // Bind to Serving PV
		},
	}

	// StorageClass must match PV
	if storageClass != "" {
		servingPVC.Spec.StorageClassName = &storageClass
	} else {
		emptyClass := ""
		servingPVC.Spec.StorageClassName = &emptyClass
	}

	// Create Serving PVC (CreatePVC will set owner reference - same namespace)
	if err := CreatePVC(ctx, r.Clientset, r.Scheme, r.Log, servingPVC, kc.Namespace, kc); err != nil {
		return fmt.Errorf("failed to create serving PVC: %w", err)
	}

	r.Log.Info("Created Serving PV and PVC",
		"pv", servingPVName,
		"pvc", servingPVCName,
		"namespace", kc.Namespace)

	return nil
}

// updateAggregateStatus aggregates status from all KernelCacheNodes
func (r *KernelCacheReconciler) updateAggregateStatus(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
) error {
	// List all KernelCacheNodes (cluster-scoped - no namespace filter)
	kcNodes := &v1alpha1.KernelCacheNodeList{}
	if err := r.List(ctx, kcNodes); err != nil {
		return err
	}

	// Aggregate counts for state calculation
	counts := &v1alpha1.CacheCounts{
		NodeCnt:         0,
		NodeErrorCnt:    0,
		NodeInUseCnt:    0,
		NodeNotInUseCnt: 0,
	}

	// Aggregate GPU compatibility
	compatibleTypes := make(map[string]bool)
	incompatibleTypes := make(map[string]bool)
	totalCompatibleGPUs := 0
	totalIncompatibleGPUs := 0

	// Aggregate serving status (Phase 2)
	servingStatus := &v1alpha1.ServingStatus{
		NamespaceCounts: make(map[string]v1alpha1.NamespaceServingCounts),
	}

	// Build cache key for lookup: {namespace}/{name}
	cacheKey := kc.Namespace + "/" + kc.Name

	for _, node := range kcNodes.Items {
		if cacheInfo, ok := node.Status.CacheStatus[cacheKey]; ok {
			counts.NodeCnt++

			// Count nodes by state
			switch cacheInfo.State {
			case v1alpha1.NodeCacheStateError:
				counts.NodeErrorCnt++
			case v1alpha1.NodeCacheStateRunning:
				counts.NodeInUseCnt++
			case v1alpha1.NodeCacheStateExtracted:
				counts.NodeNotInUseCnt++
			}

			// GPU compatibility aggregation
			totalCompatibleGPUs += len(cacheInfo.CompatibleGPUs)
			totalIncompatibleGPUs += len(cacheInfo.IncompatibleGPUs)

			// Find GPU types from node's GPUInfo
			for _, gpuInfo := range node.Status.GPUInfo {
				for _, id := range gpuInfo.IDs {
					isCompatible := false
					for _, compatID := range cacheInfo.CompatibleGPUs {
						if id == compatID {
							isCompatible = true
							compatibleTypes[gpuInfo.GPUType] = true
							break
						}
					}
					if !isCompatible {
						for _, incompatID := range cacheInfo.IncompatibleGPUs {
							if id == incompatID {
								incompatibleTypes[gpuInfo.GPUType] = true
								break
							}
						}
					}
				}
			}

			// Serving counts aggregation (Phase 2)
			for ns, nsCounts := range cacheInfo.ServingNamespaces {
				aggCounts := servingStatus.NamespaceCounts[ns]
				aggCounts.PodsUsing += nsCounts.PodsUsing
				aggCounts.PodsReady += nsCounts.PodsReady
				aggCounts.PodsTerminating += nsCounts.PodsTerminating
				servingStatus.NamespaceCounts[ns] = aggCounts
			}
		}
	}

	// Calculate serving totals
	for _, nsCounts := range servingStatus.NamespaceCounts {
		servingStatus.TotalPodsUsing += nsCounts.PodsUsing
		servingStatus.TotalPodsReady += nsCounts.PodsReady
		servingStatus.TotalPodsTerminating += nsCounts.PodsTerminating
	}
	servingStatus.TotalNamespaces = len(servingStatus.NamespaceCounts)

	// Build GPU compatibility summary
	gpuCompat := &v1alpha1.GPUCompatibilitySummary{
		CompatibleTypes:       make([]string, 0, len(compatibleTypes)),
		IncompatibleTypes:     make([]string, 0, len(incompatibleTypes)),
		TotalCompatibleGPUs:   totalCompatibleGPUs,
		TotalIncompatibleGPUs: totalIncompatibleGPUs,
	}
	for gpuType := range compatibleTypes {
		gpuCompat.CompatibleTypes = append(gpuCompat.CompatibleTypes, gpuType)
	}
	for gpuType := range incompatibleTypes {
		gpuCompat.IncompatibleTypes = append(gpuCompat.IncompatibleTypes, gpuType)
	}

	// Calculate overall state based on hierarchy: Error > Running > Extracted > Downloading > Pending
	state := v1alpha1.CacheStatePending
	if counts.NodeErrorCnt > 0 {
		state = v1alpha1.CacheStateError
	} else if counts.NodeInUseCnt > 0 || servingStatus.TotalPodsUsing > 0 {
		state = v1alpha1.CacheStateRunning
	} else if counts.NodeNotInUseCnt > 0 {
		state = v1alpha1.CacheStateExtracted
	} else if counts.NodeCnt > 0 {
		// Nodes exist but no extracted/running/error = downloading
		state = v1alpha1.CacheStateDownloading
	}

	// Update KernelCache status
	kc.Status.State = state
	kc.Status.Counts = counts
	kc.Status.GPUCompatibility = gpuCompat
	kc.Status.ServingStatus = servingStatus

	if err := r.Status().Update(ctx, kc); err != nil {
		if errors.IsConflict(err) {
			r.Log.Info("Status update conflict (will retry)",
				"cache", kc.Name, "namespace", kc.Namespace)
		} else {
			r.Log.Error(err, "Failed to update KernelCache status",
				"cache", kc.Name, "namespace", kc.Namespace)
		}
		return err
	}
	return nil
}

// nodeStatusMapper maps KernelCacheNode changes to KernelCache reconciliation requests
func (r *KernelCacheReconciler) nodeStatusMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	kcNode := obj.(*v1alpha1.KernelCacheNode)
	requests := make([]reconcile.Request, 0, len(kcNode.Status.CacheStatus))

	// Reconcile all caches referenced in this node
	for _, cacheInfo := range kcNode.Status.CacheStatus {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      cacheInfo.Name,
				Namespace: cacheInfo.Namespace,
			},
		})
	}

	return requests
}

// jobMapper maps Job changes to KernelCache reconciliation requests
func (r *KernelCacheReconciler) jobMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	job := obj.(*batchv1.Job)

	// Extract cache info from Job labels
	cacheName := job.Labels["cache"]
	cacheNamespace := job.Labels["cache-namespace"]

	if cacheName == "" || cacheNamespace == "" {
		return []reconcile.Request{}
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      cacheName,
				Namespace: cacheNamespace,
			},
		},
	}
}

// SetupWithManager configures event-driven watches (no polling needed for controller)
func (r *KernelCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to watch only KernelCacheNode status changes
	kernelCacheNodeStatusPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectOld.(*v1alpha1.KernelCacheNode)
			newNode := e.ObjectNew.(*v1alpha1.KernelCacheNode)
			return !reflect.DeepEqual(oldNode.Status, newNode.Status)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KernelCache{}).
		Owns(&corev1.PersistentVolumeClaim{}). // Watch PVC changes
		Watches(
			&v1alpha1.KernelCacheNode{},
			handler.EnqueueRequestsFromMapFunc(r.nodeStatusMapper),
			builder.WithPredicates(kernelCacheNodeStatusPredicate),
		).
		Watches(
			&batchv1.Job{},
			handler.EnqueueRequestsFromMapFunc(r.jobMapper),
		).
		Complete(r)
}
