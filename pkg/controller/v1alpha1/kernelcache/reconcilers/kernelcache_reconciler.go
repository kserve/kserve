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

const (
	KernelCacheFinalizerName = "kernelcache.kserve.io/finalizer"
)

// KernelCacheReconciler reconciles KernelCache resources (namespace-scoped)
type KernelCacheReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// Reconcile
// Step 1 - Handle deletion with finalizer
// Step 2 - Create Download PVC (RWX)
// Step 3 - Create ONE extraction Job per cache (RWX pattern)
// Step 4 - Get nodes where agent pods are running
// Step 5 - Ensure KernelCacheNode exists for each node
// Step 6 - Aggregate status from KernelCacheNodes
// Step 7 - Create Serving PVC when extraction complete
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

	// Step 1: Handle deletion
	if !kc.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, kc, kernelCacheConfig)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(kc, KernelCacheFinalizerName) {
		controllerutil.AddFinalizer(kc, KernelCacheFinalizerName)
		return ctrl.Result{}, r.Update(ctx, kc)
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

	// Step 5: For each node with agent, ensure KernelCacheNode exists and has this cache
	for nodeName := range agentNodes {
		if err := r.ensureKernelCacheNode(ctx, kc, nodeName); err != nil {
			r.Log.Error(err, "failed to ensure KernelCacheNode", "node", nodeName)
			continue
		}
	}

	// Step 6: Aggregate status from KernelCacheNodes
	if err := r.updateAggregateStatus(ctx, kc); err != nil {
		return ctrl.Result{}, err
	}

	// Step 7: Create Serving PVC when extraction complete
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

// handleDeletion: Phase 1 simple finalizer (no pod usage check)
// Phase 2 will add validating webhook for production safety
func (r *KernelCacheReconciler) handleDeletion(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	kernelCacheConfig *v1beta1.KernelCacheConfig,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(kc, KernelCacheFinalizerName) {
		return ctrl.Result{}, nil
	}

	r.Log.Info("Deleting KernelCache", "name", kc.Name, "namespace", kc.Namespace)

	jobNamespace := kernelCacheConfig.JobNamespace

	// Remove cache from all KernelCacheNodes (cluster-scoped)
	// Owner references on PVC ensure automatic cleanup
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		return ctrl.Result{}, err
	}

	for _, node := range nodes.Items {
		kcNode := &v1alpha1.KernelCacheNode{}
		kcNodeName := "kernel-cache-node-" + node.Name

		if err := r.Get(ctx, types.NamespacedName{
			Name: kcNodeName,
		}, kcNode); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return ctrl.Result{}, err
		}

		// Remove this cache from node's status
		updated := false
		newCaches := []v1alpha1.KernelCacheInfo{}
		for _, cache := range kcNode.Status.Caches {
			if cache.Name != kc.Name || cache.Namespace != kc.Namespace {
				newCaches = append(newCaches, cache)
			} else {
				updated = true
			}
		}

		if updated {
			kcNode.Status.Caches = newCaches
			if err := r.Status().Update(ctx, kcNode); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Delete extraction Jobs, then Download and Serving PVCs and PVs
	downloadPVCName := kc.Namespace + "-" + kc.Name + "-download"
	downloadPVName := kc.Namespace + "-" + kc.Name + "-download-pv"
	servingPVCName := kc.Name // Same name as KernelCache
	servingPVName := kc.Namespace + "-" + kc.Name + "-serving-pv"

	// Delete extraction Jobs and Pods first (PVC can't delete while Pods are using it)
	jobLabels := map[string]string{
		"cache":           kc.Name,
		"cache-namespace": kc.Namespace,
	}

	// Delete Pods (Job deletion doesn't immediately delete Pods)
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(jobNamespace),
		client.MatchingLabels(jobLabels),
	); err == nil {
		for i := range podList.Items {
			pod := &podList.Items[i]
			if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
				r.Log.Error(err, "Failed to delete extraction Pod", "pod", pod.Name)
			} else {
				r.Log.Info("Deleted extraction Pod", "pod", pod.Name)
			}
		}
	}

	// Delete Jobs
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList,
		client.InNamespace(jobNamespace),
		client.MatchingLabels(jobLabels),
	); err == nil {
		for i := range jobList.Items {
			job := &jobList.Items[i]
			if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
				r.Log.Error(err, "Failed to delete extraction Job", "job", job.Name)
			} else {
				r.Log.Info("Deleted extraction Job", "job", job.Name)
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
	controllerutil.RemoveFinalizer(kc, KernelCacheFinalizerName)
	return ctrl.Result{}, r.Update(ctx, kc)
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
		return r.Status().Update(ctx, kc)
	}

	// Detect tampering: annotation changed but Status is immutable
	if kc.Status.ResolvedDigest != annotationDigest {
		r.Log.Info("Annotation digest differs from Status (ignoring annotation - possible tampering)",
			"status", kc.Status.ResolvedDigest,
			"annotation", annotationDigest)
	}

	return nil
}

// ensureKernelCacheNode creates or updates KernelCacheNode for a specific node
// KernelCacheNodes are cluster-scoped (like LocalModelNode), one per physical node
// They track caches from ALL namespaces in their Status.Caches array
func (r *KernelCacheReconciler) ensureKernelCacheNode(
	ctx context.Context,
	kc *v1alpha1.KernelCache,
	nodeName string,
) error {
	// KernelCacheNode is cluster-scoped (no namespace)
	kcNode := &v1alpha1.KernelCacheNode{}
	kcNodeName := "kernel-cache-node-" + nodeName

	err := r.Get(ctx, types.NamespacedName{
		Name: kcNodeName,
	}, kcNode)

	if errors.IsNotFound(err) {
		// Create new KernelCacheNode (two-step: create resource, then update status)
		// Cluster-scoped - no namespace
		kcNode = &v1alpha1.KernelCacheNode{
			ObjectMeta: metav1.ObjectMeta{
				Name: kcNodeName,
			},
		}

		// Step 1: Create the resource
		if err := r.Create(ctx, kcNode); err != nil {
			return err
		}

		// Step 2: Update status subresource
		kcNode.Status = v1alpha1.KernelCacheNodeStatus{
			NodeName: nodeName,
			Caches: []v1alpha1.KernelCacheInfo{
				{
					Name:      kc.Name,
					Namespace: kc.Namespace,
					Image:     kc.Spec.Image,
					Digest:    kc.Status.ResolvedDigest,
				},
			},
		}

		// Note: GPU info populated by agent (not operator)
		// Agent calls populateGPUInfo() via MCV GetGpuList() when it first sees KernelCacheNode
		// Operator just creates the skeleton, agent fills in hardware details

		return r.Status().Update(ctx, kcNode)
	}

	if err != nil {
		return err
	}

	// Update KernelCacheNode if cache not present or digest changed
	cacheFound := false
	for i, cache := range kcNode.Status.Caches {
		if cache.Name == kc.Name && cache.Namespace == kc.Namespace {
			cacheFound = true
			if cache.Digest != kc.Status.ResolvedDigest || cache.Image != kc.Spec.Image {
				kcNode.Status.Caches[i].Image = kc.Spec.Image
				kcNode.Status.Caches[i].Digest = kc.Status.ResolvedDigest
				return r.Status().Update(ctx, kcNode)
			}
		}
	}

	if !cacheFound {
		kcNode.Status.Caches = append(kcNode.Status.Caches, v1alpha1.KernelCacheInfo{
			Name:      kc.Name,
			Namespace: kc.Namespace,
			Image:     kc.Spec.Image,
			Digest:    kc.Status.ResolvedDigest,
		})
		return r.Status().Update(ctx, kcNode)
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
	// Pattern from GKM CreatePv (pkg/common/k8s.go:205-288)
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
	// Pattern from GKM CreatePvc (pkg/common/k8s.go:529-608)
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

	// Only create if no Job exists OR Job failed
	if job == nil {
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

	var fsGroup int64 = 1000
	var initContainers []corev1.Container
	if config.EnablePermissionInitContainer {
		var rootUser int64 = 0
		commandString := "mkdir -p " + kernelcachecommon.MountPath +
			" && chown -R 1000:1000 " + kernelcachecommon.MountPath +
			" && chmod -R 775 " + kernelcachecommon.MountPath

		initContainer := corev1.Container{
			Name:  "fix-permissions",
			Image: "quay.io/fedora/fedora-minimal",
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

	// Aggregate download status
	counts := &v1alpha1.CacheCopies{Total: 0, Available: 0, Failed: 0, InProgress: 0}

	// Aggregate GPU compatibility
	compatibleTypes := make(map[string]bool)
	incompatibleTypes := make(map[string]bool)
	totalCompatibleGPUs := 0
	totalIncompatibleGPUs := 0

	// Aggregate serving status (Phase 2)
	servingStatus := &v1alpha1.ServingStatus{
		NamespaceCounts: make(map[string]v1alpha1.NamespaceServingCounts),
	}

	for _, node := range kcNodes.Items {
		if cacheStatus, ok := node.Status.CacheStatus[kc.Name]; ok {
			// Download counts
			counts.Total++

			switch cacheStatus.DownloadStatus {
			case v1alpha1.NodeExtractionCompleted:
				counts.Available++
			case v1alpha1.NodeExtractionFailed:
				counts.Failed++
			case v1alpha1.NodeExtractionInProgress:
				counts.InProgress++
			}

			// GPU compatibility aggregation
			totalCompatibleGPUs += len(cacheStatus.CompatibleGPUs)
			totalIncompatibleGPUs += len(cacheStatus.IncompatibleGPUs)

			// Find GPU types from node's GPUInfo
			for _, gpuInfo := range node.Status.GPUInfo {
				for _, id := range gpuInfo.IDs {
					isCompatible := false
					for _, compatID := range cacheStatus.CompatibleGPUs {
						if id == compatID {
							isCompatible = true
							compatibleTypes[gpuInfo.GPUType] = true
							break
						}
					}
					if !isCompatible {
						for _, incompatID := range cacheStatus.IncompatibleGPUs {
							if id == incompatID {
								incompatibleTypes[gpuInfo.GPUType] = true
								break
							}
						}
					}
				}
			}

			// Serving counts aggregation (Phase 2)
			for ns, nsCounts := range cacheStatus.ServingNamespaces {
				aggCounts := servingStatus.NamespaceCounts[ns]
				aggCounts.PodsUsing += nsCounts.PodsUsing
				aggCounts.PodsReady += nsCounts.PodsReady
				aggCounts.PodsTerminating += nsCounts.PodsTerminating
				servingStatus.NamespaceCounts[ns] = aggCounts
			}
		}
	}

	// Calculate serving totals
	for _, counts := range servingStatus.NamespaceCounts {
		servingStatus.TotalPods += counts.PodsUsing
		servingStatus.TotalPodsReady += counts.PodsReady
		servingStatus.TotalPodsTerminating += counts.PodsTerminating
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

	// Update KernelCache status
	kc.Status.CacheCopies = counts
	kc.Status.GPUCompatibility = gpuCompat
	kc.Status.ServingStatus = servingStatus

	return r.Status().Update(ctx, kc)
}

// nodeStatusMapper maps KernelCacheNode changes to KernelCache reconciliation requests
func (r *KernelCacheReconciler) nodeStatusMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	kcNode := obj.(*v1alpha1.KernelCacheNode)
	requests := make([]reconcile.Request, 0, len(kcNode.Status.Caches))

	// Reconcile all caches referenced in this node
	for _, cacheInfo := range kcNode.Status.Caches {
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
		Owns(&corev1.PersistentVolume{}).      // Watch PV changes
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
