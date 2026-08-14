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

// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachecaptures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachecaptures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcachecaptures/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.kserve.io,resources=kernelcaches,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=serving.kserve.io,resources=inferenceservices,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
package kernelcachecapture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/webhook/admission/pod"
)

const (
	// KCCFinalizerName is the name of the finalizer for KernelCacheCapture
	KCCFinalizerName = "kernelcachecapture.serving.kserve.io/finalizer"
)

// PodExecutor abstracts pod exec for testability
type PodExecutor interface {
	ExecInPod(ctx context.Context, namespace, podName, containerName string, cmd []string) (stdout string, stderr string, exitCode int, err error)
}

// defaultPodExecutor implements PodExecutor using SPDY
type defaultPodExecutor struct {
	client       client.Client
	clientset    kubernetes.Interface
	clientConfig *rest.Config
}

func (e *defaultPodExecutor) ExecInPod(ctx context.Context, namespace, podName, containerName string, cmd []string) (stdout string, stderr string, exitCode int, err error) {
	p := &corev1.Pod{}
	if err := e.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: podName}, p); err != nil {
		return "", "", -1, fmt.Errorf("failed to get pod: %w", err)
	}

	req := e.clientset.CoreV1().RESTClient().
		Post().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(e.clientConfig, "POST", req.URL())
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to create executor: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	execErr := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdoutBuf,
		Stderr: &stderrBuf,
	})

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if execErr != nil {
		var exitErr utilexec.CodeExitError
		if errors.As(execErr, &exitErr) {
			exitCode = exitErr.Code
			err = nil
		} else {
			exitCode = -1
			err = execErr
		}
	} else {
		exitCode = 0
	}

	return stdout, stderr, exitCode, err
}

// KernelCacheCaptureReconciler reconciles a KernelCacheCapture object
type KernelCacheCaptureReconciler struct {
	client.Client
	Clientset    kubernetes.Interface
	ClientConfig *rest.Config
	Log          logr.Logger
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	podExecutor  PodExecutor
}

// Reconcile implements the reconciliation loop for KernelCacheCapture
func (r *KernelCacheCaptureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("kernelcachecapture", req.NamespacedName)

	// Fetch the KernelCacheCapture instance
	kcc := &v1alpha1.KernelCacheCapture{}
	if err := r.Get(ctx, req.NamespacedName, kcc); err != nil {
		if apierr.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	log.Info("Reconciling KernelCacheCapture",
		"trigger", kcc.Spec.Trigger,
		"phase", kcc.Status.Phase,
		"targetImage", kcc.Spec.TargetImage,
		"deletionTimestamp", kcc.DeletionTimestamp)

	// Handle deletion
	if !kcc.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, kcc, log)
	}

	// Add finalizer if not present
	if !containsString(kcc.Finalizers, KCCFinalizerName) {
		kcc.Finalizers = append(kcc.Finalizers, KCCFinalizerName)
		if err := r.Update(ctx, kcc); err != nil {
			log.Error(err, "Failed to add finalizer")
			return reconcile.Result{}, err
		}
		log.Info("Added finalizer to KCC")
		return reconcile.Result{Requeue: true}, nil
	}

	// Initialize phase to Pending if not set
	if kcc.Status.Phase == "" {
		kcc.Status.Phase = v1alpha1.KernelCacheCapturePhasePending
		if err := r.Status().Update(ctx, kcc); err != nil {
			log.Error(err, "Failed to initialize phase to Pending")
			return reconcile.Result{}, err
		}
		log.Info("Initialized KCC phase to Pending")
		return reconcile.Result{Requeue: true}, nil
	}

	// If not triggered yet, nothing to do
	if !kcc.Spec.Trigger {
		log.Info("KCC not triggered yet, skipping reconciliation")
		return reconcile.Result{}, nil
	}

	// Handle phase transitions based on current phase
	switch kcc.Status.Phase {
	case "": // Not initialized yet
		fallthrough
	case v1alpha1.KernelCacheCapturePhasePending:
		return r.handlePendingPhase(ctx, kcc, log)
	case v1alpha1.KernelCacheCapturePhaseCapturing:
		return r.handleCapturingPhase(ctx, kcc, log)
	case v1alpha1.KernelCacheCapturePhasePushing:
		// TODO: Monitor push progress
		log.Info("KCC in Pushing phase - monitoring not yet implemented")
		return reconcile.Result{}, nil
	case v1alpha1.KernelCacheCapturePhaseComplete:
		// TODO: Auto-create KernelCache if configured
		log.Info("KCC in Complete phase")
		return reconcile.Result{}, nil
	case v1alpha1.KernelCacheCapturePhaseFailed:
		log.Info("KCC in Failed phase, not retrying")
		return reconcile.Result{}, nil
	default:
		log.Info("Unknown phase", "phase", kcc.Status.Phase)
		return reconcile.Result{}, nil
	}
}

// handlePendingPhase transitions from Pending to Capturing by finding the pod
func (r *KernelCacheCaptureReconciler) handlePendingPhase(ctx context.Context, kcc *v1alpha1.KernelCacheCapture, log logr.Logger) (ctrl.Result, error) {
	log.Info("Handling Pending phase - searching for pod with cache-capture sidecar")

	// Find pod with the cache-capture label pointing to this KCC
	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		"serving.kserve.io/cache-capture": kcc.Name,
	}
	if err := r.List(ctx, podList, client.InNamespace(kcc.Namespace), labelSelector); err != nil {
		log.Error(err, "Failed to list pods")
		return r.updatePhase(ctx, kcc, v1alpha1.KernelCacheCapturePhaseFailed, "Failed to find pod")
	}

	if len(podList.Items) == 0 {
		log.Info("No pod found yet with cache-capture label", "label", kcc.Name)
		// Pod hasn't been created yet, requeue and wait
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if len(podList.Items) > 1 {
		log.Info("Multiple pods found with cache-capture label - using first one", "count", len(podList.Items))
	}

	pod := &podList.Items[0]
	log.Info("Found pod with cache-capture sidecar",
		"pod", pod.Name,
		"podPhase", pod.Status.Phase)

	// Verify pod is running before transitioning to Capturing
	if pod.Status.Phase != corev1.PodRunning {
		log.Info("Pod not running yet, waiting", "podPhase", pod.Status.Phase)
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Verify cache-capture container is ready
	containerReady := false
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == "cache-capture" && containerStatus.Ready {
			containerReady = true
			break
		}
	}

	if !containerReady {
		log.Info("Cache-capture container not ready yet, waiting")
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Update status: store pod name and transition to Capturing
	kcc.Status.PodName = pod.Name
	kcc.Status.Phase = v1alpha1.KernelCacheCapturePhaseCapturing

	if err := r.Status().Update(ctx, kcc); err != nil {
		log.Error(err, "Failed to update status")
		return reconcile.Result{}, err
	}

	r.Recorder.Eventf(kcc, corev1.EventTypeNormal, "Capturing",
		"Started cache capture from pod %s", pod.Name)
	log.Info("Transitioned to Capturing phase", "pod", pod.Name)

	return reconcile.Result{}, nil
}

// handleCapturingPhase executes MCV build and buildah push
func (r *KernelCacheCaptureReconciler) handleCapturingPhase(ctx context.Context, kcc *v1alpha1.KernelCacheCapture, log logr.Logger) (ctrl.Result, error) {
	log.Info("Starting MCV build and push")

	// Resolve cache path from preset or override
	cachePath := pod.ResolveCachePath(kcc.Spec.CachePreset, kcc.Spec.CachePath)

	// STEP 1: Build with MCV (local only, no push yet)
	log.Info("Step 1: Building cache image with MCV", "cachePath", cachePath, "targetImage", kcc.Spec.TargetImage)
	buildCmd := []string{
		"/mcv",
		"-c",            // create mode
		"-d", cachePath, // cache directory
		"-i", kcc.Spec.TargetImage, // target image
		"--builder", "buildah", // use buildah
	}

	buildStdout, buildStderr, buildExit, err := r.podExecutor.ExecInPod(
		ctx, kcc.Namespace, kcc.Status.PodName, "cache-capture", buildCmd)

	if err != nil || buildExit != 0 {
		msg := fmt.Sprintf("MCV build failed (exit %d): %s", buildExit, buildStderr)
		log.Error(err, msg, "stdout", buildStdout)
		return r.updatePhase(ctx, kcc, v1alpha1.KernelCacheCapturePhaseFailed, msg)
	}

	log.Info("MCV build complete", "output", buildStdout)

	// STEP 2: Push with buildah
	log.Info("Step 2: Pushing image to registry")
	pushCmd := []string{
		"buildah", "push",
		"--storage-driver", "vfs",
		"--tls-verify=false", // Allow insecure registries (e.g., KIND registry)
	}
	if kcc.Spec.RegistrySecretRef != nil {
		pushCmd = append(pushCmd, "--authfile", "/var/run/secrets/registry-creds/.dockerconfigjson")
	}
	pushCmd = append(pushCmd, kcc.Spec.TargetImage)

	pushStdout, pushStderr, pushExit, err := r.podExecutor.ExecInPod(
		ctx, kcc.Namespace, kcc.Status.PodName, "cache-capture", pushCmd)

	if err != nil || pushExit != 0 {
		msg := fmt.Sprintf("buildah push failed (exit %d): %s", pushExit, pushStderr)
		log.Error(err, msg, "stdout", pushStdout)
		return r.updatePhase(ctx, kcc, v1alpha1.KernelCacheCapturePhaseFailed, msg)
	}

	log.Info("Push complete", "output", pushStdout)

	// Parse digest from buildah push output
	digest := extractDigestFromOutput(pushStdout)
	if digest == "" {
		log.Info("Warning: Could not parse digest from buildah output", "stdout", pushStdout)
	}

	// Update to Complete
	now := metav1.Now()
	kcc.Status.Phase = v1alpha1.KernelCacheCapturePhaseComplete
	kcc.Status.ImageDigest = digest
	kcc.Status.CapturedAt = &now
	kcc.Status.DetectedCachePath = cachePath

	if err := r.Status().Update(ctx, kcc); err != nil {
		log.Error(err, "Failed to update status to Complete")
		return reconcile.Result{}, err
	}

	r.Recorder.Eventf(kcc, corev1.EventTypeNormal, "CaptureComplete",
		"Successfully captured and pushed cache image %s", kcc.Spec.TargetImage)

	log.Info("Cache capture complete",
		"targetImage", kcc.Spec.TargetImage,
		"digest", digest,
		"cachePath", cachePath)

	// Auto-create KernelCache if enabled
	if err := r.createKernelCacheIfEnabled(ctx, kcc, log); err != nil {
		// Log error but don't fail the whole capture - user can manually create KC
		log.Error(err, "Failed to auto-create KernelCache")
		r.Recorder.Eventf(kcc, corev1.EventTypeWarning, "KernelCacheCreateFailed",
			"Failed to auto-create KernelCache: %v", err)
	}

	return reconcile.Result{}, nil
}

// extractDigestFromOutput parses the digest from buildah push output
func extractDigestFromOutput(output string) string {
	// buildah push outputs lines like:
	// Getting image source signatures
	// Copying blob sha256:...
	// Copying config sha256:...
	// Writing manifest to image destination
	// sha256:abc1234567890abcdef...
	//
	// The digest is the last line starting with "sha256:"
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "sha256:") {
			return line
		}
	}
	return ""
}

// updatePhase is a helper to update the phase and requeue
func (r *KernelCacheCaptureReconciler) updatePhase(ctx context.Context, kcc *v1alpha1.KernelCacheCapture, phase v1alpha1.KernelCacheCapturePhase, message string) (ctrl.Result, error) {
	kcc.Status.Phase = phase
	if err := r.Status().Update(ctx, kcc); err != nil {
		return reconcile.Result{}, err
	}
	if phase == v1alpha1.KernelCacheCapturePhaseFailed {
		r.Recorder.Eventf(kcc, corev1.EventTypeWarning, "Failed", message)
	}
	return reconcile.Result{}, nil
}

// createKernelCacheIfEnabled creates a KernelCache CR if auto-create is enabled
func (r *KernelCacheCaptureReconciler) createKernelCacheIfEnabled(ctx context.Context, kcc *v1alpha1.KernelCacheCapture, log logr.Logger) error {
	// Check if auto-create is enabled (defaults to true if not specified)
	if kcc.Spec.CreateKernelCache == nil || (kcc.Spec.CreateKernelCache.Enabled != nil && !*kcc.Spec.CreateKernelCache.Enabled) {
		log.Info("Auto-create KernelCache disabled, skipping")
		return nil
	}

	// Determine KC name (default to KCC name)
	kcName := kcc.Spec.CreateKernelCache.Name
	if kcName == "" {
		kcName = kcc.Name
	}

	// Determine mount type (default to imageVolume)
	mountType := kcc.Spec.CreateKernelCache.MountType
	if mountType == "" {
		mountType = v1alpha1.KernelCacheMountTypeImageVolume
	}

	// Build KernelCache CR
	kc := &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kcName,
			Namespace: kcc.Namespace,
			// Set owner reference for garbage collection
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         kcc.APIVersion,
					Kind:               kcc.Kind,
					Name:               kcc.Name,
					UID:                kcc.UID,
					Controller:         ptr.To(true),
					BlockOwnerDeletion: ptr.To(true),
				},
			},
		},
		Spec: v1alpha1.KernelCacheSpec{
			Image:     kcc.Spec.TargetImage,
			MountType: mountType,
		},
	}

	// Note: ImagePullSecrets are handled at the pod level when KC is mounted
	// For imageVolume mode: ISVC pod's imagePullSecrets are used
	// For PVC mode: extraction job pod template can specify secrets
	// The PullSecretRef in CreateKernelCache config is not directly used here

	// Check if KC already exists
	existingKC := &v1alpha1.KernelCache{}
	err := r.Get(ctx, types.NamespacedName{Name: kcName, Namespace: kcc.Namespace}, existingKC)
	if err == nil {
		// KC already exists - check if it's owned by this KCC
		isOwned := false
		for _, owner := range existingKC.OwnerReferences {
			if owner.UID == kcc.UID {
				isOwned = true
				break
			}
		}

		if isOwned {
			log.Info("KernelCache already exists and is owned by this KCC", "kcName", kcName)
			// Update status reference
			kcc.Status.KernelCacheRef = &v1alpha1.NamespacedName{
				Name:      kcName,
				Namespace: kcc.Namespace,
			}
			if err := r.Status().Update(ctx, kcc); err != nil {
				return fmt.Errorf("failed to update status with KC ref: %w", err)
			}
			return nil
		}

		// KC exists but not owned by this KCC - don't overwrite
		log.Info("KernelCache already exists but not owned by this KCC, skipping auto-create", "kcName", kcName)
		return fmt.Errorf("KernelCache %s already exists and is not owned by this KCC", kcName)
	}

	if !apierr.IsNotFound(err) {
		return fmt.Errorf("failed to check if KernelCache exists: %w", err)
	}

	// Create the KC
	log.Info("Creating auto-created KernelCache",
		"kcName", kcName,
		"image", kcc.Spec.TargetImage,
		"mountType", mountType)

	if err := r.Create(ctx, kc); err != nil {
		return fmt.Errorf("failed to create KernelCache: %w", err)
	}

	// Update status with KC reference
	kcc.Status.KernelCacheRef = &v1alpha1.NamespacedName{
		Name:      kcName,
		Namespace: kcc.Namespace,
	}

	if err := r.Status().Update(ctx, kcc); err != nil {
		return fmt.Errorf("failed to update status with KC ref: %w", err)
	}

	r.Recorder.Eventf(kcc, corev1.EventTypeNormal, "KernelCacheCreated",
		"Auto-created KernelCache %s from captured image", kcName)

	log.Info("Successfully auto-created KernelCache", "kcName", kcName)

	return nil
}

// handleDeletion handles KCC deletion and finalizer cleanup
func (r *KernelCacheCaptureReconciler) handleDeletion(ctx context.Context, kcc *v1alpha1.KernelCacheCapture, log logr.Logger) (ctrl.Result, error) {
	log.Info("Handling KCC deletion")

	// Check if finalizer is present
	if !containsString(kcc.Finalizers, KCCFinalizerName) {
		log.Info("Finalizer not present, allowing deletion")
		return reconcile.Result{}, nil
	}

	// Check if we created a KernelCache (check status.kernelCacheRef)
	if kcc.Status.KernelCacheRef == nil {
		log.Info("No auto-created KernelCache to clean up, removing finalizer")
		return r.removeFinalizer(ctx, kcc, log)
	}

	// Check if the owned KernelCache exists
	kcName := kcc.Status.KernelCacheRef.Name
	kcNamespace := kcc.Status.KernelCacheRef.Namespace
	if kcNamespace == "" {
		kcNamespace = kcc.Namespace
	}

	kc := &v1alpha1.KernelCache{}
	err := r.Get(ctx, types.NamespacedName{Name: kcName, Namespace: kcNamespace}, kc)
	if err != nil {
		if apierr.IsNotFound(err) {
			// KC already deleted, can remove finalizer
			log.Info("Owned KernelCache already deleted, removing finalizer", "kcName", kcName)
			return r.removeFinalizer(ctx, kcc, log)
		}
		log.Error(err, "Failed to get owned KernelCache", "kcName", kcName)
		return reconcile.Result{}, err
	}

	// Check if this KC is owned by this KCC
	isOwned := false
	for _, owner := range kc.OwnerReferences {
		if owner.UID == kcc.UID {
			isOwned = true
			break
		}
	}

	if !isOwned {
		// KC exists but not owned by us, safe to delete KCC
		log.Info("KernelCache exists but not owned by this KCC, removing finalizer", "kcName", kcName)
		return r.removeFinalizer(ctx, kcc, log)
	}

	// KC is owned by us - check if it's in use
	isvcList := &v1beta1.InferenceServiceList{}
	if err := r.List(ctx, isvcList, client.InNamespace(kcNamespace)); err != nil {
		log.Error(err, "Failed to list InferenceServices")
		return reconcile.Result{}, err
	}

	// Find ISVCs using this KC
	var usingISVCs []string
	for _, isvc := range isvcList.Items {
		// Check if ISVC has the KC label
		if kcLabel, ok := isvc.Labels[constants.KernelCacheLabel]; ok && kcLabel == kcName {
			usingISVCs = append(usingISVCs, isvc.Name)
		}
	}

	if len(usingISVCs) > 0 {
		// KC is in use, block deletion
		msg := fmt.Sprintf("Cannot delete KernelCacheCapture: owned KernelCache %q is in use by %d InferenceService(s): %v. Delete these InferenceServices first, or remove the ownerReference from the KernelCache to orphan it.",
			kcName, len(usingISVCs), usingISVCs)
		log.Info("Blocking KCC deletion - owned KC is in use",
			"kcName", kcName,
			"inUseBy", usingISVCs)

		r.Recorder.Eventf(kcc, corev1.EventTypeWarning, "DeleteBlocked", msg)

		// Don't return error - just requeue after a delay to check again
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// KC is not in use, safe to delete
	log.Info("Owned KernelCache is not in use, allowing deletion", "kcName", kcName)
	return r.removeFinalizer(ctx, kcc, log)
}

// removeFinalizer removes the finalizer from KCC and updates it
func (r *KernelCacheCaptureReconciler) removeFinalizer(ctx context.Context, kcc *v1alpha1.KernelCacheCapture, log logr.Logger) (ctrl.Result, error) {
	kcc.Finalizers = removeString(kcc.Finalizers, KCCFinalizerName)
	if err := r.Update(ctx, kcc); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return reconcile.Result{}, err
	}
	log.Info("Removed finalizer, KCC will be deleted")
	return reconcile.Result{}, nil
}

// containsString checks if a slice contains a string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString removes a string from a slice
func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager
func (r *KernelCacheCaptureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.podExecutor == nil {
		r.podExecutor = &defaultPodExecutor{
			client:       r.Client,
			clientset:    r.Clientset,
			clientConfig: r.ClientConfig,
		}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KernelCacheCapture{}).
		Owns(&v1alpha1.KernelCache{}).
		Complete(r)
}
