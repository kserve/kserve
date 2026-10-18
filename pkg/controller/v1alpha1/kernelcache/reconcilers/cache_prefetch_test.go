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
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	cacheidentity "github.com/kserve/kserve/pkg/kernelcache/identity"
	kernelcachelabels "github.com/kserve/kserve/pkg/kernelcache/labels"
)

func TestEnsureRuntimeImagePrefetchJobCreatesExpectedJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	runtimeImage := "registry.example.com/vllm@sha256:" + strings.Repeat("a", 64)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "worker-a",
		UID:  types.UID("node-uid-a"),
		Labels: map[string]string{
			"kubernetes.io/hostname": "worker-a.internal",
		},
	}}
	nodeGroup := &v1alpha1.KernelCacheNodeGroup{Spec: v1alpha1.KernelCacheNodeGroupSpec{
		Tolerations: []corev1.Toleration{{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists}},
	}}
	configuredTTL := int32(600)
	runtimeImagePrefetchTTL := int32(7200)
	config := &v1beta1.KernelCacheConfig{
		JobNamespace:               "jobs",
		PrefetchImage:              "example.com/prefetch:latest",
		JobTTLSecondsAfterFinished: &configuredTTL,
		RuntimeImagePrefetch: v1beta1.KernelCacheRuntimeImagePrefetchConfig{
			Enabled:                    true,
			JobTTLSecondsAfterFinished: &runtimeImagePrefetchTTL,
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	if err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nodeGroup.Spec.Tolerations, node, config); err != nil {
		t.Fatal(err)
	}

	jobs := &batchv1.JobList{}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected one runtime image prefetch Job, got %d", len(jobs.Items))
	}
	job := &jobs.Items[0]
	if job.Labels[runtimeImagePrefetchJobLabel] != "true" {
		t.Fatalf("runtime image prefetch label missing: %#v", job.Labels)
	}
	if job.Annotations[runtimeImagePrefetchImageAnnotation] != runtimeImage {
		t.Fatalf("unexpected runtime image annotation: %q", job.Annotations[runtimeImagePrefetchImageAnnotation])
	}
	if job.Annotations[kernelcachelabels.KernelCacheNodeAnnotation] != node.Name {
		t.Fatalf("unexpected node annotation: %q", job.Annotations[kernelcachelabels.KernelCacheNodeAnnotation])
	}
	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != runtimeImagePrefetchTTL {
		t.Fatalf("unexpected Job TTL: %#v", job.Spec.TTLSecondsAfterFinished)
	}
	podSpec := job.Spec.Template.Spec
	if podSpec.ServiceAccountName != kernelCachePrefetchServiceAccount {
		t.Fatalf("unexpected ServiceAccount: %q", podSpec.ServiceAccountName)
	}
	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Fatal("runtime image prefetch Job must disable ServiceAccount token automount")
	}
	if podSpec.NodeSelector["kubernetes.io/hostname"] != "worker-a.internal" {
		t.Fatalf("unexpected node selector: %#v", podSpec.NodeSelector)
	}
	if len(podSpec.Tolerations) != 1 || podSpec.Tolerations[0].Key != "nvidia.com/gpu" {
		t.Fatalf("unexpected tolerations: %#v", podSpec.Tolerations)
	}
	if len(podSpec.Volumes) != 1 || podSpec.Volumes[0].Image == nil {
		t.Fatalf("expected one image volume: %#v", podSpec.Volumes)
	}
	if podSpec.Volumes[0].Image.Reference != runtimeImage || podSpec.Volumes[0].Image.PullPolicy != corev1.PullIfNotPresent {
		t.Fatalf("unexpected runtime image volume: %#v", podSpec.Volumes[0].Image)
	}
	if len(podSpec.Containers) != 1 || podSpec.Containers[0].Image != config.PrefetchImage {
		t.Fatalf("unexpected prefetch container: %#v", podSpec.Containers)
	}
}

func TestEnsureRuntimeImagePrefetchJobRecreatesFailedJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	runtimeImage := "example.com/vllm:v1"
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: types.UID("node-uid")}}
	config := &v1beta1.KernelCacheConfig{JobNamespace: "jobs", PrefetchImage: "example.com/prefetch:latest"}
	failedJob := buildImagePrefetchJob(imagePrefetchJobOptions{
		name: runtimeImagePrefetchJobName(runtimeImage, node), namespace: config.JobNamespace,
		containerName: "runtime-image-prefetch", helperImage: config.PrefetchImage,
		imageReference: runtimeImage, volumeName: runtimeImagePrefetchVolumeName,
		mountPath: runtimeImagePrefetchMountPath, node: node,
	})
	failedJob.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(failedJob).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	if err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nil, node, config); err == nil {
		t.Fatal("expected failed Job deletion to requeue")
	}
	job := &batchv1.Job{}
	if err := k8sClient.Get(t.Context(), client.ObjectKeyFromObject(failedJob), job); err == nil {
		t.Fatal("expected failed Job to be deleted")
	}
	if err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nil, node, config); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Get(t.Context(), client.ObjectKeyFromObject(failedJob), job); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRuntimeImagePrefetchJobWaitsForFailedJobDeletion(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	runtimeImage := "example.com/vllm:v1"
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: types.UID("node-uid")}}
	config := &v1beta1.KernelCacheConfig{JobNamespace: "jobs", PrefetchImage: "example.com/prefetch:latest"}
	deletionTime := metav1.Now()
	failedJob := buildImagePrefetchJob(imagePrefetchJobOptions{
		name: runtimeImagePrefetchJobName(runtimeImage, node), namespace: config.JobNamespace,
		containerName: "runtime-image-prefetch", helperImage: config.PrefetchImage,
		imageReference: runtimeImage, volumeName: runtimeImagePrefetchVolumeName,
		mountPath: runtimeImagePrefetchMountPath, node: node,
	})
	failedJob.DeletionTimestamp = &deletionTime
	failedJob.Finalizers = []string{"test.kserve.io/prefetch-job"}
	failedJob.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(failedJob).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	if err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nil, node, config); err == nil {
		t.Fatal("expected failed Job deletion to requeue")
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 1 || jobs.Items[0].DeletionTimestamp.IsZero() {
		t.Fatalf("expected failed Job deletion to remain pending, got %#v", jobs.Items)
	}
}

func TestEnsureRuntimeImagePrefetchJobsSkipsWhenDisabledOrImageIsIneligible(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		image   string
	}{
		{name: "disabled", enabled: false, image: "example.com/vllm:v1"},
		{name: "missing runtime image", enabled: true},
		{name: "latest runtime image", enabled: true, image: "example.com/vllm:latest"},
		{name: "runtime image with implicit latest tag", enabled: true, image: "example.com/vllm"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := batchv1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			if err := corev1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &KernelCacheReconciler{Client: k8sClient}
			kernelCache := kernelCacheWithRuntimeImage("cache", tc.image)
			nodes := &corev1.NodeList{Items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: types.UID("uid")}}}}
			config := &v1beta1.KernelCacheConfig{
				JobNamespace: "jobs", PrefetchImage: "example.com/prefetch:latest",
				RuntimeImagePrefetch: v1beta1.KernelCacheRuntimeImagePrefetchConfig{Enabled: tc.enabled},
			}

			if err := reconciler.ensureRuntimeImagePrefetchJobs(t.Context(), kernelCache, &v1alpha1.KernelCacheNodeGroup{}, nodes, config); err != nil {
				t.Fatal(err)
			}
			jobs := &batchv1.JobList{}
			if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
				t.Fatal(err)
			}
			if len(jobs.Items) != 0 {
				t.Fatalf("expected no runtime image prefetch Jobs, got %d", len(jobs.Items))
			}
		})
	}
}

func TestRuntimeImagePrefetchJobsAreSharedByImageAndNode(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}
	config := &v1beta1.KernelCacheConfig{
		JobNamespace: "jobs", PrefetchImage: "example.com/prefetch:latest",
		RuntimeImagePrefetch: v1beta1.KernelCacheRuntimeImagePrefetchConfig{Enabled: true},
	}
	image := "example.com/vllm:v1"
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: types.UID("uid-a")}}
	nodes := &corev1.NodeList{Items: []corev1.Node{*node}}
	nodeGroup := &v1alpha1.KernelCacheNodeGroup{}

	for _, name := range []string{"cache-a", "cache-b"} {
		if err := reconciler.ensureRuntimeImagePrefetchJobs(t.Context(), kernelCacheWithRuntimeImage(name, image), nodeGroup, nodes, config); err != nil {
			t.Fatal(err)
		}
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected KCs to share one runtime image prefetch Job, got %d", len(jobs.Items))
	}

	replacement := node.DeepCopy()
	replacement.UID = types.UID("uid-b")
	if err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), image, nil, replacement, config); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 2 {
		t.Fatalf("expected a new Job for the replacement node UID, got %d", len(jobs.Items))
	}

	if err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), "example.com/vllm:v2", nil, node, config); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 3 {
		t.Fatalf("expected a new Job for a different runtime image, got %d", len(jobs.Items))
	}
}

func kernelCacheWithRuntimeImage(name, runtimeImage string) *v1alpha1.KernelCache {
	factors := map[string]string{}
	if runtimeImage != "" {
		factors[cacheidentity.RuntimeImageFactor] = runtimeImage
	}
	return &v1alpha1.KernelCache{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team"},
		Spec: v1alpha1.KernelCacheSpec{Artifact: v1alpha1.KernelCacheArtifact{
			Identity: v1alpha1.KernelCacheIdentity{Factors: factors},
		}},
	}
}
