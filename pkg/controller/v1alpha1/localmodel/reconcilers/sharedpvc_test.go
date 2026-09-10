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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func fsMode() *corev1.PersistentVolumeMode {
	m := corev1.PersistentVolumeFilesystem
	return &m
}

func blockMode() *corev1.PersistentVolumeMode {
	m := corev1.PersistentVolumeBlock
	return &m
}

func pvcWith(mode *corev1.PersistentVolumeMode, accessModes []corev1.PersistentVolumeAccessMode, request string, phase corev1.PersistentVolumeClaimPhase, capacity string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeMode:  mode,
			AccessModes: accessModes,
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: phase},
	}
	if request != "" {
		pvc.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(request)}
	}
	if capacity != "" {
		pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(capacity)}
	}
	return pvc
}

func TestCheckPVCPreflight(t *testing.T) {
	rwx := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	rwo := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	modelSize := resource.MustParse("10Gi")

	tests := []struct {
		name       string
		pvc        *corev1.PersistentVolumeClaim
		wantOK     bool
		wantReason string
	}{
		{
			name:   "valid bound rwx filesystem",
			pvc:    pvcWith(fsMode(), rwx, "20Gi", corev1.ClaimBound, "20Gi"),
			wantOK: true,
		},
		{
			name:   "nil volume mode defaults to filesystem",
			pvc:    pvcWith(nil, rwx, "20Gi", corev1.ClaimBound, "20Gi"),
			wantOK: true,
		},
		{
			name:   "unbound but sufficient request proceeds",
			pvc:    pvcWith(fsMode(), rwx, "20Gi", corev1.ClaimPending, ""),
			wantOK: true,
		},
		{
			name:       "block mode rejected",
			pvc:        pvcWith(blockMode(), rwx, "20Gi", corev1.ClaimBound, "20Gi"),
			wantOK:     false,
			wantReason: v1alpha1.ReasonUnsupportedVolumeMode,
		},
		{
			name:       "no rwx rejected",
			pvc:        pvcWith(fsMode(), rwo, "20Gi", corev1.ClaimBound, "20Gi"),
			wantOK:     false,
			wantReason: v1alpha1.ReasonUnsupportedAccessMode,
		},
		{
			name:       "insufficient request rejected",
			pvc:        pvcWith(fsMode(), rwx, "5Gi", corev1.ClaimPending, ""),
			wantOK:     false,
			wantReason: v1alpha1.ReasonInsufficientCapacity,
		},
		{
			name:       "insufficient bound capacity rejected",
			pvc:        pvcWith(fsMode(), rwx, "20Gi", corev1.ClaimBound, "5Gi"),
			wantOK:     false,
			wantReason: v1alpha1.ReasonInsufficientCapacity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, ok := checkPVCPreflight(tt.pvc, modelSize)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (reason %q)", ok, tt.wantOK, state.reason)
			}
			if !tt.wantOK && state.reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", state.reason, tt.wantReason)
			}
		})
	}
}

func jobWithCondition(condType batchv1.JobConditionType) *batchv1.Job {
	return &batchv1.Job{Status: batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: condType, Status: corev1.ConditionTrue}},
	}}
}

func TestStateFromJob(t *testing.T) {
	active := int32(1)
	tests := []struct {
		name          string
		job           *batchv1.Job
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantAvailable int
		wantFailed    int
	}{
		{
			name:          "complete",
			job:           jobWithCondition(batchv1.JobComplete),
			wantStatus:    metav1.ConditionTrue,
			wantReason:    v1alpha1.ReasonImportSucceeded,
			wantAvailable: 1,
		},
		{
			name:       "failed",
			job:        jobWithCondition(batchv1.JobFailed),
			wantStatus: metav1.ConditionFalse,
			wantReason: v1alpha1.ReasonImportFailed,
			wantFailed: 1,
		},
		{
			name:       "running",
			job:        &batchv1.Job{Status: batchv1.JobStatus{Active: active}},
			wantStatus: metav1.ConditionFalse,
			wantReason: v1alpha1.ReasonImportRunning,
		},
		{
			name:       "pending",
			job:        &batchv1.Job{},
			wantStatus: metav1.ConditionFalse,
			wantReason: v1alpha1.ReasonImportPending,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := stateFromJob(tt.job)
			if state.status != tt.wantStatus {
				t.Fatalf("status = %v, want %v", state.status, tt.wantStatus)
			}
			if state.reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", state.reason, tt.wantReason)
			}
			if state.available != tt.wantAvailable {
				t.Fatalf("available = %d, want %d", state.available, tt.wantAvailable)
			}
			if state.failed != tt.wantFailed {
				t.Fatalf("failed = %d, want %d", state.failed, tt.wantFailed)
			}
		})
	}
}

func TestImportJobName(t *testing.T) {
	short := importJobName("mymodel")
	if short != "mymodel-import" {
		t.Fatalf("short name = %q, want mymodel-import", short)
	}

	long := strings.Repeat("a", 80)
	name := importJobName(long)
	if len(name) > 63 {
		t.Fatalf("long name length = %d, want <= 63", len(name))
	}
	if !strings.HasSuffix(name, importJobNameSuffix) {
		t.Fatalf("long name %q missing suffix", name)
	}
	// Deterministic.
	if name != importJobName(long) {
		t.Fatalf("importJobName is not deterministic")
	}
	// Distinct long names must not collide.
	other := importJobName(strings.Repeat("a", 79) + "b")
	if name == other {
		t.Fatalf("distinct long cache names collided on job name %q", name)
	}
}

type deleteTrackingClient struct {
	client.Client
	propagation *metav1.DeletionPropagation
	getErr      error
}

type jobReadErrorClient struct {
	client.Client
	err error
}

func (c *deleteTrackingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.getErr != nil {
		return c.getErr
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *deleteTrackingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	options := client.DeleteOptions{}
	for _, opt := range opts {
		opt.ApplyToDelete(&options)
	}
	c.propagation = options.PropagationPolicy
	return nil
}

func (c *jobReadErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*batchv1.Job); ok {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestValidateExistingImportJobUsesForegroundDeletion(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "cache-uid"}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{UID: "pvc-uid"}}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cache-import", Namespace: "ns",
			Annotations: map[string]string{
				importPVCUIDAnnotation:     "old-pvc-uid",
				importStorageKeyAnnotation: "old-storage-key",
			},
			OwnerReferences: []metav1.OwnerReference{{Name: cache.Name, UID: cache.UID, Controller: ptrTo(true)}},
		},
	}
	trackingClient := &deleteTrackingClient{}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: trackingClient}

	gotJob, pending, err := reconciler.validateExistingImportJob(context.Background(), job, cache, pvc, "new-storage-key")
	if err != nil {
		t.Fatalf("validateExistingImportJob() error = %v", err)
	}
	if gotJob != nil || !pending {
		t.Fatalf("validateExistingImportJob() = (%v, %v), want (nil, true)", gotJob, pending)
	}
	if trackingClient.propagation == nil || *trackingClient.propagation != metav1.DeletePropagationForeground {
		t.Fatalf("delete propagation = %v, want %v", trackingClient.propagation, metav1.DeletePropagationForeground)
	}
}

func TestValidateExistingImportJobWaitsForTerminatingMatchingJob(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "cache-uid"}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{UID: "pvc-uid"}}
	deletionTimestamp := metav1.Now()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cache-import", Namespace: "ns", DeletionTimestamp: &deletionTimestamp,
			Annotations: map[string]string{
				importPVCUIDAnnotation:     string(pvc.UID),
				importStorageKeyAnnotation: "storage-key",
			},
			OwnerReferences: []metav1.OwnerReference{{Name: cache.Name, UID: cache.UID, Controller: ptrTo(true)}},
		},
	}
	reconciler := &LocalModelNamespaceCacheReconciler{}

	gotJob, pending, err := reconciler.validateExistingImportJob(context.Background(), job, cache, pvc, "storage-key")
	if err != nil {
		t.Fatalf("validateExistingImportJob() error = %v", err)
	}
	if gotJob != job || !pending {
		t.Fatalf("validateExistingImportJob() = (%v, %v), want (job, true)", gotJob, pending)
	}
}

func TestReconcileSharedPVCPreservesNotReadyOnImportJobReadError(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cache",
			Namespace:  "ns",
			Finalizers: []string{NamespaceCacheFinalizerName},
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://bucket/model",
			ModelSize:      resource.MustParse("1Gi"),
			PVCRef:         ptrTo("pvc"),
		},
	}
	cache.Status.MarkReady(cache.Generation)
	pvc := pvcWith(fsMode(), []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}, "2Gi", corev1.ClaimPending, "")
	pvc.Name = "pvc"
	pvc.Namespace = "ns"

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KServe scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch scheme: %v", err)
	}
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.LocalModelNamespaceCache{}).
		WithObjects(cache, pvc).
		Build()
	readErr := errors.New("import Job read failed")
	reconciler := &LocalModelNamespaceCacheReconciler{
		Client: &jobReadErrorClient{Client: baseClient, err: readErr},
	}

	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, nil); !errors.Is(err, readErr) {
		t.Fatalf("reconcileSharedPVC() error = %v, want %v", err, readErr)
	}
	condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	if condition == nil || condition.Reason != v1alpha1.ReasonImportPending {
		t.Fatalf("cache Ready condition = %#v, want ImportPending", condition)
	}
}

func TestFinalizeSharedPVCDeletesForegroundAndRetainsFinalizer(t *testing.T) {
	deletionTimestamp := metav1.Now()
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cache",
			Namespace:         "ns",
			UID:               "cache-uid",
			DeletionTimestamp: &deletionTimestamp,
			Finalizers:        []string{NamespaceCacheFinalizerName},
		},
	}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:      "cache-import",
		Namespace: "ns",
		OwnerReferences: []metav1.OwnerReference{{
			Name:       cache.Name,
			UID:        cache.UID,
			Controller: ptrTo(true),
		}},
	}}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KServe scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch scheme: %v", err)
	}
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.LocalModelNamespaceCache{}).
		WithObjects(cache, job).
		Build()
	trackingClient := &deleteTrackingClient{Client: baseClient}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: trackingClient, APIReader: trackingClient}

	if _, err := reconciler.finalizeSharedPVC(context.Background(), cache); err != nil {
		t.Fatalf("finalizeSharedPVC() error = %v", err)
	}
	if trackingClient.propagation == nil || *trackingClient.propagation != metav1.DeletePropagationForeground {
		t.Fatalf("delete propagation = %v, want %v", trackingClient.propagation, metav1.DeletePropagationForeground)
	}
	if !containsString(cache.Finalizers, NamespaceCacheFinalizerName) {
		t.Fatalf("cache finalizers = %v, want %q retained", cache.Finalizers, NamespaceCacheFinalizerName)
	}
	condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	if condition == nil || condition.Reason != v1alpha1.ReasonImportPending {
		t.Fatalf("cache Ready condition = %#v, want ImportPending", condition)
	}
}

func TestFinalizeSharedPVCPreservesNotReadyOnJobReadError(t *testing.T) {
	deletionTimestamp := metav1.Now()
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cache",
			Namespace:         "ns",
			UID:               "cache-uid",
			DeletionTimestamp: &deletionTimestamp,
			Finalizers:        []string{NamespaceCacheFinalizerName},
		},
	}
	cache.Status.MarkReady(cache.Generation)
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KServe scheme: %v", err)
	}
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.LocalModelNamespaceCache{}).
		WithObjects(cache).
		Build()
	readErr := errors.New("job read failed")
	trackingClient := &deleteTrackingClient{Client: baseClient, getErr: readErr}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: trackingClient, APIReader: trackingClient}

	if _, err := reconciler.finalizeSharedPVC(context.Background(), cache); !errors.Is(err, readErr) {
		t.Fatalf("finalizeSharedPVC() error = %v, want %v", err, readErr)
	}
	if !containsString(cache.Finalizers, NamespaceCacheFinalizerName) {
		t.Fatalf("cache finalizers = %v, want %q retained", cache.Finalizers, NamespaceCacheFinalizerName)
	}
	condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	if condition == nil || condition.Reason != v1alpha1.ReasonImportPending {
		t.Fatalf("cache Ready condition = %#v, want ImportPending", condition)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func ptrTo[T any](value T) *T {
	return &value
}
