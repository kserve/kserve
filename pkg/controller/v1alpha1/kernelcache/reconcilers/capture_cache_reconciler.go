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
	"errors"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	kernelcacheconfig "github.com/kserve/kserve/pkg/kernelcache/config"
	"github.com/kserve/kserve/pkg/kernelcache/nodegroup"
	kernelcachetypes "github.com/kserve/kserve/pkg/kernelcache/types"
)

type nodeGroupSelection struct {
	Name   string
	Source string
}

type nodeGroupSelectionError struct {
	Reason  string
	Message string
}

func (e *nodeGroupSelectionError) Error() string {
	return e.Message
}

func (r *KernelCacheReconciler) reconcileCaptureKernelCache(ctx context.Context, req ctrl.Request) error {
	config, err := kernelcacheconfig.Load(ctx, r.Client)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return nil
	}

	capture, err := r.findCompletedCapture(ctx, req.NamespacedName)
	if err != nil || capture == nil {
		return err
	}

	mountType, err := captureMountType(config)
	if err != nil {
		return err
	}
	selection, err := r.captureNodeGroup(ctx, capture, config)
	if err != nil {
		var selectionErr *nodeGroupSelectionError
		if !errors.As(err, &selectionErr) {
			return err
		}
		r.Log.Info("Skipping KernelCache creation", "capture", client.ObjectKeyFromObject(capture), "reason", selectionErr.Reason, "message", selectionErr.Message)
		if r.Recorder != nil {
			r.Recorder.Eventf(capture, nil, corev1.EventTypeWarning, selectionErr.Reason, "SelectNodeGroup", selectionErr.Message)
		}
		return nil
	}

	kernelCache := &v1alpha1.KernelCache{}
	if err := r.Get(ctx, req.NamespacedName, kernelCache); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		kernelCache = &v1alpha1.KernelCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
				Annotations: map[string]string{
					constants.KernelCacheNodeGroupSelectionSourceAnnotationKey: selection.Source,
				},
			},
			Spec: v1alpha1.KernelCacheSpec{
				Artifact:     *capture.Status.Artifact.DeepCopy(),
				MountType:    mountType,
				NodeGroupRef: &corev1.LocalObjectReference{Name: selection.Name},
			},
		}
		if err := r.Create(ctx, kernelCache); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
			reader := r.Reader
			if reader == nil {
				reader = r.Client
			}
			if err := reader.Get(ctx, req.NamespacedName, kernelCache); err != nil {
				return err
			}
		}
	}

	if err := r.setCaptureKernelCacheRef(ctx, capture, kernelCache); err != nil {
		return err
	}
	return nil
}

func (r *KernelCacheReconciler) findCompletedCapture(ctx context.Context, key types.NamespacedName) (*v1alpha1.KernelCacheCapture, error) {
	captures := &v1alpha1.KernelCacheCaptureList{}
	if err := r.List(ctx, captures, client.InNamespace(key.Namespace)); err != nil {
		return nil, err
	}

	for index := range captures.Items {
		capture := &captures.Items[index]
		if !isCaptureComplete(capture) {
			continue
		}
		if generatedKernelCacheName(capture.Name, capture.Status.Artifact.ImageReference) == key.Name {
			return capture, nil
		}
	}
	return nil, nil
}

func isCaptureComplete(capture *v1alpha1.KernelCacheCapture) bool {
	if condition := meta.FindStatusCondition(capture.Status.Conditions, kernelCacheCaptureReadyConditionType); condition == nil ||
		condition.Status != metav1.ConditionTrue || condition.ObservedGeneration != capture.Generation {
		return false
	}
	if active := capture.Status.ActiveSession; active != nil &&
		(capture.Status.RuntimeResult[runtimeResultCaptureSessionIDKey] != active.ID || capture.Status.RuntimeResult[runtimeResultSourcePodNameKey] != active.PodName) {
		return false
	}
	if capture.Status.Phase != v1alpha1.KernelCacheCapturePhaseComplete || capture.Status.Artifact == nil {
		return false
	}
	if !captureSigningAllowsCache(capture.Status.Signing) {
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

func captureSigningAllowsCache(signing *v1alpha1.KernelCacheSigningStatus) bool {
	if signing == nil {
		return false
	}
	if signing.State == v1alpha1.KernelCacheArtifactSecurityStateSkipped {
		return signing.Mode == "none" || signing.Mode == string(kernelcachetypes.ModeDisabled)
	}
	return signing.State == v1alpha1.KernelCacheArtifactSecurityStateSucceeded && signing.Signed
}

func generatedKernelCacheName(captureName, imageReference string) string {
	prefix := strings.TrimSuffix(captureName, "-capture")
	if len(prefix) > 38 {
		prefix = prefix[:38]
	}
	sum := sha256.Sum256([]byte(imageReference))
	return fmt.Sprintf("%s-%x", strings.TrimRight(prefix, "-."), sum[:12])
}

func (r *KernelCacheReconciler) captureNodeGroup(
	ctx context.Context,
	capture *v1alpha1.KernelCacheCapture,
	config *v1beta1.KernelCacheConfig,
) (nodeGroupSelection, error) {
	activeSession := capture.Status.ActiveSession
	if activeSession != nil && strings.TrimSpace(activeSession.RequestedNodeGroup) != "" {
		name := strings.TrimSpace(activeSession.RequestedNodeGroup)
		if err := r.validateNodeGroup(ctx, name); err != nil {
			return nodeGroupSelection{}, &nodeGroupSelectionError{Reason: "InvalidRequestedNodeGroup", Message: err.Error()}
		}
		return nodeGroupSelection{Name: name, Source: "annotation"}, nil
	}
	if activeSession != nil && activeSession.NodeName == "" {
		return nodeGroupSelection{}, &nodeGroupSelectionError{
			Reason:  "ProducerNodePending",
			Message: "producer Pod node assignment has not been observed yet",
		}
	}

	if activeSession != nil && activeSession.NodeName != "" {
		node := &corev1.Node{}
		if err := r.Get(ctx, client.ObjectKey{Name: activeSession.NodeName}, node); err != nil {
			if !apierrors.IsNotFound(err) {
				return nodeGroupSelection{}, err
			}
		} else {
			groups := &v1alpha1.KernelCacheNodeGroupList{}
			if err := r.List(ctx, groups); err != nil {
				return nodeGroupSelection{}, err
			}
			matches := nodegroup.MatchingGroups(node, groups.Items)
			switch len(matches) {
			case 1:
				return nodeGroupSelection{Name: matches[0].Name, Source: "auto"}, nil
			case 0:
			default:
				names := make([]string, 0, len(matches))
				for index := range matches {
					names = append(names, matches[index].Name)
				}
				return nodeGroupSelection{}, &nodeGroupSelectionError{
					Reason:  "AmbiguousNodeGroup",
					Message: fmt.Sprintf("producer node %s matches multiple node groups: %s; set %s", node.Name, strings.Join(names, ", "), constants.KernelCacheNodeGroupAnnotationKey),
				}
			}
		}
	}

	name := strings.TrimSpace(config.DefaultNodeGroup)
	if name == "" {
		return nodeGroupSelection{}, &nodeGroupSelectionError{Reason: "NoMatchingNodeGroup", Message: "no node group matched the producer node and no defaultNodeGroup is configured"}
	}
	if err := r.validateNodeGroup(ctx, name); err != nil {
		return nodeGroupSelection{}, &nodeGroupSelectionError{Reason: "InvalidDefaultNodeGroup", Message: err.Error()}
	}
	return nodeGroupSelection{Name: name, Source: "default"}, nil
}

func (r *KernelCacheReconciler) validateNodeGroup(ctx context.Context, name string) error {
	group := &v1alpha1.KernelCacheNodeGroup{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, group); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("KernelCacheNodeGroup %q was not found", name)
		}
		return err
	}
	return nil
}

func captureMountType(config *v1beta1.KernelCacheConfig) (v1alpha1.KernelCacheMountType, error) {
	switch config.DefaultMountType {
	case "":
		return v1alpha1.KernelCacheMountType(v1beta1.DefaultKernelCacheMountType), nil
	case string(v1alpha1.KernelCacheMountTypeOCI):
		return v1alpha1.KernelCacheMountTypeOCI, nil
	default:
		return "", fmt.Errorf("invalid kernelcache.defaultMountType %q", config.DefaultMountType)
	}
}

func (r *KernelCacheReconciler) reconcileCaptureKernelCacheRef(ctx context.Context, kernelCache *v1alpha1.KernelCache) error {
	capture, err := r.findCompletedCapture(ctx, client.ObjectKeyFromObject(kernelCache))
	if err != nil || capture == nil {
		return err
	}
	return r.setCaptureKernelCacheRef(ctx, capture, kernelCache)
}

func (r *KernelCacheReconciler) setCaptureKernelCacheRef(ctx context.Context, capture *v1alpha1.KernelCacheCapture, kernelCache *v1alpha1.KernelCache) error {
	if capture.Status.Artifact == nil || !reflect.DeepEqual(*capture.Status.Artifact, kernelCache.Spec.Artifact) {
		return fmt.Errorf("KernelCache %s/%s artifact does not match KernelCacheCapture %s/%s", kernelCache.Namespace, kernelCache.Name, capture.Namespace, capture.Name)
	}
	if capture.Status.KernelCacheRef != nil &&
		capture.Status.KernelCacheRef.Namespace == kernelCache.Namespace &&
		capture.Status.KernelCacheRef.Name == kernelCache.Name {
		return nil
	}

	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &v1alpha1.KernelCacheCapture{}
		if err := reader.Get(ctx, client.ObjectKeyFromObject(capture), current); err != nil {
			return err
		}
		if !isCaptureComplete(current) || !reflect.DeepEqual(current.Status.Artifact, capture.Status.Artifact) ||
			current.Status.RuntimeResult[runtimeResultCaptureSessionIDKey] != capture.Status.RuntimeResult[runtimeResultCaptureSessionIDKey] {
			return nil
		}
		if current.Status.KernelCacheRef != nil {
			if current.Status.KernelCacheRef.Namespace == kernelCache.Namespace && current.Status.KernelCacheRef.Name == kernelCache.Name {
				return nil
			}
		}

		current.Status.KernelCacheRef = &v1alpha1.NamespacedName{
			Namespace: kernelCache.Namespace,
			Name:      kernelCache.Name,
		}
		return r.Status().Update(ctx, current)
	})
}

// enqueueKCForCompletedKCC maps a completed KCC to the deterministic KC that represents its artifact.
// This lets the KC reconciler create or link the KC after capture finishes; incomplete KCCs are ignored.
// The handler enqueues the returned request; this function does not run KC reconciliation.
func (r *KernelCacheReconciler) enqueueKCForCompletedKCC(_ context.Context, obj client.Object) []reconcile.Request {
	capture, ok := obj.(*v1alpha1.KernelCacheCapture)
	if !ok || !isCaptureComplete(capture) {
		return nil
	}
	name := generatedKernelCacheName(capture.Name, capture.Status.Artifact.ImageReference)
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: capture.Namespace, Name: name}}}
}

// enqueueKCsOnInferenceServiceChange finds completed KCCs whose source is the changed ISVC.
// Requeuing their KCs gives completed captures another chance to materialize or refresh their KC.
// The handler enqueues the returned requests; this function does not run KC reconciliation.
func (r *KernelCacheReconciler) enqueueKCsOnInferenceServiceChange(ctx context.Context, obj client.Object) []reconcile.Request {
	inferenceService, ok := obj.(*v1beta1.InferenceService)
	if !ok {
		return nil
	}

	captures := &v1alpha1.KernelCacheCaptureList{}
	if err := r.List(ctx, captures, client.InNamespace(inferenceService.Namespace)); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(captures.Items))
	for index := range captures.Items {
		capture := &captures.Items[index]
		if !isCaptureComplete(capture) || !captureReferencesInferenceService(capture, inferenceService) {
			continue
		}
		name := generatedKernelCacheName(capture.Name, capture.Status.Artifact.ImageReference)
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: capture.Namespace, Name: name}})
	}
	return requests
}
