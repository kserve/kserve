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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	kernelcacheutil "github.com/kserve/kserve/pkg/kernelcache"
	kernelcacheconfig "github.com/kserve/kserve/pkg/kernelcache/config"
	cacheidentity "github.com/kserve/kserve/pkg/kernelcache/identity"
	"github.com/kserve/kserve/pkg/kernelcache/reporter"
	kernelcachesecurity "github.com/kserve/kserve/pkg/kernelcache/security"
	kernelcachetypes "github.com/kserve/kserve/pkg/kernelcache/types"
	"github.com/kserve/kserve/pkg/kernelcache/workload"
)

const (
	runtimeResultStateKey                        = "state"
	runtimeResultMessageKey                      = "message"
	runtimeResultReasonKey                       = "reason"
	runtimeResultImageReferenceKey               = "imageReference"
	runtimeResultCachePathsKey                   = "cachePaths"
	runtimeResultRuntimeInfoKey                  = "runtimeInfo"
	runtimeResultCapturedAtKey                   = "capturedAt"
	runtimeResultCompletedAtKey                  = "completedAt"
	runtimeResultCacheSizeBytesKey               = "cacheSizeBytes"
	runtimeResultSourcePodNameKey                = "sourcePodName"
	runtimeResultCaptureSessionIDKey             = "captureSessionID"
	runtimeResultSucceededState                  = "Succeeded"
	runtimeResultUnchangedState                  = "Unchanged"
	runtimeResultFailedState                     = "Failed"
	runtimeResultWaitingForWorkloadState         = "WaitingForWorkload"
	runtimeResultCapturingState                  = "Capturing"
	runtimeResultPushingState                    = "Pushing"
	kernelCacheCaptureReadyConditionType         = "Ready"
	kernelCacheCaptureReasonPending              = "Pending"
	kernelCacheCaptureReasonWaitingForWorkload   = "WaitingForWorkload"
	kernelCacheCaptureReasonCapturing            = "Capturing"
	kernelCacheCaptureReasonPushing              = "Pushing"
	kernelCacheCaptureReasonComplete             = "CaptureComplete"
	kernelCacheCaptureReasonUnchanged            = "CacheUnchanged"
	kernelCacheCaptureReasonFailed               = "CaptureFailed"
	kernelCacheCaptureReasonProducerGone         = "ProducerGone"
	kernelCacheCaptureReasonRetryingProducerGone = "RetryingProducerGone"
	kernelCacheCaptureReasonInvalidResult        = "InvalidRuntimeResult"
	runtimeInfoCommandHashKey                    = cacheidentity.CommandHashFactor
	runtimeInfoArgsHashKey                       = cacheidentity.ArgsHashFactor
	runtimeInfoModelURIHashKey                   = cacheidentity.ModelURIHashFactor
)

var errCaptureProducerGone = errors.New("capture producer Pod is no longer available")

// KernelCacheCaptureReconciler processes capture results for enabled InferenceServices.
type KernelCacheCaptureReconciler struct {
	client.Client
	Reader client.Reader
}

func (r *KernelCacheCaptureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	inferenceService := &v1beta1.InferenceService{}
	if err := r.Get(ctx, req.NamespacedName, inferenceService); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !inferenceService.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	kernelCacheConfig, err := kernelcacheconfig.Load(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !kernelCacheConfig.Enabled {
		return ctrl.Result{}, nil
	}

	captures := &v1alpha1.KernelCacheCaptureList{}
	if err := r.List(ctx, captures, client.InNamespace(inferenceService.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	for i := range captures.Items {
		if !captureReferencesInferenceService(&captures.Items[i], inferenceService) {
			continue
		}
		current, err := r.reconcileRuntimeResult(ctx, &captures.Items[i])
		if err != nil {
			return ctrl.Result{}, err
		}
		if current == nil {
			continue
		}
		if err := r.cleanupCaptureIdentities(ctx, current); err != nil {
			return ctrl.Result{}, err
		}
		deleteCapture, err := r.shouldDeleteGeneratedCapture(ctx, current, kernelCacheConfig)
		if err != nil {
			return ctrl.Result{}, err
		}
		if deleteCapture {
			uid := current.UID
			if err := r.Delete(ctx, current, client.Preconditions{UID: &uid}); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
			continue
		}
		if err := r.reconcileArtifactSigning(ctx, current, kernelCacheConfig); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *KernelCacheCaptureReconciler) cleanupCaptureIdentities(ctx context.Context, capture *v1alpha1.KernelCacheCapture) error {
	if capture.Labels[constants.KernelCacheCaptureGeneratedLabelKey] != "true" {
		return nil
	}
	if capture.Status.Phase != v1alpha1.KernelCacheCapturePhaseComplete &&
		capture.Status.Phase != v1alpha1.KernelCacheCapturePhaseUnchanged &&
		capture.Status.Phase != v1alpha1.KernelCacheCapturePhaseFailed {
		return nil
	}
	resources := []struct {
		object client.Object
		label  string
	}{
		{object: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: reporter.SecretName(capture.Name), Namespace: capture.Namespace}}, label: reporter.ManagedLabel},
		{object: &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: reporter.RoleBindingName(capture.Name), Namespace: capture.Namespace}}, label: reporter.ManagedLabel},
		{object: &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: reporter.RoleName(capture.Name), Namespace: capture.Namespace}}, label: reporter.ManagedLabel},
		{object: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: reporter.ServiceAccountName(capture.Name), Namespace: capture.Namespace}}, label: reporter.ManagedLabel},
	}
	for _, resource := range resources {
		if err := r.deleteOwnedCaptureObject(ctx, capture, resource.object, resource.label); err != nil {
			return err
		}
	}
	return nil
}

func (r *KernelCacheCaptureReconciler) deleteOwnedCaptureObject(ctx context.Context, capture *v1alpha1.KernelCacheCapture, object client.Object, managedLabel string) error {
	current := object.DeepCopyObject().(client.Object)
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	if err := reader.Get(ctx, client.ObjectKeyFromObject(object), current); err != nil {
		return client.IgnoreNotFound(err)
	}
	if current.GetLabels()[managedLabel] != "true" {
		return nil
	}
	owner := metav1.GetControllerOf(current)
	expected := metav1.NewControllerRef(capture, v1alpha1.SchemeGroupVersion.WithKind("KernelCacheCapture"))
	if owner == nil || owner.UID != expected.UID || owner.Name != expected.Name || owner.Kind != expected.Kind || owner.APIVersion != expected.APIVersion {
		return nil
	}
	uid := current.GetUID()
	resourceVersion := current.GetResourceVersion()
	return client.IgnoreNotFound(r.Delete(ctx, current, client.Preconditions{UID: &uid, ResourceVersion: &resourceVersion}))
}

func (r *KernelCacheCaptureReconciler) shouldDeleteGeneratedCapture(
	ctx context.Context,
	capture *v1alpha1.KernelCacheCapture,
	config *v1beta1.KernelCacheConfig,
) (bool, error) {
	if capture.Labels[constants.KernelCacheCaptureGeneratedLabelKey] != "true" {
		return false, nil
	}
	if isReopenableProducerGoneCapture(capture) {
		return config.AbandonedCapturePolicy == "delete", nil
	}
	if capture.Status.ActiveSession == nil || capture.Status.ActiveSession.PodName == "" {
		return false, nil
	}
	active := capture.Status.ActiveSession
	if active != nil && capture.Status.RuntimeResult != nil &&
		(capture.Status.RuntimeResult[runtimeResultCaptureSessionIDKey] != active.ID ||
			capture.Status.RuntimeResult[runtimeResultSourcePodNameKey] != active.PodName) {
		return false, nil
	}
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	pod := &corev1.Pod{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: capture.Namespace, Name: active.PodName}, pod); err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}
	if pod.Name != "" && pod.DeletionTimestamp == nil && pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
		return false, nil
	}

	if capture.Status.Phase == v1alpha1.KernelCacheCapturePhaseUnchanged &&
		capture.Status.RuntimeResult[runtimeResultStateKey] == runtimeResultUnchangedState &&
		capture.Status.Artifact == nil && capture.Status.KernelCacheRef == nil {
		return true, nil
	}
	if !captureInProgress(capture.Status.Phase) {
		return false, nil
	}
	if err := r.markCaptureProducerGone(ctx, capture); err != nil {
		return false, err
	}
	return config.AbandonedCapturePolicy == "delete", nil
}

func captureInProgress(phase v1alpha1.KernelCacheCapturePhase) bool {
	return phase == v1alpha1.KernelCacheCapturePhasePending ||
		phase == v1alpha1.KernelCacheCapturePhaseWaitingForWorkload ||
		phase == v1alpha1.KernelCacheCapturePhaseCapturing ||
		phase == v1alpha1.KernelCacheCapturePhasePushing
}

func isTerminalCapture(capture *v1alpha1.KernelCacheCapture) bool {
	if capture == nil {
		return false
	}
	switch capture.Status.Phase {
	case v1alpha1.KernelCacheCapturePhaseComplete,
		v1alpha1.KernelCacheCapturePhaseUnchanged,
		v1alpha1.KernelCacheCapturePhaseFailed:
		return true
	default:
		return false
	}
}

func (r *KernelCacheCaptureReconciler) markCaptureProducerGone(ctx context.Context, capture *v1alpha1.KernelCacheCapture) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.KernelCacheCapture{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(capture), current); err != nil {
			return client.IgnoreNotFound(err)
		}
		if !captureInProgress(current.Status.Phase) {
			return nil
		}
		desired := current.Status.DeepCopy()
		desired.Phase = v1alpha1.KernelCacheCapturePhaseFailed
		setCaptureCondition(desired, metav1.ConditionFalse, kernelCacheCaptureReasonProducerGone, "capture producer Pod no longer exists", current.Generation)
		current.Status = *desired
		return r.Status().Update(ctx, current)
	})
}

func (r *KernelCacheCaptureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.InferenceService{}).
		Watches(&v1alpha1.KernelCacheCapture{}, handler.EnqueueRequestsFromMapFunc(r.captureSourceRequests)).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.capturePodSourceRequests), builder.WithPredicates(capturePodPredicate())).
		Complete(r)
}

// capturePodPredicate accepts only Pods associated with an InferenceService.
func capturePodPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetLabels()[constants.InferenceServicePodLabelKey] != ""
	})
}

func (r *KernelCacheCaptureReconciler) capturePodSourceRequests(_ context.Context, object client.Object) []reconcile.Request {
	pod, ok := object.(*corev1.Pod)
	if !ok {
		return nil
	}
	inferenceServiceName := pod.Labels[constants.InferenceServicePodLabelKey]
	if inferenceServiceName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: client.ObjectKey{Namespace: pod.Namespace, Name: inferenceServiceName}}}
}

func captureSigningSpec(config *v1beta1.KernelCacheConfig) *v1alpha1.KernelCacheSigningSpec {
	if config.ArtifactSecurity.Mode != string(kernelcachetypes.ModeCert) || config.ArtifactSecurity.Cert.SigningProfileRef == "" {
		return nil
	}
	return &v1alpha1.KernelCacheSigningSpec{
		ProfileRef: &corev1.LocalObjectReference{Name: config.ArtifactSecurity.Cert.SigningProfileRef},
	}
}

func (r *KernelCacheCaptureReconciler) captureSourceRequests(_ context.Context, object client.Object) []reconcile.Request {
	capture, ok := object.(*v1alpha1.KernelCacheCapture)
	if !ok || capture.Spec.SourceRef.Kind != "InferenceService" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{
		Namespace: capture.Namespace,
		Name:      capture.Spec.SourceRef.Name,
	}}}
}

func captureReferencesInferenceService(capture *v1alpha1.KernelCacheCapture, inferenceService *v1beta1.InferenceService) bool {
	return capture.Namespace == inferenceService.Namespace &&
		capture.Spec.SourceRef.Kind == "InferenceService" &&
		capture.Spec.SourceRef.Name == inferenceService.Name
}

func (r *KernelCacheCaptureReconciler) reconcileRuntimeResult(ctx context.Context, capture *v1alpha1.KernelCacheCapture) (*v1alpha1.KernelCacheCapture, error) {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}

	key := client.ObjectKeyFromObject(capture)
	var current *v1alpha1.KernelCacheCapture
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current = &v1alpha1.KernelCacheCapture{}
		if err := reader.Get(ctx, key, current); err != nil {
			if apierrors.IsNotFound(err) {
				current = nil
				return nil
			}
			return err
		}
		return r.reconcileCurrentRuntimeResult(ctx, current)
	})
	if err != nil {
		return nil, err
	}
	return current, nil
}

func (r *KernelCacheCaptureReconciler) reconcileCurrentRuntimeResult(ctx context.Context, capture *v1alpha1.KernelCacheCapture) error {
	// Terminal captures must ignore late runtime reports from the selected
	// producer. Capture-scoped credentials are cleaned up separately.
	if isTerminalCapture(capture) {
		return nil
	}
	state := capture.Status.RuntimeResult[runtimeResultStateKey]
	if capture.Status.ActiveSession == nil {
		if sourcePodName := capture.Status.RuntimeResult[runtimeResultSourcePodNameKey]; sourcePodName != "" {
			capturePod, err := r.capturePod(ctx, capture, capture.Status.RuntimeResult)
			if err != nil {
				return err
			}
			if capturePod == nil {
				if state != runtimeResultSucceededState {
					if state != "" && !isKnownRuntimeResultState(state) {
						return r.failInvalidRuntimeResult(ctx, capture, state)
					}
					return nil
				}
			} else {
				sessionID := capture.Status.RuntimeResult[runtimeResultCaptureSessionIDKey]
				if sessionID == "" {
					return errors.New("runtime result captureSessionID is required for a capture claim")
				}
				capture.Status.ActiveSession = &v1alpha1.KernelCacheCaptureSession{
					ID:                 sessionID,
					PodName:            capturePod.Name,
					NodeName:           capturePod.Spec.NodeName,
					RequestedNodeGroup: capturePod.Annotations[constants.KernelCacheNodeGroupAnnotationKey],
				}
			}
		}
	}

	if state == "" {
		if capture.Status.ActiveSession == nil || capture.Status.Phase == v1alpha1.KernelCacheCapturePhasePending {
			return nil
		}
		desired := capture.Status.DeepCopy()
		desired.Phase = v1alpha1.KernelCacheCapturePhasePending
		setCaptureCondition(desired, metav1.ConditionFalse, kernelCacheCaptureReasonPending, "waiting for the selected capture session", capture.Generation)
		capture.Status = *desired
		return r.Status().Update(ctx, capture)
	}
	if active := capture.Status.ActiveSession; active != nil {
		if capture.Status.RuntimeResult[runtimeResultCaptureSessionIDKey] != active.ID ||
			capture.Status.RuntimeResult[runtimeResultSourcePodNameKey] != active.PodName {
			return nil
		}
	}
	if captureRuntimeResultComplete(capture) {
		return nil
	}

	desired := capture.Status.DeepCopy()
	switch state {
	case runtimeResultWaitingForWorkloadState:
		desired.Phase = v1alpha1.KernelCacheCapturePhaseWaitingForWorkload
		setCaptureCondition(desired, metav1.ConditionFalse, kernelCacheCaptureReasonWaitingForWorkload, runtimeMessage(capture.Status.RuntimeResult, "waiting for workload readiness"), capture.Generation)
	case runtimeResultCapturingState:
		desired.Phase = v1alpha1.KernelCacheCapturePhaseCapturing
		setCaptureCondition(desired, metav1.ConditionFalse, kernelCacheCaptureReasonCapturing, runtimeMessage(capture.Status.RuntimeResult, "capture is in progress"), capture.Generation)
	case runtimeResultPushingState:
		desired.Phase = v1alpha1.KernelCacheCapturePhasePushing
		setCaptureCondition(desired, metav1.ConditionFalse, kernelCacheCaptureReasonPushing, runtimeMessage(capture.Status.RuntimeResult, "cache image is being pushed"), capture.Generation)
	case runtimeResultSucceededState:
		err := r.completeCaptureStatus(ctx, desired, capture)
		if err != nil {
			desired.Phase = v1alpha1.KernelCacheCapturePhaseFailed
			reason := kernelCacheCaptureReasonInvalidResult
			if errors.Is(err, errCaptureProducerGone) {
				reason = kernelCacheCaptureReasonProducerGone
				desired.Artifact = nil
				desired.KernelCacheRef = nil
				desired.Signing = nil
			}
			setCaptureCondition(desired, metav1.ConditionFalse, reason, err.Error(), capture.Generation)
		} else {
			desired.Phase = v1alpha1.KernelCacheCapturePhaseComplete
			setCaptureCondition(desired, metav1.ConditionTrue, kernelCacheCaptureReasonComplete, "kernel cache capture completed", capture.Generation)
		}
	case runtimeResultUnchangedState:
		desired.Phase = v1alpha1.KernelCacheCapturePhaseUnchanged
		setCaptureCondition(desired, metav1.ConditionTrue, kernelCacheCaptureReasonUnchanged, "no new kernel cache directories were created", capture.Generation)
	case runtimeResultFailedState:
		desired.Phase = v1alpha1.KernelCacheCapturePhaseFailed
		setCaptureCondition(desired, metav1.ConditionFalse, kernelCacheCaptureReasonFailed, runtimeMessage(capture.Status.RuntimeResult, "kernel cache capture failed"), capture.Generation)
	default:
		return r.failInvalidRuntimeResult(ctx, capture, state)
	}

	if reflect.DeepEqual(capture.Status, *desired) {
		return nil
	}
	capture.Status = *desired
	return r.Status().Update(ctx, capture)
}

func (r *KernelCacheCaptureReconciler) failInvalidRuntimeResult(ctx context.Context, capture *v1alpha1.KernelCacheCapture, state string) error {
	desired := capture.Status.DeepCopy()
	desired.Phase = v1alpha1.KernelCacheCapturePhaseFailed
	desired.Artifact = nil
	desired.KernelCacheRef = nil
	desired.Signing = nil
	setCaptureCondition(
		desired,
		metav1.ConditionFalse,
		kernelCacheCaptureReasonInvalidResult,
		fmt.Sprintf("unsupported runtime result state %q", state),
		capture.Generation,
	)
	if reflect.DeepEqual(capture.Status, *desired) {
		return nil
	}
	capture.Status = *desired
	return r.Status().Update(ctx, capture)
}

func isKnownRuntimeResultState(state string) bool {
	switch state {
	case runtimeResultWaitingForWorkloadState,
		runtimeResultCapturingState,
		runtimeResultPushingState,
		runtimeResultSucceededState,
		runtimeResultUnchangedState,
		runtimeResultFailedState:
		return true
	default:
		return false
	}
}

func captureRuntimeResultComplete(capture *v1alpha1.KernelCacheCapture) bool {
	if capture.Status.Phase != v1alpha1.KernelCacheCapturePhaseComplete || capture.Status.Artifact == nil {
		return false
	}
	switch capture.Status.RuntimeResult[runtimeResultStateKey] {
	case runtimeResultSucceededState:
		return capture.Status.Artifact.ImageReference == capture.Status.RuntimeResult[runtimeResultImageReferenceKey]
	case runtimeResultUnchangedState:
		return capture.Status.KernelCacheRef != nil
	default:
		return false
	}
}

func (r *KernelCacheCaptureReconciler) reconcileArtifactSigning(
	ctx context.Context,
	capture *v1alpha1.KernelCacheCapture,
	config *v1beta1.KernelCacheConfig,
) error {
	if !captureRuntimeResultComplete(capture) {
		return nil
	}
	mode := config.ArtifactSecurity.Mode
	if mode == string(kernelcachetypes.ModeDisabled) {
		mode = "none"
	}
	if signing := capture.Status.Signing; signing != nil && signing.Mode == mode &&
		(signing.State == v1alpha1.KernelCacheArtifactSecurityStateSkipped ||
			signing.State == v1alpha1.KernelCacheArtifactSecurityStateSucceeded) {
		return nil
	}
	if config.ArtifactSecurity.Mode == string(kernelcachetypes.ModeCert) &&
		(capture.Spec.Signing == nil || capture.Spec.Signing.ProfileRef == nil || capture.Spec.Signing.ProfileRef.Name == "") {
		return r.updateCaptureSigningStatus(ctx, capture, v1alpha1.KernelCacheSigningStatus{
			Mode:    mode,
			State:   v1alpha1.KernelCacheArtifactSecurityStateFailed,
			Reason:  "SigningProfileNotFound",
			Message: "spec.signing.profileRef.name is required for cert signing",
		})
	}

	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	signer, err := kernelcachesecurity.NewSigner(ctx, config.ArtifactSecurity.ToSecurityConfig(), kernelcachesecurity.NewKubernetesSecretSource(reader))
	if err != nil {
		return r.updateCaptureSigningStatus(ctx, capture, v1alpha1.KernelCacheSigningStatus{
			Mode:    mode,
			State:   v1alpha1.KernelCacheArtifactSecurityStateFailed,
			Reason:  "SignerUnavailable",
			Message: err.Error(),
		})
	}

	profileRef := ""
	if capture.Spec.Signing != nil && capture.Spec.Signing.ProfileRef != nil {
		profileRef = capture.Namespace + "/" + capture.Spec.Signing.ProfileRef.Name
	}
	result, err := signer.Sign(ctx, kernelcachetypes.SignRequest{
		ImageRef:   capture.Status.Artifact.ImageReference,
		ProfileRef: profileRef,
	})
	if err != nil {
		statusErr := r.updateCaptureSigningStatus(ctx, capture, v1alpha1.KernelCacheSigningStatus{
			Mode:    mode,
			State:   v1alpha1.KernelCacheArtifactSecurityStateFailed,
			Reason:  "SigningFailed",
			Message: err.Error(),
		})
		if statusErr != nil {
			return statusErr
		}
		return err
	}

	if result.Mode == kernelcachetypes.ModeDisabled {
		return r.updateCaptureSigningStatus(ctx, capture, v1alpha1.KernelCacheSigningStatus{
			Mode:    captureSigningMode(result.Mode),
			State:   v1alpha1.KernelCacheArtifactSecurityStateSkipped,
			Reason:  "SigningNotConfigured",
			Message: "artifact signing is not configured",
		})
	}
	parts := strings.SplitN(capture.Status.Artifact.ImageReference, "@", 2)
	if len(parts) != 2 {
		return r.updateCaptureSigningStatus(ctx, capture, v1alpha1.KernelCacheSigningStatus{
			Mode:    string(result.Mode),
			State:   v1alpha1.KernelCacheArtifactSecurityStateFailed,
			Reason:  "InvalidArtifactReference",
			Message: "artifact imageReference must be digest-pinned",
		})
	}
	expectedDigest := parts[1]
	if result.Digest != expectedDigest {
		return r.updateCaptureSigningStatus(ctx, capture, v1alpha1.KernelCacheSigningStatus{
			Mode:    string(result.Mode),
			State:   v1alpha1.KernelCacheArtifactSecurityStateFailed,
			Reason:  "SignedDigestMismatch",
			Message: fmt.Sprintf("signer returned digest %q for artifact digest %q", result.Digest, expectedDigest),
		})
	}

	now := metav1.Now()
	return r.updateCaptureSigningStatus(ctx, capture, v1alpha1.KernelCacheSigningStatus{
		Mode:     string(result.Mode),
		State:    v1alpha1.KernelCacheArtifactSecurityStateSucceeded,
		Signed:   true,
		Reason:   "SigningSucceeded",
		Message:  "artifact signing completed",
		SignedAt: &now,
	})
}

func captureSigningMode(mode kernelcachetypes.Mode) string {
	if mode == kernelcachetypes.ModeDisabled {
		return "none"
	}
	return string(mode)
}

func (r *KernelCacheCaptureReconciler) updateCaptureSigningStatus(
	ctx context.Context,
	capture *v1alpha1.KernelCacheCapture,
	signing v1alpha1.KernelCacheSigningStatus,
) error {
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.KernelCacheCapture{}
		if err := reader.Get(ctx, client.ObjectKeyFromObject(capture), current); err != nil {
			return err
		}
		desired := current.Status.DeepCopy()
		desired.Signing = &signing
		switch signing.State {
		case v1alpha1.KernelCacheArtifactSecurityStateFailed:
			setCaptureCondition(desired, metav1.ConditionFalse, signing.Reason, signing.Message, current.Generation)
		case v1alpha1.KernelCacheArtifactSecurityStateSkipped:
			setCaptureCondition(desired, metav1.ConditionTrue, kernelCacheCaptureReasonComplete, "kernel cache capture completed", current.Generation)
		case v1alpha1.KernelCacheArtifactSecurityStateSucceeded:
			setCaptureCondition(desired, metav1.ConditionTrue, kernelCacheCaptureReasonComplete, "kernel cache capture completed and artifact signing succeeded", current.Generation)
		}
		if reflect.DeepEqual(current.Status, *desired) {
			return nil
		}
		current.Status = *desired
		return r.Status().Update(ctx, current)
	})
}

func (r *KernelCacheCaptureReconciler) completeCaptureStatus(ctx context.Context, status *v1alpha1.KernelCacheCaptureStatus, capture *v1alpha1.KernelCacheCapture) error {
	result := capture.Status.RuntimeResult
	imageReference := result[runtimeResultImageReferenceKey]
	if !isDigestReference(imageReference) {
		return errors.New("runtime result imageReference must be a digest-pinned OCI reference")
	}

	cachePaths, err := captureCachePaths(capture, result)
	if err != nil {
		return err
	}
	if capturedAt := runtimeTime(result); capturedAt != nil {
		status.CapturedAt = capturedAt
	} else if status.CapturedAt == nil {
		now := metav1.Now()
		status.CapturedAt = &now
	}
	if value := result[runtimeResultCacheSizeBytesKey]; value != "" {
		size, err := strconv.ParseInt(value, 10, 64)
		if err != nil || size < 0 {
			return errors.New("runtime result cacheSizeBytes must be a non-negative integer")
		}
		status.CapturedCacheSizeBytes = &size
	}

	runtimeInfo, err := parseRuntimeInfo(result)
	if err != nil {
		return err
	}
	capturePod, err := r.capturePod(ctx, capture, result)
	if err != nil {
		return err
	}
	if capturePod == nil {
		if result[runtimeResultSourcePodNameKey] == "" {
			return errors.New("runtime result sourcePodName is required")
		}
		return errCaptureProducerGone
	}
	for index := range cachePaths {
		containerName, err := kernelcacheutil.ResolveRuntimeContainerName(capturePod.Spec.Containers, cachePaths[index].ContainerName)
		if err != nil {
			return err
		}
		cachePaths[index].ContainerName = containerName
		container := findContainer(capturePod.Spec.Containers, containerName)
		containerPath, err := kernelcacheutil.ResolveContainerPath(container, cachePaths[index].ContainerPath)
		if err != nil {
			return err
		}
		cachePaths[index].ContainerPath = containerPath
		ociPath, err := kernelcacheutil.ResolveOCIPath(cachePaths[index].OCIPath)
		if err != nil {
			return err
		}
		cachePaths[index].OCIPath = ociPath
	}
	runtimeImage, err := runtimeImage(capturePod, cachePaths[0].ContainerName)
	if err != nil {
		return err
	}
	if runtimeImage == "" {
		return errors.New("capture container image is required")
	}
	runtimeConfigFactors := cacheidentity.ExtractRuntimeConfigFactors(findContainer(capturePod.Spec.Containers, cachePaths[0].ContainerName))
	cacheIdentity, err := cacheidentity.Build(cacheidentity.KernelCacheIdentityFactors{
		Namespace:            capture.Namespace,
		WorkloadKind:         capture.Spec.SourceRef.Kind,
		WorkloadName:         capture.Spec.SourceRef.Name,
		RuntimeImage:         runtimeImage,
		ModelURIHash:         runtimeInfo[runtimeInfoModelURIHashKey],
		CommandHash:          runtimeInfo[runtimeInfoCommandHashKey],
		ArgsHash:             runtimeInfo[runtimeInfoArgsHashKey],
		RuntimeConfigFactors: runtimeConfigFactors,
	})
	if err != nil {
		return err
	}
	artifact := &v1alpha1.KernelCacheArtifact{
		ImageReference: imageReference,
		CachePaths:     cachePaths,
		Identity:       cacheIdentity,
	}
	if !reflect.DeepEqual(status.Artifact, artifact) {
		status.KernelCacheRef = nil
		status.Signing = nil
	}
	status.Artifact = artifact
	return nil
}

func (r *KernelCacheCaptureReconciler) capturePod(ctx context.Context, capture *v1alpha1.KernelCacheCapture, result map[string]string) (*corev1.Pod, error) {
	name := result[runtimeResultSourcePodNameKey]
	if name == "" {
		return nil, nil
	}
	pod := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: capture.Namespace, Name: name}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if pod.DeletionTimestamp != nil {
		return nil, nil
	}
	if pod.Labels[constants.InferenceServicePodLabelKey] != capture.Spec.SourceRef.Name {
		return nil, errors.New("capture Pod does not belong to sourceRef")
	}
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	if capture.Labels[constants.KernelCacheCaptureGeneratedLabelKey] == "true" {
		podRevisionID := pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
		if podRevisionID == "" {
			return nil, errors.New("generated capture Pod revision label is required")
		}
		revision, err := workload.ResolveDeploymentBackedInferenceServiceRevision(ctx, reader, pod)
		if err != nil {
			return nil, err
		}
		if revision == nil || revision.Source.Name != capture.Spec.SourceRef.Name {
			return nil, errors.New("capture Pod workload revision does not match sourceRef")
		}
		replicaSetRevisionID := revision.ReplicaSet.Labels[appsv1.DefaultDeploymentUniqueLabelKey]
		if replicaSetRevisionID == "" || podRevisionID != replicaSetRevisionID {
			return nil, errors.New("capture Pod workload revision does not match ReplicaSet")
		}
		owner := metav1.GetControllerOf(capture)
		if owner == nil || owner.Kind != "ReplicaSet" || owner.APIVersion != appsv1.SchemeGroupVersion.String() || owner.UID != revision.ReplicaSet.UID {
			return nil, errors.New("capture Pod workload revision does not match capture owner")
		}
	} else if resolved, err := workload.ResolveDeploymentBackedInferenceService(ctx, reader, pod); err != nil {
		return nil, err
	} else if resolved != nil && resolved.Name != capture.Spec.SourceRef.Name {
		return nil, errors.New("capture Pod ownership chain does not match sourceRef")
	}
	return pod, nil
}

func runtimeImage(pod *corev1.Pod, containerName string) (string, error) {
	container := findContainer(pod.Spec.Containers, containerName)
	if container == nil {
		return "", fmt.Errorf("capture container %q was not found in source Pod", containerName)
	}
	return container.Image, nil
}

func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for index := range containers {
		if containers[index].Name == name {
			return &containers[index]
		}
	}
	return nil
}

func parseRuntimeInfo(result map[string]string) (map[string]string, error) {
	value := result[runtimeResultRuntimeInfoKey]
	if value == "" {
		return nil, errors.New("runtime result runtimeInfo with modelURIHash is required")
	}
	info := map[string]string{}
	if err := json.Unmarshal([]byte(value), &info); err != nil {
		return nil, fmt.Errorf("runtime result runtimeInfo is invalid: %w", err)
	}
	if info[runtimeInfoModelURIHashKey] == "" {
		return nil, errors.New("runtimeInfo.modelURIHash is required")
	}
	for _, key := range []string{runtimeInfoCommandHashKey, runtimeInfoArgsHashKey, runtimeInfoModelURIHashKey} {
		if value := info[key]; value != "" && !cacheidentity.IsSHA256(value) {
			return nil, fmt.Errorf("runtimeInfo.%s must be a SHA-256 value", key)
		}
	}
	return info, nil
}

func captureCachePaths(capture *v1alpha1.KernelCacheCapture, result map[string]string) ([]v1alpha1.KernelCachePath, error) {
	if len(capture.Spec.CachePaths) > 0 {
		return append([]v1alpha1.KernelCachePath(nil), capture.Spec.CachePaths...), nil
	}
	value := result[runtimeResultCachePathsKey]
	if value == "" {
		return nil, errors.New("runtime result cachePaths is required when capture spec cachePaths is empty")
	}
	var paths []v1alpha1.KernelCachePath
	if err := json.Unmarshal([]byte(value), &paths); err != nil {
		return nil, fmt.Errorf("runtime result cachePaths is invalid: %w", err)
	}
	if len(paths) == 0 {
		return nil, errors.New("runtime result cachePaths must not be empty")
	}
	return paths, nil
}

func runtimeMessage(result map[string]string, fallback string) string {
	if message := result[runtimeResultMessageKey]; message != "" {
		return message
	}
	if reason := result[runtimeResultReasonKey]; reason != "" {
		return reason
	}
	return fallback
}

func runtimeTime(result map[string]string) *metav1.Time {
	value := result[runtimeResultCapturedAtKey]
	if value == "" {
		value = result[runtimeResultCompletedAtKey]
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	observed := metav1.NewTime(parsed)
	return &observed
}

func setCaptureCondition(status *v1alpha1.KernelCacheCaptureStatus, conditionStatus metav1.ConditionStatus, reason, message string, generation int64) {
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               kernelCacheCaptureReadyConditionType,
		Status:             conditionStatus,
		ObservedGeneration: generation,
		Reason:             reason,
		Message:            message,
	})
}

func isDigestReference(value string) bool {
	if !strings.Contains(value, "@sha256:") {
		return false
	}
	parts := strings.Split(value, "@sha256:")
	return len(parts) == 2 && parts[0] != "" && len(parts[1]) == 64 && isLowerHex(parts[1])
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return value != ""
}
