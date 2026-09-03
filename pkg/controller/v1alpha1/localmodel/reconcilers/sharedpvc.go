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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/localmodel/jobs"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	importJobNameSuffix        = "-import"
	importPVCUIDAnnotation     = "serving.kserve.io/import-pvc-uid"
	importStorageKeyAnnotation = "serving.kserve.io/import-storage-key"
)

var errImportJobConflict = errors.New("import Job name conflict")

// sharedState is the derived readiness/copies outcome for a shared-PVC cache.
type sharedState struct {
	status    metav1.ConditionStatus
	reason    string
	message   string
	available int
	failed    int
}

// reconcileSharedPVC handles a LocalModelNamespaceCache in shared-PVC mode (spec.pvcRef set).
// It skips node fan-out and per-node PV/PVC creation entirely: it validates the referenced
// PVC, ensures a single deterministic import Job, and derives copies/Ready from PVC and Job
// state. The referenced PVC and the imported model data are user-owned and never mutated or
// deleted by the controller. The owned Job is deleted before the cache finalizer is removed,
// preventing a successor cache from importing into the same destination concurrently.
func (c *LocalModelNamespaceCacheReconciler) reconcileSharedPVC(ctx context.Context, localModel *v1alpha1.LocalModelNamespaceCache, isvcConfigMap *corev1.ConfigMap) (ctrl.Result, error) {
	log := c.Log.WithValues("name", localModel.Name, "namespace", localModel.Namespace, "pvcRef", *localModel.Spec.PVCRef)

	if !localModel.DeletionTimestamp.IsZero() {
		return c.finalizeSharedPVC(ctx, localModel)
	}
	if !utils.Includes(localModel.Finalizers, NamespaceCacheFinalizerName) {
		patch := client.MergeFrom(localModel.DeepCopy())
		localModel.Finalizers = append(localModel.Finalizers, NamespaceCacheFinalizerName)
		if err := c.Patch(ctx, localModel, patch); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	pvcName := *localModel.Spec.PVCRef
	storageKey := v1alpha1.GetStorageKey(localModel.Spec.SourceModelUri)

	// Fetch the referenced PVC from the cache namespace.
	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: localModel.Namespace}, pvc); err != nil {
		if apierr.IsNotFound(err) {
			log.Info("Referenced PVC not found")
			return c.applySharedStatus(ctx, localModel, sharedState{
				status:  metav1.ConditionFalse,
				reason:  v1alpha1.ReasonPVCNotFound,
				message: fmt.Sprintf("PersistentVolumeClaim %q not found in namespace %q", pvcName, localModel.Namespace),
			})
		}
		return ctrl.Result{}, err
	}

	// PVC preflight: volume mode, access mode, and capacity.
	if state, ok := checkPVCPreflight(pvc, localModel.Spec.ModelSize); !ok {
		log.Info("PVC preflight failed", "reason", state.reason, "message", state.message)
		return c.applySharedStatus(ctx, localModel, state)
	}

	// Destination ownership: only one cache may own (namespace, pvcRef, storageKey).
	if conflict, err := c.destinationConflict(ctx, localModel, storageKey); err != nil {
		return ctrl.Result{}, err
	} else if conflict != "" {
		log.Info("Destination conflict", "message", conflict)
		return c.applySharedStatus(ctx, localModel, sharedState{
			status:  metav1.ConditionFalse,
			reason:  v1alpha1.ReasonDestinationConflict,
			message: conflict,
		})
	}

	// Get or create the single deterministic import Job.
	job, requeue, err := c.getOrCreateImportJob(ctx, localModel, pvc, storageKey, isvcConfigMap)
	if err != nil {
		if errors.Is(err, errImportJobConflict) {
			if _, statusErr := c.applySharedStatus(ctx, localModel, sharedState{
				status:  metav1.ConditionFalse,
				reason:  v1alpha1.ReasonImportJobConflict,
				message: err.Error(),
			}); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
		}
		return ctrl.Result{}, err
	}
	if requeue {
		// The owned Job's deletion event triggers the next reconciliation.
		return c.applySharedStatus(ctx, localModel, sharedState{
			status:  metav1.ConditionFalse,
			reason:  v1alpha1.ReasonImportPending,
			message: "Referenced PVC identity changed; replacing the import Job",
		})
	}

	return c.applySharedStatus(ctx, localModel, stateFromJob(job))
}

func (c *LocalModelNamespaceCacheReconciler) finalizeSharedPVC(ctx context.Context, localModel *v1alpha1.LocalModelNamespaceCache) (ctrl.Result, error) {
	job := &batchv1.Job{}
	reader := c.APIReader
	if reader == nil {
		reader = c.Client
	}
	// Finalizer release is a correctness decision: use an authoritative read so a newly
	// created Job cannot be missed because the informer cache has not observed it yet.
	err := reader.Get(ctx, types.NamespacedName{Name: importJobName(localModel.Name), Namespace: localModel.Namespace}, job)
	if err == nil && metav1.IsControlledBy(job, localModel) {
		if job.DeletionTimestamp.IsZero() {
			if err := c.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !apierr.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
		// The owned Job's deletion event re-enqueues the cache so finalization can finish.
		return ctrl.Result{}, nil
	}
	if err != nil && !apierr.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	patch := client.MergeFrom(localModel.DeepCopy())
	localModel.Finalizers = utils.RemoveString(localModel.Finalizers, NamespaceCacheFinalizerName)
	if err := c.Patch(ctx, localModel, patch); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// checkPVCPreflight validates the referenced PVC. It returns ok=false with a populated
// sharedState (status False, reason, message) when the PVC cannot be used, and ok=true when
// Job creation may proceed. An unbound PVC with a sufficient request is allowed to proceed so
// provisioning and binding can occur.
func checkPVCPreflight(pvc *corev1.PersistentVolumeClaim, modelSize resource.Quantity) (sharedState, bool) {
	// Require filesystem volume mode (nil defaults to Filesystem).
	if pvc.Spec.VolumeMode != nil && *pvc.Spec.VolumeMode == corev1.PersistentVolumeBlock {
		return sharedState{
			status:  metav1.ConditionFalse,
			reason:  v1alpha1.ReasonUnsupportedVolumeMode,
			message: "referenced PVC must use Filesystem volume mode",
		}, false
	}

	// Require ReadWriteMany so the import Job can write shared storage.
	hasRWX := false
	for _, mode := range pvc.Spec.AccessModes {
		if mode == corev1.ReadWriteMany {
			hasRWX = true
			break
		}
	}
	if !hasRWX {
		return sharedState{
			status:  metav1.ConditionFalse,
			reason:  v1alpha1.ReasonUnsupportedAccessMode,
			message: "referenced PVC must include the ReadWriteMany access mode",
		}, false
	}

	// Compare modelSize against the requested storage.
	if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		if req.Cmp(modelSize) < 0 {
			return sharedState{
				status:  metav1.ConditionFalse,
				reason:  v1alpha1.ReasonInsufficientCapacity,
				message: fmt.Sprintf("PVC requested storage %s is smaller than model size %s", req.String(), modelSize.String()),
			}, false
		}
	}

	// If bound, also verify the actual capacity.
	if pvc.Status.Phase == corev1.ClaimBound {
		if capacity, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			if capacity.Cmp(modelSize) < 0 {
				return sharedState{
					status:  metav1.ConditionFalse,
					reason:  v1alpha1.ReasonInsufficientCapacity,
					message: fmt.Sprintf("PVC bound capacity %s is smaller than model size %s", capacity.String(), modelSize.String()),
				}, false
			}
		}
	}

	return sharedState{}, true
}

// destinationConflict returns a non-empty message when another shared-PVC cache in the same
// namespace already owns the (namespace, pvcRef, storageKey) tuple. Different storage keys on
// the same claim are allowed.
func (c *LocalModelNamespaceCacheReconciler) destinationConflict(ctx context.Context, localModel *v1alpha1.LocalModelNamespaceCache, storageKey string) (string, error) {
	caches := &v1alpha1.LocalModelNamespaceCacheList{}
	reader := c.APIReader
	if reader == nil {
		reader = c.Client
	}
	// Destination ownership is a correctness decision, so bypass the informer cache to make
	// concurrent admissions visible before either cache can create its import Job.
	if err := reader.List(ctx, caches, client.InNamespace(localModel.Namespace)); err != nil {
		return "", err
	}
	for i := range caches.Items {
		other := &caches.Items[i]
		if other.Name == localModel.Name {
			continue
		}
		if !other.Spec.SharedPVCMode() {
			continue
		}
		if *other.Spec.PVCRef != *localModel.Spec.PVCRef {
			continue
		}
		if v1alpha1.GetStorageKey(other.Spec.SourceModelUri) == storageKey && cachePrecedes(other, localModel) {
			return fmt.Sprintf("destination pvc %q already holds this model via cache %s/%s",
				*localModel.Spec.PVCRef, other.Namespace, other.Name), nil
		}
	}
	return "", nil
}

// cachePrecedes reports whether candidate is the deterministic owner ahead of current.
// Creation time preserves first-writer ownership; name and UID make ties deterministic.
func cachePrecedes(candidate, current *v1alpha1.LocalModelNamespaceCache) bool {
	if !candidate.CreationTimestamp.Equal(&current.CreationTimestamp) {
		return candidate.CreationTimestamp.Time.Before(current.CreationTimestamp.Time)
	}
	if candidate.Name != current.Name {
		return candidate.Name < current.Name
	}
	return string(candidate.UID) < string(current.UID)
}

// getOrCreateImportJob returns the existing deterministic import Job, or creates it. A retained
// succeeded/failed Job is returned as-is; a second Job is never created while one exists.
func (c *LocalModelNamespaceCacheReconciler) getOrCreateImportJob(ctx context.Context, localModel *v1alpha1.LocalModelNamespaceCache, pvc *corev1.PersistentVolumeClaim, storageKey string, isvcConfigMap *corev1.ConfigMap) (*batchv1.Job, bool, error) {
	jobName := importJobName(localModel.Name)
	job := &batchv1.Job{}
	err := c.Get(ctx, types.NamespacedName{Name: jobName, Namespace: localModel.Namespace}, job)
	if err == nil {
		return c.validateExistingImportJob(ctx, job, localModel, pvc, storageKey)
	}
	if !apierr.IsNotFound(err) {
		return nil, false, err
	}

	job, err = c.buildImportJob(ctx, localModel, jobName, pvc, storageKey, isvcConfigMap)
	if err != nil {
		return nil, false, err
	}
	if err := controllerutil.SetControllerReference(localModel, job, c.Scheme); err != nil {
		return nil, false, err
	}
	if err := c.Create(ctx, job); err != nil {
		if apierr.IsAlreadyExists(err) {
			// Lost a create race; fetch and return the existing Job.
			if getErr := c.Get(ctx, types.NamespacedName{Name: jobName, Namespace: localModel.Namespace}, job); getErr != nil {
				return nil, false, getErr
			}
			return c.validateExistingImportJob(ctx, job, localModel, pvc, storageKey)
		}
		return nil, false, err
	}
	c.Log.Info("Created shared-PVC import job", "name", jobName, "namespace", localModel.Namespace, "storageKey", storageKey)
	return job, false, nil
}

func (c *LocalModelNamespaceCacheReconciler) validateExistingImportJob(ctx context.Context, job *batchv1.Job, localModel *v1alpha1.LocalModelNamespaceCache, pvc *corev1.PersistentVolumeClaim, storageKey string) (*batchv1.Job, bool, error) {
	if !metav1.IsControlledBy(job, localModel) {
		return nil, false, fmt.Errorf("%w: Job %s/%s already exists and is not controlled by LocalModelNamespaceCache %s/%s",
			errImportJobConflict,
			job.Namespace, job.Name, localModel.Namespace, localModel.Name)
	}
	if job.Annotations[importPVCUIDAnnotation] == string(pvc.UID) &&
		job.Annotations[importStorageKeyAnnotation] == storageKey {
		return job, false, nil
	}

	c.Log.Info("Replacing stale shared-PVC import job", "name", job.Name, "namespace", job.Namespace,
		"pvcUID", pvc.UID, "storageKey", storageKey)
	if err := c.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !apierr.IsNotFound(err) {
		return nil, false, err
	}
	return nil, true, nil
}

// buildImportJob constructs the single import Job for a shared-PVC cache.
func (c *LocalModelNamespaceCacheReconciler) buildImportJob(ctx context.Context, localModel *v1alpha1.LocalModelNamespaceCache, jobName string, pvc *corev1.PersistentVolumeClaim, storageKey string, isvcConfigMap *corev1.ConfigMap) (*batchv1.Job, error) {
	storageInitializerConfig, err := v1beta1.GetStorageInitializerConfigs(isvcConfigMap)
	if err != nil {
		c.Log.Error(err, "Failed to get storage initializer config, using defaults")
	}

	container, err := jobs.ResolveDownloadContainer(ctx, c.Client, storageInitializerConfig, localModel.Spec.SourceModelUri)
	if err != nil {
		return nil, err
	}
	container.Args = []string{localModel.Spec.SourceModelUri, jobs.MountPath}
	container.VolumeMounts = []corev1.VolumeMount{
		{
			MountPath: jobs.MountPath,
			Name:      jobs.PvcSourceMountName,
			ReadOnly:  false,
			SubPath:   filepath.Join("models", storageKey),
		},
	}

	volumes := []corev1.Volume{
		{
			Name: jobs.PvcSourceMountName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Name,
				},
			},
		},
	}

	if localModel.Spec.ServiceAccountName != "" || localModel.Spec.Storage != nil {
		if c.CredentialBuilder == nil {
			c.CredentialBuilder = credentials.NewCredentialBuilder(c.Client, c.Clientset, isvcConfigMap)
		}
		if err := jobs.InjectCredentials(ctx, c.CredentialBuilder, c.Log, container, &volumes,
			localModel.Spec.ServiceAccountName, localModel.Spec.Storage, localModel.Namespace); err != nil {
			c.Log.Error(err, "Failed to inject credentials", "model", localModel.Name)
		}
	}

	var fsGroup *int64
	if localModelConfig, cfgErr := v1beta1.NewLocalModelConfig(isvcConfigMap); cfgErr == nil {
		fsGroup = localModelConfig.FSGroup
	}

	parallelism := int32(1)
	completions := int32(1)
	backoffLimit := int32(2)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: localModel.Namespace,
			Annotations: map[string]string{
				importPVCUIDAnnotation:     string(pvc.UID),
				importStorageKeyAnnotation: storageKey,
			},
			Labels: map[string]string{
				"model":          localModel.Name,
				"modelNamespace": localModel.Namespace,
			},
		},
		Spec: batchv1.JobSpec{
			Parallelism:  &parallelism,
			Completions:  &completions,
			BackoffLimit: &backoffLimit,
			// No ttlSecondsAfterFinished: succeeded and failed Jobs are retained as durable
			// evidence and to gate re-import.
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					// No node selector: the import runs on any node that can mount the RWX claim.
					Containers:    []corev1.Container{*container},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes:       volumes,
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: fsGroup,
					},
				},
			},
		},
	}
	return job, nil
}

// stateFromJob maps import Job state to copies and the Ready condition.
func stateFromJob(job *batchv1.Job) sharedState {
	switch {
	case jobHasCondition(job, batchv1.JobComplete):
		return sharedState{status: metav1.ConditionTrue, reason: v1alpha1.ReasonImportSucceeded, message: "Model import completed", available: 1}
	case jobHasCondition(job, batchv1.JobFailed):
		return sharedState{status: metav1.ConditionFalse, reason: v1alpha1.ReasonImportFailed, message: "Model import job failed; delete the job to retry", failed: 1}
	case job.Status.Active > 0 || (job.Status.Ready != nil && *job.Status.Ready > 0):
		return sharedState{status: metav1.ConditionFalse, reason: v1alpha1.ReasonImportRunning, message: "Model import job is running"}
	default:
		return sharedState{status: metav1.ConditionFalse, reason: v1alpha1.ReasonImportPending, message: "Model import job is pending"}
	}
}

func jobHasCondition(job *batchv1.Job, condType batchv1.JobConditionType) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == condType && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// applySharedStatus writes copies and the Ready condition for a shared-PVC cache, only when
// something changed. It clears node-keyed status which does not apply in shared-PVC mode.
func (c *LocalModelNamespaceCacheReconciler) applySharedStatus(ctx context.Context, localModel *v1alpha1.LocalModelNamespaceCache, state sharedState) (ctrl.Result, error) {
	desired := localModel.DeepCopy()
	desired.Status.NodeStatus = nil
	desired.Status.ModelCopies = &v1alpha1.ModelCopies{Total: 1, Available: state.available, Failed: state.failed}
	switch state.status {
	case metav1.ConditionTrue:
		desired.Status.MarkReady(localModel.Generation)
	case metav1.ConditionUnknown:
		desired.Status.MarkReadyUnknown(state.reason, state.message, localModel.Generation)
	default:
		desired.Status.MarkNotReady(state.reason, state.message, localModel.Generation)
	}

	if reflect.DeepEqual(localModel.Status, desired.Status) {
		return ctrl.Result{}, nil
	}
	localModel.Status = desired.Status
	if err := c.Status().Update(ctx, localModel); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// importJobName returns a deterministic, DNS-1123 (<=63 char) Job name derived from the cache
// name. A hash suffix keeps the name unique when the cache name is too long to fit.
func importJobName(cacheName string) string {
	name := cacheName + importJobNameSuffix
	if len(name) <= 63 {
		return name
	}
	sum := sha256.Sum256([]byte(cacheName))
	hash := hex.EncodeToString(sum[:])[:8]
	keep := 63 - len(importJobNameSuffix) - 1 - len(hash) // room for '-' + hash
	prefix := strings.TrimRight(cacheName[:keep], "-")
	return prefix + "-" + hash + importJobNameSuffix
}
