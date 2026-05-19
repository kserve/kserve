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
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
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
// Step 2 - Create Download PVC (operator creates ALL PVCs, agent creates only Jobs)
// Step 3 - Get nodes in cluster
// Step 4 - Ensure KernelCacheNode exists for each node
// Step 5 - Aggregate status from KernelCacheNodes
func (r *KernelCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log.Info("Reconciling KernelCache", "name", req.Name, "namespace", req.Namespace)

	kc := &v1alpha1.KernelCache{}
	if err := r.Get(ctx, req.NamespacedName, kc); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// Step 1: Handle deletion
	if !kc.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, kc)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(kc, KernelCacheFinalizerName) {
		controllerutil.AddFinalizer(kc, KernelCacheFinalizerName)
		return ctrl.Result{}, r.Update(ctx, kc)
	}

	// Step 2: Create Download PVC (operator creates ALL PVCs, agent creates only Jobs)
	if err := r.ensureDownloadPVC(ctx, kc); err != nil {
		r.Log.Error(err, "failed to create download PVC")
		return ctrl.Result{}, err
	}

	// Step 3: Get nodes in cluster
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		return ctrl.Result{}, err
	}

	// Step 4: For each node, ensure KernelCacheNode exists and has this cache
	for _, node := range nodes.Items {
		if err := r.ensureKernelCacheNode(ctx, kc, node.Name); err != nil {
			r.Log.Error(err, "failed to ensure KernelCacheNode", "node", node.Name)
			continue
		}
	}

	// Step 5: Aggregate status from KernelCacheNodes
	if err := r.updateAggregateStatus(ctx, kc); err != nil {
		return ctrl.Result{}, err
	}

	// No periodic requeue needed - all operations synchronous
	// Watches on KernelCacheNode status changes trigger reconciliation
	return ctrl.Result{}, nil
}

// handleDeletion: Phase 1 simple finalizer (no pod usage check)
// Phase 2 will add validating webhook for production safety
func (r *KernelCacheReconciler) handleDeletion(ctx context.Context, kc *v1alpha1.KernelCache) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(kc, KernelCacheFinalizerName) {
		return ctrl.Result{}, nil
	}

	r.Log.Info("Deleting KernelCache", "name", kc.Name, "namespace", kc.Namespace)

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

	// Delete PVC and PV (no owner references, manual cleanup required)
	jobNamespace := "kserve"
	pvcName := kc.Namespace + "-" + kc.Name + "-download"
	pvName := kc.Namespace + "-" + kc.Name + "-download-pv"

	// Delete PVC
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: jobNamespace,
	}, pvc); err == nil {
		if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
			r.Log.Error(err, "Failed to delete PVC", "pvc", pvcName)
			return ctrl.Result{}, err
		}
		r.Log.Info("Deleted PVC", "pvc", pvcName)
	}

	// Delete PV
	pv := &corev1.PersistentVolume{}
	if err := r.Get(ctx, types.NamespacedName{
		Name: pvName,
	}, pv); err == nil {
		if err := r.Delete(ctx, pv); err != nil && !errors.IsNotFound(err) {
			r.Log.Error(err, "Failed to delete PV", "pv", pvName)
			return ctrl.Result{}, err
		}
		r.Log.Info("Deleted PV", "pv", pvName)
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(kc, KernelCacheFinalizerName)
	return ctrl.Result{}, r.Update(ctx, kc)
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
) error {
	// Create Download PV and PVC in job namespace (kserve), NOT in KernelCache namespace
	// Job namespace is where agents run and create extraction Jobs

	jobNamespace := "kserve" // Phase 1: hardcoded, Phase 2: from ConfigMap
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
				"app.kubernetes.io/name":       "kernelcache",
				"app.kubernetes.io/component":  "download",
				"kernelcache.kserve.io/cache":  kc.Name,
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
				"app.kubernetes.io/name":       "kernelcache",
				"app.kubernetes.io/component":  "download",
				"kernelcache.kserve.io/cache":  kc.Name,
				"kernelcache.kserve.io/namespace": kc.Namespace,
				"kernelcache.kserve.io/pv":     pvName,
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
	requests := []reconcile.Request{}

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
		Complete(r)
}
