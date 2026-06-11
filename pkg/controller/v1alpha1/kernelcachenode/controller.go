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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcaches,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
package kernelcachenode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/kernelcachecommon"
)

var (
	nodeName         = os.Getenv("NODE_NAME")
	cachesRootFolder = filepath.Join(kernelcachecommon.MountPath, "kernel-cache")
)

type KernelCacheNodeReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
	fsHelper  FileSystemInterface
}

// InitializeFileSystemHelper initializes the filesystem helper and ensures cache root exists
func (r *KernelCacheNodeReconciler) InitializeFileSystemHelper() error {
	r.fsHelper = NewFileSystemHelper(cachesRootFolder)
	if err := r.fsHelper.ensureCacheRootFolderExists(); err != nil {
		return fmt.Errorf("failed to create cache root folder %s: %w", cachesRootFolder, err)
	}
	return nil
}

// EnsureKernelCacheNode creates the KernelCacheNode CR if it doesn't exist
// Agent owns KernelCacheNode creation - operator never creates it
func (r *KernelCacheNodeReconciler) EnsureKernelCacheNode(cfg *rest.Config) error {
	kcNodeName := nodeName

	// Create client - can't use r.Client as manager hasn't started yet
	c, err := client.New(cfg, client.Options{Scheme: r.Scheme})
	if err != nil {
		return err
	}

	// Check if KernelCacheNode already exists
	kcNode := &v1alpha1.KernelCacheNode{}
	err = c.Get(context.Background(), types.NamespacedName{Name: kcNodeName}, kcNode)

	if err == nil {
		// Already exists
		r.Log.Info("KernelCacheNode already exists", "name", kcNodeName, "node", nodeName)
		return nil
	}

	if !errors.IsNotFound(err) {
		return err
	}

	// Create new KernelCacheNode
	r.Log.Info("Creating KernelCacheNode", "name", kcNodeName, "node", nodeName)

	kcNode = &v1alpha1.KernelCacheNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: kcNodeName,
		},
	}

	// Create the resource
	if err := c.Create(context.Background(), kcNode); err != nil {
		return err
	}

	// Update status with NodeName
	kcNode.Status = v1alpha1.KernelCacheNodeStatus{
		NodeName:    nodeName,
		CacheStatus: make(map[string]v1alpha1.CacheNodeCacheInfo),
	}

	if err := c.Status().Update(context.Background(), kcNode); err != nil {
		if strings.Contains(err.Error(), "object has been modified") {
			r.Log.Info("Initial status update conflict (will retry)",
				"name", kcNodeName, "node", nodeName)
		} else {
			r.Log.Error(err, "Failed to create KernelCacheNode status",
				"name", kcNodeName, "node", nodeName)
		}
		return err
	}

	r.Log.Info("Created KernelCacheNode", "name", kcNodeName, "node", nodeName)
	return nil
}

// Reconcile implements controller-runtime Reconciler
func (r *KernelCacheNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	kcNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, req.NamespacedName, kcNode); err != nil {
		if errors.IsNotFound(err) {
			// Resource deleted between watch trigger and reconcile - normal during deletion
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	r.Log.Info("Reconciling KernelCacheNode", "name", req.Name, "namespace", req.Namespace)

	// Load config from inferenceservice-config ConfigMap
	kernelCacheConfig, err := kernelcachecommon.LoadKernelCacheConfig(ctx, r.Clientset)
	if err != nil {
		r.Log.Error(err, "unable to load kernel cache config", "name", constants.InferenceServiceConfigMapName)
		return ctrl.Result{}, err
	}

	reconcileInterval := time.Duration(*kernelCacheConfig.ReconcileIntervalSeconds) * time.Second

	// Populate GPU info if not present (MCV detection or stub for KIND)
	// This runs once per node - subsequent reconciles skip if GPUInfo already populated
	gpuInfoChanged := false
	if len(kcNode.Status.GPUInfo) == 0 {
		if err := r.populateGPUInfo(kcNode, kernelCacheConfig.NoGPU); err != nil {
			r.Log.Error(err, "failed to populate GPU info", "node", kcNode.Status.NodeName)
			// Non-fatal - continue with reconciliation
		} else if len(kcNode.Status.GPUInfo) > 0 {
			gpuInfoChanged = true
			r.Log.Info("GPU info populated", "node", kcNode.Status.NodeName, "gpuTypes", len(kcNode.Status.GPUInfo))
		}
	}

	// Discover caches by watching KernelCache CRs
	if err := r.discoverCaches(ctx, kcNode); err != nil {
		r.Log.Error(err, "failed to discover caches")
		return ctrl.Result{}, err
	}

	// Process each cache - check extraction Job status and update state
	for cacheKey, cacheInfo := range kcNode.Status.CacheStatus {
		if err := r.checkCacheAvailability(ctx, kcNode, cacheKey, cacheInfo, kernelCacheConfig); err != nil {
			r.Log.Error(err, "failed to check cache availability", "cache", cacheKey)
			// Continue processing other caches
		}
	}

	// Cleanup cache directories no longer in CacheStatus (run before updateStatus to ensure it always runs)
	if err := r.deleteCaches(kcNode); err != nil {
		r.Log.Error(err, "failed to delete orphaned cache directories")
		// Non-fatal - continue with reconciliation
	}

	// Update status (agent owns all KernelCacheNode writes)
	if err := r.updateStatus(ctx, kcNode, kernelCacheConfig, gpuInfoChanged); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: reconcileInterval}, nil
}

// discoverCaches lists all KernelCache CRs and populates CacheStatus map
// Agent owns cache discovery
func (r *KernelCacheNodeReconciler) discoverCaches(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
) error {
	// List all KernelCache CRs across all namespaces
	kcList := &v1alpha1.KernelCacheList{}
	if err := r.List(ctx, kcList); err != nil {
		return err
	}

	// Initialize CacheStatus map if needed
	if kcNode.Status.CacheStatus == nil {
		kcNode.Status.CacheStatus = make(map[string]v1alpha1.CacheNodeCacheInfo)
	}

	// Track which caches exist (for cleanup of deleted caches)
	activeCaches := make(map[string]bool)

	// Add/update cache entries from KernelCache CRs
	for _, kc := range kcList.Items {
		// Cache key: {namespace}/{name} for uniqueness
		cacheKey := kc.Namespace + "/" + kc.Name

		activeCaches[cacheKey] = true

		// Get existing entry or create new
		cacheInfo, exists := kcNode.Status.CacheStatus[cacheKey]
		if !exists {
			cacheInfo = v1alpha1.CacheNodeCacheInfo{
				State: v1alpha1.NodeCacheStatePending,
			}
		}

		// Update cache identity from KernelCache CR
		cacheInfo.Name = kc.Name
		cacheInfo.Namespace = kc.Namespace
		cacheInfo.Image = kc.Spec.Image
		cacheInfo.Digest = kc.Status.ResolvedDigest

		kcNode.Status.CacheStatus[cacheKey] = cacheInfo
	}

	// Remove caches that no longer exist
	for cacheKey := range kcNode.Status.CacheStatus {
		if !activeCaches[cacheKey] {
			delete(kcNode.Status.CacheStatus, cacheKey)
			r.Log.Info("Removed deleted cache", "cache", cacheKey, "node", kcNode.Status.NodeName)
		}
	}

	return nil
}

// deleteCaches removes cache directories from filesystem that are no longer in CacheStatus
// Follows LocalModelNode pattern - compares filesystem with CacheStatus map and removes orphaned directories
func (r *KernelCacheNodeReconciler) deleteCaches(kcNode *v1alpha1.KernelCacheNode) error {
	r.Log.V(1).Info("Running cache cleanup", "node", kcNode.Status.NodeName, "cachesRootFolder", cachesRootFolder)

	// 1. Scan cache directory and get list of existing folders (storage keys)
	foldersToRemove := make(map[string]struct{})
	entries, err := r.fsHelper.getCacheFolders()
	if err != nil {
		r.Log.Error(err, "Failed to list cache folders", "path", cachesRootFolder)
		return err
	}

	r.Log.V(1).Info("Found cache folders on filesystem", "count", len(entries), "node", kcNode.Status.NodeName)

	for _, entry := range entries {
		// Caches exist in subdirectories (storage key = hash of image URI)
		if entry.IsDir() {
			foldersToRemove[entry.Name()] = struct{}{}
			r.Log.V(1).Info("Filesystem folder candidate for removal", "storageKey", entry.Name())
		}
	}

	// 2. Compare with caches in CacheStatus map using storage keys
	r.Log.V(1).Info("Checking CacheStatus for active caches", "cacheCount", len(kcNode.Status.CacheStatus), "node", kcNode.Status.NodeName)

	for cacheKey, cacheInfo := range kcNode.Status.CacheStatus {
		// Build image URI with digest (same as what operator uses)
		imageWithDigest := cacheInfo.Image
		if cacheInfo.Digest != "" {
			imageWithDigest = kernelcachecommon.ReplaceUrlTag(cacheInfo.Image, cacheInfo.Digest)
		} else {
			r.Log.Info("Cache missing digest, using tag for storage key", "cache", cacheKey, "image", cacheInfo.Image)
		}

		// Calculate storage key (hash)
		storageKey := v1alpha1.GetKernelCacheStorageKey(imageWithDigest)

		// Remove expected caches from removal set
		delete(foldersToRemove, storageKey)

		r.Log.V(1).Info("Retaining cache directory",
			"cache", cacheKey,
			"storageKey", storageKey,
			"image", imageWithDigest,
			"digest", cacheInfo.Digest)
	}

	// 3. Delete orphaned cache directories (not in CacheStatus)
	if len(foldersToRemove) > 0 {
		r.Log.Info("Found cache(s) to remove from filesystem",
			"count", len(foldersToRemove),
			"node", kcNode.Status.NodeName)

		for storageKey := range foldersToRemove {
			r.Log.Info("Removing cache directory", "storageKey", storageKey, "node", kcNode.Status.NodeName)
			if err := r.fsHelper.removeCache(storageKey); err != nil {
				r.Log.Error(err, "Failed to remove cache directory",
					"storageKey", storageKey,
					"node", kcNode.Status.NodeName)
				return err
			}
		}
	}

	return nil
}

// checkCacheAvailability monitors cache availability on this node
// Operator creates extraction Job, agent checks if cache is accessible
func (r *KernelCacheNodeReconciler) checkCacheAvailability(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
	cacheKey string,
	cacheInfo v1alpha1.CacheNodeCacheInfo,
	config *v1beta1.KernelCacheConfig,
) error {
	jobNamespace := config.JobNamespace

	// Check if KernelCache CR is being deleted - skip availability check
	kc := &v1alpha1.KernelCache{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cacheInfo.Name,
		Namespace: cacheInfo.Namespace,
	}, kc); err != nil {
		if errors.IsNotFound(err) {
			// Cache deleted - discoverCaches will remove from status
			r.Log.V(1).Info("Cache deleted, skipping availability check", "cache", cacheKey)
			return nil
		}
		return err
	}
	if !kc.DeletionTimestamp.IsZero() {
		// Cache being deleted - skip availability check (operator finalizer may have deleted PVC)
		r.Log.V(1).Info("Cache being deleted, skipping availability check", "cache", cacheKey)
		return nil
	}

	// PVC naming: {namespace}-{cachename}-download (created by operator)
	pvcName := cacheInfo.Namespace + "-" + cacheInfo.Name + "-download"

	// Verify PVC exists (created by operator)
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: jobNamespace,
	}, pvc); err != nil {
		if errors.IsNotFound(err) {
			r.Log.Info("Download PVC not found (operator should create it)",
				"pvc", pvcName, "cache", cacheInfo.Name)
			return fmt.Errorf("download PVC not found: %s", pvcName)
		}
		return err
	}

	// Check extraction Job status (created by operator)
	job, err := r.getExtractionJob(ctx, cacheInfo, jobNamespace)
	if err != nil {
		return err
	}

	// Cache is available if:
	// 1. Job exists and completed successfully
	// 2. PVC is bound (RWX storage distributes data)
	// Future: could add actual mount/file check
	available := job != nil && r.jobCompleted(job) && pvc.Status.Phase == corev1.ClaimBound

	// Update status in updateStatus() function
	// Store availability state for status update
	if available {
		r.Log.V(1).Info("Cache available on node", "cache", cacheInfo.Name, "node", kcNode.Status.NodeName)
	}

	return nil
}

// getExtractionJob retrieves the extraction Job for this cache (created by operator)
func (r *KernelCacheNodeReconciler) getExtractionJob(
	ctx context.Context,
	cacheInfo v1alpha1.CacheNodeCacheInfo,
	jobNamespace string,
) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	labels := map[string]string{
		"cache":           cacheInfo.Name,
		"cache-namespace": cacheInfo.Namespace,
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

func (r *KernelCacheNodeReconciler) jobFailed(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *KernelCacheNodeReconciler) updateStatus(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
	config *v1beta1.KernelCacheConfig,
	gpuInfoChanged bool,
) error {
	jobNamespace := config.JobNamespace

	// Track if status changed
	statusChanged := gpuInfoChanged

	// Update extraction state for each cache
	// Check extraction Job status (operator creates ONE Job per cache)
	for cacheKey, cacheInfo := range kcNode.Status.CacheStatus {
		job, err := r.getExtractionJob(ctx, cacheInfo, jobNamespace)
		if err != nil {
			r.Log.Error(err, "failed to get extraction job", "cache", cacheKey)
			continue
		}

		oldState := cacheInfo.State
		oldMessage := cacheInfo.Message

		// Check Job status and PVC availability
		if job != nil {
			switch {
			case r.jobCompleted(job):
				// Verify PVC is accessible on this node
				pvcName := cacheInfo.Namespace + "-" + cacheInfo.Name + "-download"
				pvc := &corev1.PersistentVolumeClaim{}
				if err := r.Get(ctx, types.NamespacedName{
					Name:      pvcName,
					Namespace: jobNamespace,
				}, pvc); err == nil && pvc.Status.Phase == corev1.ClaimBound {
					// Job completed and PVC bound = cache extracted
					cacheInfo.State = v1alpha1.NodeCacheStateExtracted
					cacheInfo.Message = ""
				} else {
					// Job completed but PVC not ready
					cacheInfo.State = v1alpha1.NodeCacheStateDownloading
					cacheInfo.Message = "Waiting for PVC to bind"
				}
			case r.jobFailed(job):
				cacheInfo.State = v1alpha1.NodeCacheStateError
				cacheInfo.Message = "Extraction job failed"
			default:
				cacheInfo.State = v1alpha1.NodeCacheStateDownloading
				cacheInfo.Message = ""
			}
		}

		// Update timestamp if state or message changed
		if oldState != cacheInfo.State || oldMessage != cacheInfo.Message {
			cacheInfo.LastUpdate = metav1.Now()
			statusChanged = true
		}

		kcNode.Status.CacheStatus[cacheKey] = cacheInfo
	}

	// Save old serving state before updating pod counts
	oldCacheStates := make(map[string]v1alpha1.NodeCacheState)
	for cacheKey, cacheInfo := range kcNode.Status.CacheStatus {
		oldCacheStates[cacheKey] = cacheInfo.State
	}

	// Update pod serving counts (check if pods are using caches)
	// This updates ServingNamespaces and transitions Extracted -> Running
	if err := r.updateServingCounts(ctx, kcNode); err != nil {
		r.Log.Error(err, "failed to update serving counts")
		// Non-fatal - continue with status update
	} else {
		// Check if serving counts changed any cache states
		for cacheKey, cacheInfo := range kcNode.Status.CacheStatus {
			if oldState, exists := oldCacheStates[cacheKey]; exists {
				if oldState != cacheInfo.State {
					statusChanged = true
					break
				}
			}
		}
	}

	// Calculate aggregate counts for printer columns
	newCounts := r.calculateAggregateCounts(kcNode)
	if kcNode.Status.Counts == nil || !nodeCountsEqual(kcNode.Status.Counts, newCounts) {
		kcNode.Status.Counts = newCounts
		statusChanged = true
	}

	// Only update status if something changed
	if !statusChanged {
		return nil
	}

	if err := r.Status().Update(ctx, kcNode); err != nil {
		if strings.Contains(err.Error(), "object has been modified") {
			r.Log.Info("Status update conflict (will retry)",
				"node", kcNode.Status.NodeName)
		} else {
			r.Log.Error(err, "Failed to update KernelCacheNode status",
				"node", kcNode.Status.NodeName)
		}
		return err
	}
	return nil
}

// updateServingCounts counts pods on this node using each cache
// Updates ServingNamespaces and transitions state from Extracted to Running
func (r *KernelCacheNodeReconciler) updateServingCounts(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
) error {
	// List all pods on this node using field indexer
	pods := &corev1.PodList{}
	if err := r.List(ctx, pods,
		client.MatchingFields{"spec.nodeName": kcNode.Status.NodeName}); err != nil {
		return err
	}

	r.Log.V(1).Info("Updating serving counts", "node", kcNode.Status.NodeName, "totalPods", len(pods.Items))

	// Count pods per cache per namespace
	for cacheKey, cacheInfo := range kcNode.Status.CacheStatus {
		nsCounts := make(map[string]v1alpha1.NamespaceServingCounts)
		matchedPods := 0

		for _, pod := range pods.Items {
			// Skip completed/failed pods
			if pod.Status.Phase == corev1.PodSucceeded ||
				pod.Status.Phase == corev1.PodFailed {
				continue
			}

			// Check if pod mounts this cache's Serving PVC
			// Only check pods in same namespace as cache
			// Serving PVC naming: {cachename} in same namespace as cache
			// (created by operator after extraction completes)
			if pod.Namespace == cacheInfo.Namespace && r.podMountsCachePVC(&pod, cacheInfo.Name) {
				matchedPods++
				counts := nsCounts[pod.Namespace]
				counts.PodsUsing++

				// Check if pod is ready
				if r.isPodReady(&pod) {
					counts.PodsReady++
				}

				// Check if pod is terminating
				if pod.DeletionTimestamp != nil {
					counts.PodsTerminating++
				}

				nsCounts[pod.Namespace] = counts
				r.Log.V(1).Info("Pod using cache", "pod", pod.Namespace+"/"+pod.Name, "cache", cacheKey, "ready", r.isPodReady(&pod))
			}
		}

		if matchedPods > 0 {
			r.Log.Info("Cache pod counts", "cache", cacheKey, "matchedPods", matchedPods, "namespaces", len(nsCounts))
		}

		// Update state to Running if pods are using cache
		oldState := cacheInfo.State
		if len(nsCounts) > 0 && cacheInfo.State == v1alpha1.NodeCacheStateExtracted {
			cacheInfo.State = v1alpha1.NodeCacheStateRunning
			r.Log.Info("Cache state transition", "cache", cacheKey, "from", oldState, "to", cacheInfo.State)
		}
		// Transition back to Extracted if no pods using cache
		if len(nsCounts) == 0 && cacheInfo.State == v1alpha1.NodeCacheStateRunning {
			cacheInfo.State = v1alpha1.NodeCacheStateExtracted
			r.Log.Info("Cache state transition", "cache", cacheKey, "from", oldState, "to", cacheInfo.State)
		}

		cacheInfo.ServingNamespaces = nsCounts
		kcNode.Status.CacheStatus[cacheKey] = cacheInfo
	}

	return nil
}

// podMountsCachePVC checks if pod mounts a kernel cache PVC
// Checks against Serving PVC naming pattern: {cachename}
func (r *KernelCacheNodeReconciler) podMountsCachePVC(pod *corev1.Pod, cacheName string) bool {
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			// Match Serving PVC name (exact match or contains cache name)
			pvcName := vol.PersistentVolumeClaim.ClaimName
			if pvcName == cacheName || pvcName == cacheName+"-serving" {
				return true
			}
		}
	}
	return false
}

// isPodReady checks if pod is in Running state with Ready condition true
func (r *KernelCacheNodeReconciler) isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// servingNamespacesEqual compares two ServingNamespaces maps for equality
func servingNamespacesEqual(a, b map[string]v1alpha1.NamespaceServingCounts) bool {
	if len(a) != len(b) {
		return false
	}
	for ns, countsA := range a {
		countsB, exists := b[ns]
		if !exists {
			return false
		}
		if countsA.PodsUsing != countsB.PodsUsing ||
			countsA.PodsReady != countsB.PodsReady ||
			countsA.PodsTerminating != countsB.PodsTerminating {
			return false
		}
	}
	return true
}

// calculateAggregateCounts aggregates cache and pod counts across all caches on this node
func (r *KernelCacheNodeReconciler) calculateAggregateCounts(kcNode *v1alpha1.KernelCacheNode) *v1alpha1.NodeCacheCounts {
	counts := &v1alpha1.NodeCacheCounts{}

	for _, cacheStatus := range kcNode.Status.CacheStatus {
		// Count caches by state
		switch cacheStatus.State {
		case v1alpha1.NodeCacheStateRunning:
			counts.CachesInUse++
		case v1alpha1.NodeCacheStateExtracted:
			counts.CachesNotInUse++
		case v1alpha1.NodeCacheStateError:
			counts.CachesError++
		}

		// Aggregate pod counts across all namespaces for this cache
		for _, nsCounts := range cacheStatus.ServingNamespaces {
			counts.TotalPodsUsing += nsCounts.PodsUsing
			counts.TotalPodsTerminating += nsCounts.PodsTerminating
		}
	}

	return counts
}

// nodeCountsEqual compares two NodeCacheCounts for equality
func nodeCountsEqual(a, b *v1alpha1.NodeCacheCounts) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.CachesInUse == b.CachesInUse &&
		a.CachesNotInUse == b.CachesNotInUse &&
		a.CachesError == b.CachesError &&
		a.TotalPodsUsing == b.TotalPodsUsing &&
		a.TotalPodsTerminating == b.TotalPodsTerminating
}

func (r *KernelCacheNodeReconciler) jobCompleted(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// kernelCacheToNodeMapper maps KernelCache changes to KernelCacheNode reconcile requests
// Only reconciles THIS node's KernelCacheNode (filtered by nodeName)
func (r *KernelCacheNodeReconciler) kernelCacheToNodeMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	kc := obj.(*v1alpha1.KernelCache)

	// Only reconcile this node's KernelCacheNode
	kcNodeName := nodeName
	kcNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, types.NamespacedName{Name: kcNodeName}, kcNode); err != nil {
		if !errors.IsNotFound(err) {
			r.Log.Error(err, "failed to get KernelCacheNode", "node", nodeName)
		}
		return []reconcile.Request{}
	}

	// Check if this node has the cache (using cache key format: {namespace}/{name})
	cacheKey := kc.Namespace + "/" + kc.Name
	if _, ok := kcNode.Status.CacheStatus[cacheKey]; ok {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: kcNodeName,
				},
			},
		}
	}

	return []reconcile.Request{}
}

// jobToNodeMapper maps Job changes to KernelCacheNode reconcile requests
// Only reconciles THIS node's KernelCacheNode (filtered by nodeName)
func (r *KernelCacheNodeReconciler) jobToNodeMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	job := obj.(*batchv1.Job)

	// Extract cache info from Job labels
	cacheName := job.Labels["cache"]
	cacheNamespace := job.Labels["cache-namespace"]

	if cacheName == "" || cacheNamespace == "" {
		return []reconcile.Request{}
	}

	// Only reconcile this node's KernelCacheNode
	kcNodeName := nodeName
	kcNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, types.NamespacedName{Name: kcNodeName}, kcNode); err != nil {
		if !errors.IsNotFound(err) {
			r.Log.Error(err, "failed to get KernelCacheNode", "node", nodeName)
		}
		return []reconcile.Request{}
	}

	// Check if this node has the cache (using cache key format: {namespace}/{name})
	cacheKey := cacheNamespace + "/" + cacheName
	if _, ok := kcNode.Status.CacheStatus[cacheKey]; ok {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Name: kcNodeName,
				},
			},
		}
	}

	return []reconcile.Request{}
}

// podToNodeMapper maps Pod changes to KernelCacheNode reconcile requests
// Only reconciles THIS node's KernelCacheNode (filtered by nodeName and PVC predicate)
func (r *KernelCacheNodeReconciler) podToNodeMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	pod := obj.(*corev1.Pod)

	// Only watch pods on THIS node
	if pod.Spec.NodeName != nodeName {
		return []reconcile.Request{}
	}

	// Reconcile this node's KernelCacheNode
	kcNodeName := nodeName
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name: kcNodeName,
			},
		},
	}
}

// podHasPVCVolume checks if pod has any PVC volume, used in event filter to reduce unnecessary reconciliations
func podHasPVCVolume(pod *corev1.Pod) bool {
	for _, vol := range pod.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager
func (r *KernelCacheNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up field indexer for pods by node name
	// This allows efficient List queries for pods on specific nodes
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.nodeName",
		func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KernelCacheNode{}).
		Watches(
			&v1alpha1.KernelCache{},
			handler.EnqueueRequestsFromMapFunc(r.kernelCacheToNodeMapper),
		).
		Watches(
			&batchv1.Job{},
			handler.EnqueueRequestsFromMapFunc(r.jobToNodeMapper),
		).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.podToNodeMapper),
		).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				// Filter KernelCacheNode to only this node
				if kcNode, ok := e.Object.(*v1alpha1.KernelCacheNode); ok {
					return kcNode.Status.NodeName == nodeName
				}
				// Filter Pods to only those with PVC volumes (reduces noise)
				if pod, ok := e.Object.(*corev1.Pod); ok {
					return podHasPVCVolume(pod)
				}
				return true // Allow other object types (KernelCache, Job) through
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Filter KernelCacheNode to only this node
				if kcNode, ok := e.ObjectNew.(*v1alpha1.KernelCacheNode); ok {
					return kcNode.Status.NodeName == nodeName
				}
				// Filter Pods to only those with PVC volumes
				if pod, ok := e.ObjectNew.(*corev1.Pod); ok {
					return podHasPVCVolume(pod)
				}
				return true // Allow other object types through
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Filter KernelCacheNode to only this node
				if kcNode, ok := e.Object.(*v1alpha1.KernelCacheNode); ok {
					return kcNode.Status.NodeName == nodeName
				}
				// Filter Pods to only those with PVC volumes
				if pod, ok := e.Object.(*corev1.Pod); ok {
					return podHasPVCVolume(pod)
				}
				return true // Allow other object types through
			},
		}).
		Complete(r)
}
