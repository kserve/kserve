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
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	ExtractContainerName = "kserve-kernelcache-extract"
	CachePVCMountName    = "cache-pvc"
	MountPath            = "/mnt/kernel-cache"
)

var (
	defaultExtractImage                      = "quay.io/gkm/gkm-extract:latest"
	jobTTLSecondsAfterFinished int32         = 3600
	reconcileInterval          time.Duration = 1 * time.Minute
	nodeName                                 = os.Getenv("NODE_NAME")
)

type KernelCacheNodeReconciler struct {
	client.Client
	Clientset *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// Reconcile implements controller-runtime Reconciler
func (r *KernelCacheNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("Reconciling KernelCacheNode", "name", req.Name, "namespace", req.Namespace)

	// Get KernelCacheNode
	kcNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, req.NamespacedName, kcNode); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Load config from inferenceservice-config ConfigMap
	isvcConfigMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, r.Clientset)
	if err != nil {
		r.Log.Error(err, "unable to get configmap", "name", constants.InferenceServiceConfigMapName)
		return reconcile.Result{}, err
	}

	// Get job namespace from config (default: kserve)
	jobNamespace := "kserve"
	if isvcConfigMap != nil {
		if ns, ok := isvcConfigMap.Data["localModel"]; ok && ns != "" {
			// Parse localModel config for jobNamespace
			// For now, use default
			_ = ns
		}
	}

	// Process each cache in this node's spec
	for _, cacheInfo := range kcNode.Status.Caches {
		if err := r.ensureCacheExtracted(ctx, kcNode, cacheInfo, jobNamespace); err != nil {
			r.Log.Error(err, "failed to ensure cache extracted", "cache", cacheInfo.Name)
			// Continue processing other caches
		}
	}

	// Update status
	if err := r.updateStatus(ctx, kcNode); err != nil {
		return ctrl.Result{}, err
	}

	// Cleanup old jobs
	if err := r.cleanupJobs(ctx, kcNode); err != nil {
		r.Log.Error(err, "failed to cleanup jobs")
	}

	return ctrl.Result{RequeueAfter: reconcileInterval}, nil
}

func (r *KernelCacheNodeReconciler) ensureCacheExtracted(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
	cacheInfo v1alpha1.KernelCacheInfo,
	jobNamespace string,
) error {
	// PVC naming: {cachename}-download (created by operator)
	pvcName := fmt.Sprintf("%s-download", cacheInfo.Name)

	// Verify PVC exists (created by operator, NOT by agent)
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

	// Check if extraction Job needed
	job, err := r.getLatestJob(ctx, kcNode, cacheInfo)
	if err != nil {
		return err
	}

	if job == nil || r.jobFailed(job) {
		return r.launchExtractionJob(ctx, kcNode, cacheInfo, pvcName, jobNamespace)
	}

	return nil
}

func (r *KernelCacheNodeReconciler) getLatestJob(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
	cacheInfo v1alpha1.KernelCacheInfo,
) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	labels := map[string]string{
		"cache": cacheInfo.Name,
		"node":  kcNode.Status.NodeName,
	}

	if err := r.List(ctx, jobList,
		client.InNamespace(kcNode.Namespace),
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

func (r *KernelCacheNodeReconciler) launchExtractionJob(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
	cacheInfo v1alpha1.KernelCacheInfo,
	pvcName string,
	jobNamespace string,
) error {
	jobName := fmt.Sprintf("%s-%s-extract", cacheInfo.Name, kcNode.Status.NodeName)

	// Hash-based storage key for deduplication
	storageKey := v1alpha1.GetKernelCacheStorageKey(cacheInfo.Image)

	container := &corev1.Container{
		Name:  ExtractContainerName,
		Image: defaultExtractImage,
		Env: []corev1.EnvVar{
			{Name: "CACHE_DIR", Value: MountPath},
			{Name: "IMAGE_URL", Value: cacheInfo.Image},
			{Name: "NO_GPU", Value: "false"},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      CachePVCMountName,
				MountPath: MountPath,
				ReadOnly:  false,
				SubPath:   filepath.Join("kernel-cache", storageKey),
			},
		},
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: jobName + "-",
			Namespace:    jobNamespace,
			Labels: map[string]string{
				"app":   "kernel-cache-extract",
				"cache": cacheInfo.Name,
				"node":  kcNode.Status.NodeName,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(jobTTLSecondsAfterFinished),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeName:      kcNode.Status.NodeName,
					Containers:    []corev1.Container{*container},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: CachePVCMountName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(kcNode, job, r.Scheme); err != nil {
		return err
	}

	r.Log.Info("Creating extraction job", "job", jobName, "cache", cacheInfo.Name, "node", kcNode.Status.NodeName)
	return r.Create(ctx, job)
}

func (r *KernelCacheNodeReconciler) updateStatus(
	ctx context.Context,
	kcNode *v1alpha1.KernelCacheNode,
) error {
	// Initialize CacheStatus map if needed
	if kcNode.Status.CacheStatus == nil {
		kcNode.Status.CacheStatus = make(map[string]v1alpha1.CacheNodeExtractionStatus)
	}

	// Update download status for each cache
	for _, cacheInfo := range kcNode.Status.Caches {
		job, err := r.getLatestJob(ctx, kcNode, cacheInfo)
		if err != nil {
			r.Log.Error(err, "failed to get latest job", "cache", cacheInfo.Name)
			continue
		}

		status := v1alpha1.CacheNodeExtractionStatus{
			DownloadStatus: v1alpha1.NodeExtractionPending,
			LastUpdate:     metav1.Now(),
		}

		if job != nil {
			if r.jobCompleted(job) {
				status.DownloadStatus = v1alpha1.NodeExtractionCompleted
			} else if r.jobFailed(job) {
				status.DownloadStatus = v1alpha1.NodeExtractionFailed
				status.Message = "Extraction job failed"
			} else {
				status.DownloadStatus = v1alpha1.NodeExtractionInProgress
			}
		}

		kcNode.Status.CacheStatus[cacheInfo.Name] = status
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
) error {
	// Jobs with TTL will auto-delete, but we can help clean up failed jobs
	jobList := &batchv1.JobList{}
	labels := map[string]string{
		"node": kcNode.Status.NodeName,
	}

	if err := r.List(ctx, jobList,
		client.InNamespace(kcNode.Namespace),
		client.MatchingLabels(labels),
	); err != nil {
		return err
	}

	for i := range jobList.Items {
		job := &jobList.Items[i]
		// Delete failed jobs older than 1 hour
		if r.jobFailed(job) {
			age := time.Since(job.CreationTimestamp.Time)
			if age > time.Hour {
				r.Log.Info("Deleting old failed job", "job", job.Name)
				if err := r.Delete(ctx, job); err != nil && !errors.IsNotFound(err) {
					r.Log.Error(err, "failed to delete job", "job", job.Name)
				}
			}
		}
	}

	return nil
}

// kernelCacheToNodeMapper maps KernelCache changes to KernelCacheNode reconcile requests
func (r *KernelCacheNodeReconciler) kernelCacheToNodeMapper(ctx context.Context, obj client.Object) []reconcile.Request {
	// When a KernelCache changes, reconcile all KernelCacheNodes that reference it
	kc := obj.(*v1alpha1.KernelCache)

	nodeList := &v1alpha1.KernelCacheNodeList{}
	if err := r.List(ctx, nodeList, client.InNamespace(kc.Namespace)); err != nil {
		r.Log.Error(err, "failed to list KernelCacheNodes")
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		for _, cacheInfo := range node.Status.Caches {
			if cacheInfo.Name == kc.Name && cacheInfo.Namespace == kc.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      node.Name,
						Namespace: node.Namespace,
					},
				})
				break
			}
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager
func (r *KernelCacheNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KernelCacheNode{}).
		Owns(&batchv1.Job{}).
		Watches(
			&v1alpha1.KernelCache{},
			handler.EnqueueRequestsFromMapFunc(r.kernelCacheToNodeMapper),
		).
		Complete(r)
}
