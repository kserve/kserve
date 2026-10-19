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
	"errors"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

	needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nodeGroup.Spec.Tolerations, node, config)
	if err != nil {
		t.Fatal(err)
	}
	if needsRequeue {
		t.Fatal("did not expect a requeue after creating a runtime image prefetch Job")
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
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != runtimeImagePrefetchActiveDeadlineSeconds {
		t.Fatalf("unexpected runtime image prefetch deadline: %#v", job.Spec.ActiveDeadlineSeconds)
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

func TestEnsureRuntimeImagePrefetchJobDeletesFailedJobBeforeRecreating(t *testing.T) {
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
		name: runtimeImagePrefetchJobName(runtimeImage, node, nil), namespace: config.JobNamespace,
		containerName: "runtime-image-prefetch", helperImage: config.PrefetchImage,
		imageReference: runtimeImage, volumeName: runtimeImagePrefetchVolumeName,
		mountPath: runtimeImagePrefetchMountPath, node: node,
	})
	failedJob.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
	deleteOptions := client.DeleteOptions{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(failedJob).WithInterceptorFuncs(interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, object client.Object, options ...client.DeleteOption) error {
			for _, option := range options {
				option.ApplyToDelete(&deleteOptions)
			}
			return c.Delete(ctx, object, options...)
		},
	}).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nil, node, config)
	if err != nil {
		t.Fatal(err)
	}
	if !needsRequeue {
		t.Fatal("expected failed Job deletion to request a requeue")
	}
	if deleteOptions.PropagationPolicy == nil || *deleteOptions.PropagationPolicy != metav1.DeletePropagationBackground {
		t.Fatalf("expected background deletion, got %#v", deleteOptions.PropagationPolicy)
	}
	job := &batchv1.Job{}
	if err := k8sClient.Get(t.Context(), client.ObjectKeyFromObject(failedJob), job); err == nil {
		t.Fatal("expected failed Job to be deleted")
	}
	needsRequeue, err = reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nil, node, config)
	if err != nil {
		t.Fatal(err)
	}
	if needsRequeue {
		t.Fatal("did not expect a requeue after the failed Job was deleted")
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
		name: runtimeImagePrefetchJobName(runtimeImage, node, nil), namespace: config.JobNamespace,
		containerName: "runtime-image-prefetch", helperImage: config.PrefetchImage,
		imageReference: runtimeImage, volumeName: runtimeImagePrefetchVolumeName,
		mountPath: runtimeImagePrefetchMountPath, node: node,
	})
	failedJob.DeletionTimestamp = &deletionTime
	failedJob.Finalizers = []string{"test.kserve.io/prefetch-job"}
	failedJob.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(failedJob).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nil, node, config)
	if err != nil {
		t.Fatal(err)
	}
	if !needsRequeue {
		t.Fatal("expected a requeue while the failed Job is deleting")
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 1 || jobs.Items[0].DeletionTimestamp.IsZero() {
		t.Fatalf("expected failed Job deletion to remain pending, got %#v", jobs.Items)
	}
}

func TestEnsureRuntimeImagePrefetchJobsContinuesWhileOneJobIsDeleting(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	runtimeImage := "example.com/vllm:v1"
	deletingNode := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", UID: types.UID("node-uid-a")}}
	readyNode := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-b", UID: types.UID("node-uid-b")}}
	config := &v1beta1.KernelCacheConfig{
		JobNamespace: "jobs", PrefetchImage: "example.com/prefetch:latest",
		RuntimeImagePrefetch: v1beta1.KernelCacheRuntimeImagePrefetchConfig{Enabled: true},
	}
	deletionTime := metav1.Now()
	deletingJob := buildImagePrefetchJob(imagePrefetchJobOptions{
		name: runtimeImagePrefetchJobName(runtimeImage, &deletingNode, nil), namespace: config.JobNamespace,
		containerName: "runtime-image-prefetch", helperImage: config.PrefetchImage,
		imageReference: runtimeImage, volumeName: runtimeImagePrefetchVolumeName,
		mountPath: runtimeImagePrefetchMountPath, node: &deletingNode,
	})
	deletingJob.DeletionTimestamp = &deletionTime
	deletingJob.Finalizers = []string{"test.kserve.io/prefetch-job"}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deletingJob).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}
	nodes := &corev1.NodeList{Items: []corev1.Node{deletingNode, readyNode}}

	needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJobs(t.Context(), kernelCacheWithRuntimeImage("cache", runtimeImage), &v1alpha1.KernelCacheNodeGroup{}, nodes, config)
	if err != nil {
		t.Fatal(err)
	}
	if !needsRequeue {
		t.Fatal("expected a requeue while one runtime image prefetch Job is deleting")
	}
	createdJob := &batchv1.Job{}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Name: runtimeImagePrefetchJobName(runtimeImage, &readyNode, nil), Namespace: config.JobNamespace}, createdJob); err != nil {
		t.Fatalf("expected a Job for the node without a deleting Job: %v", err)
	}
}

func TestEnsureRuntimeImagePrefetchJobsContinuesAfterAPIErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	image := "example.com/vllm:v1"
	config := &v1beta1.KernelCacheConfig{
		JobNamespace: "jobs", PrefetchImage: "example.com/prefetch:v1",
		RuntimeImagePrefetch: v1beta1.KernelCacheRuntimeImagePrefetchConfig{Enabled: true},
	}
	nodes := &corev1.NodeList{}
	names := map[string]string{}
	for _, name := range []string{"get", "create", "delete", "deleting", "healthy"} {
		node := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name)}}
		nodes.Items = append(nodes.Items, node)
		names[name] = runtimeImagePrefetchJobName(image, &node, nil)
	}
	failedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: names["delete"], Namespace: "jobs", UID: "failed-job"},
		Status:     batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}},
	}
	now := metav1.Now()
	deletingJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: names["deleting"], Namespace: "jobs", DeletionTimestamp: &now,
		Finalizers: []string{"test.kserve.io/prefetch-job"},
	}}
	getErr := errors.New("get failed")
	createErr := errors.New("create failed")
	deleteErr := errors.New("delete failed")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(failedJob, deletingJob).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == names["get"] {
					return getErr
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if obj.GetName() == names["create"] {
					return createErr
				}
				return c.Create(ctx, obj, opts...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if obj.GetName() == names["delete"] {
					return deleteErr
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()
	r := &KernelCacheReconciler{Client: k8sClient}
	needsRequeue, err := r.ensureRuntimeImagePrefetchJobs(t.Context(), kernelCacheWithRuntimeImage("cache", image), &v1alpha1.KernelCacheNodeGroup{}, nodes, config)
	if !needsRequeue {
		t.Fatal("expected deletion requeue to survive other nodes' API errors")
	}
	for _, want := range []error{getErr, createErr, deleteErr} {
		if !errors.Is(err, want) {
			t.Fatalf("expected joined error to include %v, got %v", want, err)
		}
	}
	if err := k8sClient.Get(t.Context(), client.ObjectKey{Namespace: "jobs", Name: names["healthy"]}, &batchv1.Job{}); err != nil {
		t.Fatalf("expected healthy node Job despite earlier API errors: %v", err)
	}
	if err := k8sClient.Get(t.Context(), client.ObjectKeyFromObject(failedJob), &batchv1.Job{}); err != nil {
		t.Fatalf("expected failed Job to remain after delete error: %v", err)
	}
}

func TestEnsureRuntimeImagePrefetchJobRequeuesAfterCreateRace(t *testing.T) {
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
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, object client.Object, options ...client.CreateOption) error {
			return apierrors.NewAlreadyExists(schema.GroupResource{Resource: "jobs"}, object.GetName())
		},
	}).Build()
	reconciler := &KernelCacheReconciler{Client: k8sClient}

	needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), runtimeImage, nil, node, config)
	if err != nil {
		t.Fatal(err)
	}
	if !needsRequeue {
		t.Fatal("expected a requeue after a concurrent Job create")
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
		{name: "malformed runtime image", enabled: true, image: "not a valid image reference"},
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

			needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJobs(t.Context(), kernelCache, &v1alpha1.KernelCacheNodeGroup{}, nodes, config)
			if err != nil {
				t.Fatal(err)
			}
			if needsRequeue {
				t.Fatal("did not expect a requeue when runtime image prefetch is skipped")
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
		needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJobs(t.Context(), kernelCacheWithRuntimeImage(name, image), nodeGroup, nodes, config)
		if err != nil {
			t.Fatal(err)
		}
		if needsRequeue {
			t.Fatal("did not expect a requeue for an existing runtime image prefetch Job")
		}
	}
	jobs := &batchv1.JobList{}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected KCs to share one runtime image prefetch Job, got %d", len(jobs.Items))
	}

	toleratedNodeGroup := &v1alpha1.KernelCacheNodeGroup{Spec: v1alpha1.KernelCacheNodeGroupSpec{
		Tolerations: []corev1.Toleration{{Key: "gpu", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}},
	}}
	for _, name := range []string{"cache-c", "cache-d"} {
		needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJobs(t.Context(), kernelCacheWithRuntimeImage(name, image), toleratedNodeGroup, nodes, config)
		if err != nil {
			t.Fatal(err)
		}
		if needsRequeue {
			t.Fatal("did not expect a requeue for a runtime image prefetch Job with matching tolerations")
		}
	}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 2 {
		t.Fatalf("expected distinct Jobs for different tolerations, got %d", len(jobs.Items))
	}

	replacement := node.DeepCopy()
	replacement.UID = types.UID("uid-b")
	needsRequeue, err := reconciler.ensureRuntimeImagePrefetchJob(t.Context(), image, nil, replacement, config)
	if err != nil {
		t.Fatal(err)
	}
	if needsRequeue {
		t.Fatal("did not expect a requeue after creating a Job for a replacement node")
	}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 3 {
		t.Fatalf("expected a new Job for the replacement node UID, got %d", len(jobs.Items))
	}

	needsRequeue, err = reconciler.ensureRuntimeImagePrefetchJob(t.Context(), "example.com/vllm:v2", nil, node, config)
	if err != nil {
		t.Fatal(err)
	}
	if needsRequeue {
		t.Fatal("did not expect a requeue after creating a Job for a new runtime image")
	}
	if err := k8sClient.List(t.Context(), jobs, client.InNamespace(config.JobNamespace)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 4 {
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
