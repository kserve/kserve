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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcaches,verbs=get;list;watch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachenodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
package kernelcachenode

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
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

var nodeName = os.Getenv("NODE_NAME")

type KernelCacheNodeReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
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
	if err := r.populateGPUInfo(kcNode, kernelCacheConfig.NoGPU); err != nil {
		r.Log.Error(err, "failed to populate GPU info", "node", kcNode.Status.NodeName)
		// Non-fatal - continue with reconciliation
		// Status update will persist GPUInfo if successful
	} else if len(kcNode.Status.GPUInfo) > 0 {
		// GPU info was just populated - update status immediately
		if err := r.Status().Update(ctx, kcNode); err != nil {
			r.Log.Error(err, "failed to update status with GPU info")
			return ctrl.Result{}, err
		}
		r.Log.Info("GPU info populated", "node", kcNode.Status.NodeName, "gpuTypes", len(kcNode.Status.GPUInfo))
	}

	// Process each cache in this node's spec
	// Monitor cache availability (operator creates extraction Job)
	for _, cacheInfo := range kcNode.Status.Caches {
		if err := r.checkCacheAvailability(ctx, kcNode, cacheInfo, kernelCacheConfig); err != nil {
			r.Log.Error(err, "failed to check cache availability", "cache", cacheInfo.Name)
			// Continue processing other caches
		}
	}

	// Update status
	if err := r.updateStatus(ctx, kcNode, kernelCacheConfig); err != nil {
		return ctrl.Result{}, err
	}

	// Cleanup old jobs
	if err := r.cleanupJobs(ctx, kcNode, kernelCacheConfig); err != nil {
		r.Log.Error(err, "failed to cleanup jobs")
	}

	return ctrl.Result{RequeueAfter: reconcileInterval}, nil
}

// checkCacheAvailability monitors cache availability on this node
// Operator creates extraction Job, agent checks if cache is accessible
func (r *KernelCacheNodeReconciler) checkCacheAvailability(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
	cacheInfo v1alpha1.KernelCacheInfo,
	config *v1beta1.KernelCacheConfig,
) error {
	jobNamespace := config.JobNamespace

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
	cacheInfo v1alpha1.KernelCacheInfo,
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
) error {
	jobNamespace := config.JobNamespace

	// Initialize CacheStatus map if needed
	if kcNode.Status.CacheStatus == nil {
		kcNode.Status.CacheStatus = make(map[string]v1alpha1.CacheNodeExtractionStatus)
	}

	// Track if status changed
	statusChanged := false

	// Update download status for each cache
	// Check extraction Job status (operator creates ONE Job per cache)
	for _, cacheInfo := range kcNode.Status.Caches {
		job, err := r.getExtractionJob(ctx, cacheInfo, jobNamespace)
		if err != nil {
			r.Log.Error(err, "failed to get extraction job", "cache", cacheInfo.Name)
			continue
		}

		status := v1alpha1.CacheNodeExtractionStatus{
			DownloadStatus: v1alpha1.NodeExtractionPending,
			LastUpdate:     metav1.Now(),
		}

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
					// Job completed and PVC bound = cache available
					status.DownloadStatus = v1alpha1.NodeExtractionCompleted
				} else {
					// Job completed but PVC not ready
					status.DownloadStatus = v1alpha1.NodeExtractionInProgress
					status.Message = "Waiting for PVC to bind"
				}
			case r.jobFailed(job):
				status.DownloadStatus = v1alpha1.NodeExtractionFailed
				status.Message = "Extraction job failed"
			default:
				status.DownloadStatus = v1alpha1.NodeExtractionInProgress
			}
		}

		// Only update if status changed (compare without LastUpdate timestamp)
		oldStatus, exists := kcNode.Status.CacheStatus[cacheInfo.Name]
		if !exists || oldStatus.DownloadStatus != status.DownloadStatus || oldStatus.Message != status.Message {
			statusChanged = true
		}

		kcNode.Status.CacheStatus[cacheInfo.Name] = status
	}

	// Only update status if something changed
	if !statusChanged {
		return nil
	}

	return r.Status().Update(ctx, kcNode)
}

func (r *KernelCacheNodeReconciler) jobCompleted(job *batchv1.Job) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *KernelCacheNodeReconciler) cleanupJobs(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
	config *v1beta1.KernelCacheConfig,
) error {
	// Agent no longer creates Jobs (operator creates them)
	// Job cleanup handled by TTL and operator deletion handler
	// This function is now a no-op but kept for compatibility
	return nil
}

// kernelCacheToNodeMapper maps KernelCache changes to KernelCacheNode reconcile requests
// Only reconciles THIS node's KernelCacheNode (filtered by nodeName)
func (r *KernelCacheNodeReconciler) kernelCacheToNodeMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	kc := obj.(*v1alpha1.KernelCache)

	// Only reconcile this node's KernelCacheNode
	kcNodeName := "kernel-cache-node-" + nodeName
	kcNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, types.NamespacedName{Name: kcNodeName}, kcNode); err != nil {
		if !errors.IsNotFound(err) {
			r.Log.Error(err, "failed to get KernelCacheNode", "node", nodeName)
		}
		return []reconcile.Request{}
	}

	// Check if this node has the cache
	for _, cacheInfo := range kcNode.Status.Caches {
		if cacheInfo.Name == kc.Name && cacheInfo.Namespace == kc.Namespace {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: kcNodeName,
					},
				},
			}
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
	kcNodeName := "kernel-cache-node-" + nodeName
	kcNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, types.NamespacedName{Name: kcNodeName}, kcNode); err != nil {
		if !errors.IsNotFound(err) {
			r.Log.Error(err, "failed to get KernelCacheNode", "node", nodeName)
		}
		return []reconcile.Request{}
	}

	// Check if this node has the cache
	for _, cacheInfo := range kcNode.Status.Caches {
		if cacheInfo.Name == cacheName && cacheInfo.Namespace == cacheNamespace {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name: kcNodeName,
					},
				},
			}
		}
	}

	return []reconcile.Request{}
}

// SetupWithManager sets up the controller with the Manager
func (r *KernelCacheNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				if kcNode, ok := e.Object.(*v1alpha1.KernelCacheNode); ok {
					return kcNode.Status.NodeName == nodeName
				}
				return true // Allow other object types (KernelCache, Job) through
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				if kcNode, ok := e.ObjectNew.(*v1alpha1.KernelCacheNode); ok {
					return kcNode.Status.NodeName == nodeName
				}
				return true // Allow other object types through
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				if kcNode, ok := e.Object.(*v1alpha1.KernelCacheNode); ok {
					return kcNode.Status.NodeName == nodeName
				}
				return true // Allow other object types through
			},
		}).
		Complete(r)
}
