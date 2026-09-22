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

package kernelcachenode

import (
	"context"
	"errors"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// Preparation status is derived from Jobs, Pods, and node image inventory.
func (r *KernelCacheNodeReconciler) updateCacheStatuses(
	ctx context.Context,
	kernelCacheNode *v1alpha1.KernelCacheNode,
	config *v1beta1.KernelCacheConfig,
	validateImages bool,
) error {
	var node *corev1.Node
	var nodeLoaded bool
	for cacheKey, cacheInfo := range kernelCacheNode.Status.CacheStatus {
		kernelCache := &v1alpha1.KernelCache{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: cacheInfo.KernelCacheRef.Namespace,
			Name:      cacheInfo.KernelCacheRef.Name,
		}, kernelCache); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("get KernelCache %s/%s: %w", cacheInfo.KernelCacheRef.Namespace, cacheInfo.KernelCacheRef.Name, err)
		}

		job, err := r.getPreparationJob(ctx, kernelCache, config.JobNamespace)
		if err != nil {
			return fmt.Errorf("get preparation Job for KernelCache %s/%s: %w", kernelCache.Namespace, kernelCache.Name, err)
		}
		podFailureMessage, err := r.getPreparationPodFailure(ctx, job)
		if err != nil {
			return fmt.Errorf("inspect preparation Pods for KernelCache %s/%s: %w", kernelCache.Namespace, kernelCache.Name, err)
		}
		if validateImages && (job == nil || cacheInfo.State == v1alpha1.KernelCacheNodePreparationStateReady) && !nodeLoaded {
			node = &corev1.Node{}
			if err := r.Get(ctx, client.ObjectKey{Name: r.NodeName}, node); err != nil {
				return fmt.Errorf("get Node %q for image validation: %w", r.NodeName, err)
			}
			nodeLoaded = true
		}
		state, message := cachePreparationState(job, cacheInfo, podFailureMessage)
		if validateImages && (job == nil || cacheInfo.State == v1alpha1.KernelCacheNodePreparationStateReady) {
			state, message = cacheImageValidationState(cacheInfo, node)
		}
		if state != cacheInfo.State || message != cacheInfo.Message {
			cacheInfo.State = state
			cacheInfo.Message = message
			cacheInfo.LastUpdate = metav1.Now()
		}
		kernelCacheNode.Status.CacheStatus[cacheKey] = cacheInfo
	}
	return nil
}

func (r *KernelCacheNodeReconciler) getPreparationJob(
	ctx context.Context,
	kernelCache *v1alpha1.KernelCache,
	jobNamespace string,
) (*batchv1.Job, error) {
	if jobNamespace == "" {
		return nil, errors.New("kernelcache.jobNamespace is required")
	}

	jobs := &batchv1.JobList{}
	if err := r.List(ctx, jobs,
		client.InNamespace(jobNamespace),
		client.MatchingLabels{
			kernelCacheNameLabel:      kernelCache.Name,
			kernelCacheNamespaceLabel: kernelCache.Namespace,
		},
	); err != nil {
		return nil, fmt.Errorf("list preparation Jobs in namespace %q for KernelCache %s/%s: %w", jobNamespace, kernelCache.Namespace, kernelCache.Name, err)
	}

	var latest *batchv1.Job
	for i := range jobs.Items {
		job := &jobs.Items[i]
		// Ignore prefetch results for a previous artifact of the same cache.
		staleArtifact := false
		for _, volume := range job.Spec.Template.Spec.Volumes {
			if volume.Image != nil && volume.Image.Reference != kernelCache.Spec.Artifact.ImageReference {
				staleArtifact = true
				break
			}
		}
		if staleArtifact {
			continue
		}
		if nodeName, ok := job.Labels[kernelCacheNodeLabel]; !ok || nodeName != r.NodeName {
			continue
		}
		if latest == nil || job.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = job
		}
	}
	return latest, nil
}

func (r *KernelCacheNodeReconciler) getPreparationPodFailure(ctx context.Context, job *batchv1.Job) (string, error) {
	if job == nil || jobCompleted(job) || failedJobMessage(job) != "" {
		return "", nil
	}

	pods := &corev1.PodList{}
	// Prefetch Pods are not in the ISVC-filtered manager cache.
	reader := client.Reader(r.Client)
	if r.Reader != nil {
		reader = r.Reader
	}
	if err := reader.List(ctx, pods,
		client.InNamespace(job.Namespace),
		client.MatchingLabels{batchv1.JobNameLabel: job.Name},
	); err != nil {
		return "", fmt.Errorf("list Pods for preparation Job %s/%s: %w", job.Namespace, job.Name, err)
	}

	for i := range pods.Items {
		if message := failedPodMessage(&pods.Items[i]); message != "" {
			return message, nil
		}
	}
	return "", nil
}

func cachePreparationState(
	job *batchv1.Job,
	cacheInfo v1alpha1.KernelCacheNodeCacheInfo,
	podFailureMessage string,
) (v1alpha1.KernelCacheNodePreparationState, string) {
	return preparationState(job, cacheInfo.State, podFailureMessage)
}

func cacheImageValidationState(
	cacheInfo v1alpha1.KernelCacheNodeCacheInfo,
	node *corev1.Node,
) (v1alpha1.KernelCacheNodePreparationState, string) {
	if nodeContainsImage(node, cacheInfo.ImageReference) {
		return v1alpha1.KernelCacheNodePreparationStateReady, "OCI artifact is present on the node"
	}
	return v1alpha1.KernelCacheNodePreparationStatePending, missingImageMessage
}

func preparationState(
	job *batchv1.Job,
	current v1alpha1.KernelCacheNodePreparationState,
	podFailureMessage string,
) (v1alpha1.KernelCacheNodePreparationState, string) {
	if job != nil {
		if message := failedJobMessage(job); message != "" {
			return v1alpha1.KernelCacheNodePreparationStateError, message
		}
		if jobCompleted(job) {
			return v1alpha1.KernelCacheNodePreparationStateReady, "OCI artifact was prefetched on the node"
		}
		if podFailureMessage != "" {
			return v1alpha1.KernelCacheNodePreparationStateError, podFailureMessage
		}
		if job.Status.StartTime == nil && job.Status.Active == 0 {
			return v1alpha1.KernelCacheNodePreparationStatePulling, "waiting for the OCI prefetch Job to start"
		}
		return v1alpha1.KernelCacheNodePreparationStatePulling, "OCI artifact is being prefetched"
	}
	if current == v1alpha1.KernelCacheNodePreparationStateReady {
		return current, "OCI artifact was prefetched on the node"
	}
	return v1alpha1.KernelCacheNodePreparationStatePending, "waiting for the OCI prefetch Job"
}

func nodeContainsImage(node *corev1.Node, imageReference string) bool {
	if node == nil || imageReference == "" {
		return false
	}

	for _, image := range node.Status.Images {
		for _, name := range image.Names {
			if name == imageReference {
				return true
			}
		}
	}
	return false
}

func jobCompleted(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func failedJobMessage(job *batchv1.Job) string {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			if condition.Message != "" {
				return condition.Message
			}
			if condition.Reason != "" {
				return condition.Reason
			}
			return "preparation Job failed"
		}
	}
	return ""
}

func failedPodMessage(pod *corev1.Pod) string {
	statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
	for _, status := range statuses {
		if waiting := status.State.Waiting; waiting != nil && isPreparationFailureReason(waiting.Reason) {
			if waiting.Message != "" {
				return waiting.Message
			}
			if waiting.Reason != "" {
				return waiting.Reason
			}
		}
		if terminated := status.State.Terminated; terminated != nil && terminated.ExitCode != 0 {
			if terminated.Message != "" {
				return terminated.Message
			}
			if terminated.Reason != "" {
				return terminated.Reason
			}
			return fmt.Sprintf("preparation container exited with code %d", terminated.ExitCode)
		}
	}

	if pod.Status.Message != "" {
		return pod.Status.Message
	}
	if pod.Status.Reason != "" {
		return pod.Status.Reason
	}
	if pod.Status.Phase == corev1.PodFailed {
		return "preparation Pod failed"
	}

	return ""
}

func isPreparationFailureReason(reason string) bool {
	switch reason {
	case "ErrImagePull", "ImagePullBackOff", "ErrImageNeverPull", "InvalidImageName", "CreateContainerConfigError", "CreateContainerError", "RunContainerError", "ContainerCannotRun", "CrashLoopBackOff":
		return true
	default:
		return false
	}
}
