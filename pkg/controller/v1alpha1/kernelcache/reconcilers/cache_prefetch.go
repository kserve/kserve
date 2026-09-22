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
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

const (
	kernelCachePrefetchServiceAccount = "kernel-cache-prefetcher"
	kernelCachePrefetchManagedLabel   = "internal.serving.kserve.io/kernelcache-prefetcher"
)

func (r *KernelCacheReconciler) checkNamespace(ctx context.Context, name string) error {
	namespace := &corev1.Namespace{}
	reader := r.Reader
	if reader == nil {
		reader = r.Client
	}
	if err := reader.Get(ctx, client.ObjectKey{Name: name}, namespace); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("job namespace %q was not found; create it before using KernelCache", name)
		}
		return err
	}
	return nil
}

func (r *KernelCacheReconciler) ensurePrefetchServiceAccount(ctx context.Context, namespace string) error {
	automount := false
	desired := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:      kernelCachePrefetchServiceAccount,
		Namespace: namespace,
		Labels:    map[string]string{kernelCachePrefetchManagedLabel: "true"},
	}, AutomountServiceAccountToken: &automount}

	current := &corev1.ServiceAccount{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if current.Labels[kernelCachePrefetchManagedLabel] != "true" {
		return fmt.Errorf("reserved ServiceAccount %s/%s already exists and is not managed by KernelCache", namespace, kernelCachePrefetchServiceAccount)
	}
	if current.AutomountServiceAccountToken == nil || *current.AutomountServiceAccountToken {
		return fmt.Errorf("managed ServiceAccount %s/%s must disable token automount", namespace, kernelCachePrefetchServiceAccount)
	}
	return nil
}

func (r *KernelCacheReconciler) ensureOCIPrefetchJobs(
	ctx context.Context,
	kernelCache *v1alpha1.KernelCache,
	nodeGroup *v1alpha1.KernelCacheNodeGroup,
	readyNodes *corev1.NodeList,
	config *v1beta1.KernelCacheConfig,
) error {
	for i := range readyNodes.Items {
		if err := r.ensureOCIPrefetchJob(ctx, kernelCache, nodeGroup, &readyNodes.Items[i], config); err != nil {
			return err
		}
	}
	return nil
}

func (r *KernelCacheReconciler) ensureOCIPrefetchJob(
	ctx context.Context,
	kernelCache *v1alpha1.KernelCache,
	nodeGroup *v1alpha1.KernelCacheNodeGroup,
	node *corev1.Node,
	config *v1beta1.KernelCacheConfig,
) error {
	kernelCacheNode := &v1alpha1.KernelCacheNode{}
	if err := r.Get(ctx, client.ObjectKey{Name: node.Name}, kernelCacheNode); err == nil {
		cacheInfo, exists := kernelCacheNode.Status.CacheStatus[kernelCache.Namespace+"/"+kernelCache.Name]
		if !exists {
			return nil
		}
		if cacheInfo.State == v1alpha1.KernelCacheNodePreparationStateReady &&
			cacheInfo.ImageReference == kernelCache.Spec.Artifact.ImageReference &&
			cacheInfo.Footprints == kernelCache.Spec.Artifact.Identity.Footprints {
			return nil
		}
	} else if apierrors.IsNotFound(err) {
		return nil
	} else {
		return err
	}

	jobName := kernelCachePrefetchJobName(kernelCache, node)
	job := &batchv1.Job{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName, Namespace: config.JobNamespace}, job); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	labels := kernelCacheLabels(kernelCache, node.Name)
	job = &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: config.JobNamespace, Labels: labels},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: kernelCacheJobTTL(config),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: kernelCachePrefetchServiceAccount,
					NodeSelector:       map[string]string{"kubernetes.io/hostname": nodeHostname(node)},
					Tolerations:        append([]corev1.Toleration(nil), nodeGroup.Spec.Tolerations...),
					Containers: []corev1.Container{{
						Name:            "kernel-cache-prefetch",
						Image:           config.PrefetchImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Command:         []string{"/bin/sh", "-c", "test -d /mnt/kernel-cache"},
						VolumeMounts:    []corev1.VolumeMount{{Name: "kernel-cache", MountPath: "/mnt/kernel-cache", ReadOnly: true}},
					}},
					Volumes: []corev1.Volume{{Name: "kernel-cache", VolumeSource: corev1.VolumeSource{
						Image: &corev1.ImageVolumeSource{Reference: kernelCache.Spec.Artifact.ImageReference, PullPolicy: corev1.PullIfNotPresent},
					}}},
				},
			},
		},
	}
	automount := false
	job.Spec.Template.Spec.AutomountServiceAccountToken = &automount
	if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func kernelCacheLabels(kernelCache *v1alpha1.KernelCache, nodeName string) map[string]string {
	return map[string]string{
		kernelCacheNameLabel:      kernelCache.Name,
		kernelCacheNamespaceLabel: kernelCache.Namespace,
		kernelCacheNodeLabel:      nodeName,
	}
}

func kernelCachePrefetchJobName(kernelCache *v1alpha1.KernelCache, node *corev1.Node) string {
	key := kernelCache.Namespace + "/" + kernelCache.Name + "/" + kernelCache.Spec.Artifact.ImageReference + "/" + node.Name
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("kc-%s-%s-%s", readableJobNamePart(kernelCache.Name, 24), readableJobNamePart(node.Name, 18), hex.EncodeToString(hash[:])[:16])
}

func readableJobNamePart(value string, maxLength int) string {
	value = strings.ReplaceAll(strings.ToLower(value), ".", "-")
	if len(value) > maxLength {
		value = value[:maxLength]
	}
	value = strings.Trim(value, "-.")
	if value == "" {
		return "unknown"
	}
	return value
}

func kernelCacheJobTTL(config *v1beta1.KernelCacheConfig) *int32 {
	if config.JobTTLSecondsAfterFinished != nil {
		return config.JobTTLSecondsAfterFinished
	}
	ttl := v1beta1.DefaultKernelCacheJobTTLSeconds
	return &ttl
}

func nodeHostname(node *corev1.Node) string {
	if hostname := node.Labels["kubernetes.io/hostname"]; hostname != "" {
		return hostname
	}
	return node.Name
}
