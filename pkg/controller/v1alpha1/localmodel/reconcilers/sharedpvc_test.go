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
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
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
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{UID: "pvc-uid"}}
	completed := jobWithCondition(batchv1.JobComplete)
	completed.Status.CompletionTime = ptrTo(metav1.Now())
	tests := []struct {
		name              string
		job               *batchv1.Job
		wantStatus        metav1.ConditionStatus
		wantReason        string
		wantAvailable     int
		wantFailed        int
		wantImported      *v1alpha1.SharedPVCImportStatus
		wantClearImported bool
	}{
		{
			name:          "complete",
			job:           completed,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    v1alpha1.ReasonImportSucceeded,
			wantAvailable: 1,
			wantImported:  &v1alpha1.SharedPVCImportStatus{PVCUID: "pvc-uid", CompletionTime: completed.Status.CompletionTime},
		},
		{
			name:              "failed",
			job:               jobWithCondition(batchv1.JobFailed),
			wantStatus:        metav1.ConditionFalse,
			wantReason:        v1alpha1.ReasonImportFailed,
			wantFailed:        1,
			wantClearImported: true,
		},
		{
			name:              "running",
			job:               &batchv1.Job{Status: batchv1.JobStatus{Active: active}},
			wantStatus:        metav1.ConditionFalse,
			wantReason:        v1alpha1.ReasonImportRunning,
			wantClearImported: true,
		},
		{
			name:              "pending",
			job:               &batchv1.Job{},
			wantStatus:        metav1.ConditionFalse,
			wantReason:        v1alpha1.ReasonImportPending,
			wantClearImported: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := stateFromJob(tt.job, pvc)
			if !reflect.DeepEqual(state.imported, tt.wantImported) {
				t.Fatalf("imported = %#v, want %#v", state.imported, tt.wantImported)
			}
			if state.clearImported != tt.wantClearImported {
				t.Fatalf("clearImported = %v, want %v", state.clearImported, tt.wantClearImported)
			}
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

type consumerListErrorClient struct {
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

func (c *consumerListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v1beta1.InferenceServiceList); ok {
		return c.err
	}
	return c.Client.List(ctx, list, opts...)
}

func TestSetConsumerReferences(t *testing.T) {
	status := &v1alpha1.LocalModelCacheStatus{
		InferenceServices:    []v1alpha1.NamespacedName{{Name: "old-isvc", Namespace: "ns"}},
		LLMInferenceServices: []v1alpha1.NamespacedName{{Name: "old-llm", Namespace: "ns"}},
	}
	setConsumerReferences(status, cacheConsumers{
		isvcs: []v1beta1.InferenceService{{ObjectMeta: metav1.ObjectMeta{Name: "isvc", Namespace: "ns"}}},
	})
	if len(status.InferenceServices) != 1 || status.InferenceServices[0].Name != "isvc" {
		t.Fatalf("InferenceServices = %#v, want current consumer", status.InferenceServices)
	}
	if status.LLMInferenceServices != nil {
		t.Fatalf("LLMInferenceServices = %#v, want cleared", status.LLMInferenceServices)
	}
}

func TestCollectCacheConsumersReturnsListErrorWithoutPartialSnapshot(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns"},
	}
	baseClient := fake.NewClientBuilder().Build()
	readErr := errors.New("consumer list failed")
	reconcilerClient := &consumerListErrorClient{Client: baseClient, err: readErr}

	consumers, err := collectCacheConsumers(context.Background(), reconcilerClient, logr.Discard(), nil, cache, false)
	if !errors.Is(err, readErr) {
		t.Fatalf("collectCacheConsumers() error = %v, want %v", err, readErr)
	}
	if consumers.isvcs != nil || consumers.llmSvcs != nil {
		t.Fatalf("collectCacheConsumers() = %#v, want empty snapshot", consumers)
	}
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

func TestValidateExistingImportJobReplacesNonCompletedJobOnSpecHashChange(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "cache-uid"},
		Spec:       v1alpha1.LocalModelNamespaceCacheSpec{ServiceAccountName: "fixed-sa"},
	}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{UID: "pvc-uid"}}
	stale := cache.DeepCopy()
	stale.Spec.ServiceAccountName = "broken-sa"
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cache-import", Namespace: "ns",
			Annotations: map[string]string{
				importPVCUIDAnnotation:     string(pvc.UID),
				importStorageKeyAnnotation: "storage-key",
				importSpecHashAnnotation:   importSpecHash(stale),
			},
			OwnerReferences: []metav1.OwnerReference{{Name: cache.Name, UID: cache.UID, Controller: ptrTo(true)}},
		},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}},
	}
	trackingClient := &deleteTrackingClient{}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: trackingClient}

	gotJob, pending, err := reconciler.validateExistingImportJob(context.Background(), job, cache, pvc, "storage-key")
	if err != nil {
		t.Fatalf("validateExistingImportJob() error = %v", err)
	}
	if gotJob != nil || !pending {
		t.Fatalf("validateExistingImportJob() = (%v, %v), want (nil, true)", gotJob, pending)
	}
	if trackingClient.propagation == nil {
		t.Fatalf("expected the failed Job to be deleted after the credential spec changed")
	}
}

func TestValidateExistingImportJobKeepsCompletedJobOnSpecHashChange(t *testing.T) {
	cache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "ns", UID: "cache-uid"},
		Spec:       v1alpha1.LocalModelNamespaceCacheSpec{ServiceAccountName: "new-sa"},
	}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{UID: "pvc-uid"}}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cache-import", Namespace: "ns",
			Annotations: map[string]string{
				importPVCUIDAnnotation:     string(pvc.UID),
				importStorageKeyAnnotation: "storage-key",
				importSpecHashAnnotation:   "stale-hash",
			},
			OwnerReferences: []metav1.OwnerReference{{Name: cache.Name, UID: cache.UID, Controller: ptrTo(true)}},
		},
		Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
	}
	trackingClient := &deleteTrackingClient{}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: trackingClient}

	gotJob, pending, err := reconciler.validateExistingImportJob(context.Background(), job, cache, pvc, "storage-key")
	if err != nil {
		t.Fatalf("validateExistingImportJob() error = %v", err)
	}
	if gotJob != job || pending {
		t.Fatalf("validateExistingImportJob() = (%v, %v), want (job, false)", gotJob, pending)
	}
	if trackingClient.propagation != nil {
		t.Fatalf("completed Job must not be deleted on a credential spec change")
	}
}

func TestImportSpecHash(t *testing.T) {
	base := &v1alpha1.LocalModelNamespaceCache{}
	sameAsBase := &v1alpha1.LocalModelNamespaceCache{}
	if importSpecHash(base) != importSpecHash(sameAsBase) {
		t.Fatalf("importSpecHash() must be stable for equal specs")
	}
	withSA := base.DeepCopy()
	withSA.Spec.ServiceAccountName = "sa"
	if importSpecHash(withSA) == importSpecHash(base) {
		t.Fatalf("importSpecHash() must change when serviceAccountName changes")
	}
	key := "key"
	withStorage := base.DeepCopy()
	withStorage.Spec.Storage = &v1alpha1.LocalModelStorageSpec{StorageKey: &key}
	if importSpecHash(withStorage) == importSpecHash(base) {
		t.Fatalf("importSpecHash() must change when storage changes")
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

	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, nil, cacheConsumers{}); !errors.Is(err, readErr) {
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

func TestReconcileSharedPVCSurfacesCredentialErrorWithoutCreatingJob(t *testing.T) {
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
			Storage:        &v1alpha1.LocalModelStorageSpec{StorageKey: ptrTo("missing-key")},
		},
	}
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
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.LocalModelNamespaceCache{}).
		WithObjects(cache, pvc).
		Build()
	reconciler := &LocalModelNamespaceCacheReconciler{
		Client: cl,
		Scheme: scheme,
		// The storage secret exists but lacks the requested key, so credential injection fails.
		CredentialBuilder: credentials.NewCredentialBuilderFromConfig(cl, k8sfake.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: constants.DefaultStorageSpecSecret, Namespace: "ns"},
		}), credentials.CredentialConfig{}),
	}

	_, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, cacheConsumers{})
	if !errors.Is(err, errImportCredentials) {
		t.Fatalf("reconcileSharedPVC() error = %v, want errImportCredentials", err)
	}
	condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	if condition == nil || condition.Reason != v1alpha1.ReasonImportCredentialError {
		t.Fatalf("cache Ready condition = %#v, want %s", condition, v1alpha1.ReasonImportCredentialError)
	}
	if !strings.Contains(condition.Message, "missing-key") {
		t.Fatalf("condition message %q should name the unresolved storage key", condition.Message)
	}
	jobs := &batchv1.JobList{}
	if err := cl.List(context.Background(), jobs, client.InNamespace("ns")); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("expected no import Job to be created on credential error, got %d", len(jobs.Items))
	}
}

// sharedPVCFixture builds a shared-PVC cache, its claim, and a fake client with the given
// extra objects, ready for reconcileSharedPVC.
func sharedPVCFixture(t *testing.T, cache *v1alpha1.LocalModelNamespaceCache, pvc *corev1.PersistentVolumeClaim, extra ...client.Object) (client.WithWatch, *runtime.Scheme) {
	t.Helper()
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
	objects := append([]client.Object{cache, pvc}, extra...)
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.LocalModelNamespaceCache{}).
		WithObjects(objects...).
		Build(), scheme
}

func importedSharedCache(pvcUID types.UID) *v1alpha1.LocalModelNamespaceCache {
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
	if pvcUID != "" {
		cache.Status.SharedPVCImport = &v1alpha1.SharedPVCImportStatus{PVCUID: pvcUID}
	}
	return cache
}

func boundSharedPVC(uid types.UID) *corev1.PersistentVolumeClaim {
	pvc := pvcWith(fsMode(), []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}, "2Gi", corev1.ClaimBound, "2Gi")
	pvc.Name = "pvc"
	pvc.Namespace = "ns"
	pvc.UID = uid
	return pvc
}

func sharedConsumer() cacheConsumers {
	return cacheConsumers{isvcs: []v1beta1.InferenceService{{
		ObjectMeta: metav1.ObjectMeta{Name: "consumer", Namespace: "ns"},
	}}}
}

func listImportJobs(t *testing.T, cl client.Client) []batchv1.Job {
	t.Helper()
	jobs := &batchv1.JobList{}
	if err := cl.List(context.Background(), jobs, client.InNamespace("ns")); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	return jobs.Items
}

func TestReconcileSharedPVCBlocksReimportWhileConsumersRemain(t *testing.T) {
	cache := importedSharedCache("pvc-uid")
	pvc := boundSharedPVC("pvc-uid")
	cl, scheme := sharedPVCFixture(t, cache, pvc)
	reconciler := &LocalModelNamespaceCacheReconciler{Client: cl, Scheme: scheme}

	for i := range 2 {
		if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, sharedConsumer()); err != nil {
			t.Fatalf("reconcileSharedPVC() #%d error = %v, want nil: a blocked re-import is a steady state, not a retry", i+1, err)
		}
	}
	if jobs := listImportJobs(t, cl); len(jobs) != 0 {
		t.Fatalf("expected no replacement import Job while consumers remain, got %d", len(jobs))
	}
	condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != v1alpha1.ReasonReimportBlocked {
		t.Fatalf("cache Ready condition = %#v, want False/%s", condition, v1alpha1.ReasonReimportBlocked)
	}
	if !strings.Contains(condition.Message, "1 InferenceService") || !strings.Contains(condition.Message, "remove them") {
		t.Fatalf("condition message %q should count consumers and name the remedy", condition.Message)
	}
	if cache.Status.SharedPVCImport == nil || cache.Status.SharedPVCImport.PVCUID != "pvc-uid" {
		t.Fatalf("import record %#v must survive the blocked state; it is the only evidence of prior success", cache.Status.SharedPVCImport)
	}
}

func TestReconcileSharedPVCReimportsOnceConsumersRemoved(t *testing.T) {
	cache := importedSharedCache("pvc-uid")
	pvc := boundSharedPVC("pvc-uid")
	cl, scheme := sharedPVCFixture(t, cache, pvc)
	reconciler := &LocalModelNamespaceCacheReconciler{Client: cl, Scheme: scheme}

	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, sharedConsumer()); err != nil {
		t.Fatalf("blocked reconcile error = %v", err)
	}
	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, cacheConsumers{}); err != nil {
		t.Fatalf("reconcile after consumer removal error = %v", err)
	}
	if jobs := listImportJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected exactly one replacement import Job, got %d", len(jobs))
	}
	condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady)
	if condition == nil || condition.Reason != v1alpha1.ReasonImportPending {
		t.Fatalf("cache Ready condition = %#v, want %s", condition, v1alpha1.ReasonImportPending)
	}
	if cache.Status.SharedPVCImport != nil {
		t.Fatalf("import record %#v must be cleared once a fresh import is underway", cache.Status.SharedPVCImport)
	}
	// A second pass with the Job present is a no-op: still one Job.
	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, cacheConsumers{}); err != nil {
		t.Fatalf("repeat reconcile error = %v", err)
	}
	if jobs := listImportJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected the single Job to be reused, got %d", len(jobs))
	}
}

func TestReconcileSharedPVCReimportsOntoRecreatedPVCDespiteConsumers(t *testing.T) {
	cache := importedSharedCache("old-pvc-uid")
	pvc := boundSharedPVC("new-pvc-uid")
	cl, scheme := sharedPVCFixture(t, cache, pvc)
	reconciler := &LocalModelNamespaceCacheReconciler{Client: cl, Scheme: scheme}

	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, sharedConsumer()); err != nil {
		t.Fatalf("reconcileSharedPVC() error = %v", err)
	}
	if jobs := listImportJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("a recreated claim holds no data; expected one import Job, got %d", len(jobs))
	}
	if cache.Status.SharedPVCImport != nil {
		t.Fatalf("stale import record %#v for the old claim must be cleared", cache.Status.SharedPVCImport)
	}
}

func TestReconcileSharedPVCRetriesInitialImportWithoutRecord(t *testing.T) {
	// A cache that never became Ready has no import record: deleting its failed Job creates
	// exactly one replacement, as before.
	cache := importedSharedCache("")
	cache.Status = v1alpha1.LocalModelCacheStatus{}
	pvc := boundSharedPVC("pvc-uid")
	failed := jobWithCondition(batchv1.JobFailed)
	failed.Name = importJobName(cache.Name)
	failed.Namespace = cache.Namespace
	failed.UID = "failed-job-uid"
	failed.Annotations = map[string]string{
		importPVCUIDAnnotation:     string(pvc.UID),
		importStorageKeyAnnotation: v1alpha1.GetStorageKey(cache.Spec.SourceModelUri),
		importSpecHashAnnotation:   importSpecHash(cache),
	}
	cl, scheme := sharedPVCFixture(t, cache, pvc, failed)
	if err := controllerutil.SetControllerReference(cache, failed, scheme); err != nil {
		t.Fatalf("set owner: %v", err)
	}
	if err := cl.Update(context.Background(), failed); err != nil {
		t.Fatalf("update failed job owner: %v", err)
	}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: cl, Scheme: scheme}

	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, cacheConsumers{}); err != nil {
		t.Fatalf("reconcile with failed Job error = %v", err)
	}
	if condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady); condition == nil || condition.Reason != v1alpha1.ReasonImportFailed {
		t.Fatalf("cache Ready condition = %#v, want %s", condition, v1alpha1.ReasonImportFailed)
	}
	if jobs := listImportJobs(t, cl); len(jobs) != 1 || jobs[0].UID != failed.UID {
		t.Fatalf("the failed Job must be retained, got %d Job(s)", len(jobs))
	}
	if cache.Status.SharedPVCImport != nil {
		t.Fatalf("a failed import must not leave an import record, got %#v", cache.Status.SharedPVCImport)
	}

	if err := cl.Delete(context.Background(), failed); err != nil {
		t.Fatalf("delete failed job: %v", err)
	}
	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, cacheConsumers{}); err != nil {
		t.Fatalf("reconcile after Job deletion error = %v", err)
	}
	jobs := listImportJobs(t, cl)
	if len(jobs) != 1 || jobs[0].UID == failed.UID {
		t.Fatalf("expected exactly one replacement Job, got %d", len(jobs))
	}
}

func TestReconcileSharedPVCRecordsImportWhenJobCompletes(t *testing.T) {
	cache := importedSharedCache("")
	cache.Status = v1alpha1.LocalModelCacheStatus{}
	pvc := boundSharedPVC("pvc-uid")
	completed := jobWithCondition(batchv1.JobComplete)
	completed.Name = importJobName(cache.Name)
	completed.Namespace = cache.Namespace
	completed.Status.CompletionTime = ptrTo(metav1.Now())
	completed.Annotations = map[string]string{
		importPVCUIDAnnotation:     string(pvc.UID),
		importStorageKeyAnnotation: v1alpha1.GetStorageKey(cache.Spec.SourceModelUri),
		importSpecHashAnnotation:   importSpecHash(cache),
	}
	cl, scheme := sharedPVCFixture(t, cache, pvc, completed)
	if err := controllerutil.SetControllerReference(cache, completed, scheme); err != nil {
		t.Fatalf("set owner: %v", err)
	}
	if err := cl.Update(context.Background(), completed); err != nil {
		t.Fatalf("update completed job owner: %v", err)
	}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: cl, Scheme: scheme}

	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, cacheConsumers{}); err != nil {
		t.Fatalf("reconcileSharedPVC() error = %v", err)
	}
	if !cache.IsReady() {
		t.Fatalf("cache should be Ready, got %#v", cache.Status.GetCondition(v1alpha1.LocalModelCacheReady))
	}
	got := cache.Status.SharedPVCImport
	if got == nil || got.PVCUID != pvc.UID || got.CompletionTime == nil {
		t.Fatalf("import record = %#v, want PVC UID %q and a completion time", got, pvc.UID)
	}
}

func TestReconcileSharedPVCBlocksAfterTerminatingCompletedJobIsGone(t *testing.T) {
	cache := importedSharedCache("pvc-uid")
	pvc := boundSharedPVC("pvc-uid")
	terminating := jobWithCondition(batchv1.JobComplete)
	terminating.Name = importJobName(cache.Name)
	terminating.Namespace = cache.Namespace
	terminating.Finalizers = []string{"test.kserve.io/hold"}
	terminating.Annotations = map[string]string{
		importPVCUIDAnnotation:     string(pvc.UID),
		importStorageKeyAnnotation: v1alpha1.GetStorageKey(cache.Spec.SourceModelUri),
		importSpecHashAnnotation:   importSpecHash(cache),
	}
	cl, scheme := sharedPVCFixture(t, cache, pvc, terminating)
	if err := controllerutil.SetControllerReference(cache, terminating, scheme); err != nil {
		t.Fatalf("set owner: %v", err)
	}
	if err := cl.Update(context.Background(), terminating); err != nil {
		t.Fatalf("update job owner: %v", err)
	}
	// The finalizer keeps the Job around with a deletion timestamp.
	if err := cl.Delete(context.Background(), terminating); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	reconciler := &LocalModelNamespaceCacheReconciler{Client: cl, Scheme: scheme}

	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, sharedConsumer()); err != nil {
		t.Fatalf("reconcile with terminating Job error = %v", err)
	}
	if condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady); condition == nil || condition.Reason != v1alpha1.ReasonImportPending {
		t.Fatalf("cache Ready condition = %#v, want %s while the Job terminates", condition, v1alpha1.ReasonImportPending)
	}
	if cache.Status.SharedPVCImport == nil {
		t.Fatal("import record must survive while the completed Job terminates")
	}

	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(terminating), terminating); err != nil {
		t.Fatalf("get terminating job: %v", err)
	}
	terminating.Finalizers = nil
	if err := cl.Update(context.Background(), terminating); err != nil {
		t.Fatalf("release job: %v", err)
	}
	if _, err := reconciler.reconcileSharedPVC(context.Background(), cache, &corev1.ConfigMap{}, sharedConsumer()); err != nil {
		t.Fatalf("reconcile after Job is gone error = %v", err)
	}
	if jobs := listImportJobs(t, cl); len(jobs) != 0 {
		t.Fatalf("expected no replacement Job while consumers remain, got %d", len(jobs))
	}
	if condition := cache.Status.GetCondition(v1alpha1.LocalModelCacheReady); condition == nil || condition.Reason != v1alpha1.ReasonReimportBlocked {
		t.Fatalf("cache Ready condition = %#v, want %s", condition, v1alpha1.ReasonReimportBlocked)
	}
}

func TestReconcileSharedPVCConcurrentReconcilesCreateOneJob(t *testing.T) {
	cache := importedSharedCache("pvc-uid")
	pvc := boundSharedPVC("pvc-uid")
	cl, scheme := sharedPVCFixture(t, cache, pvc)

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each worker reconciles its own copy of the cache, as separate controller
			// instances would after removal of the last consumer.
			reconciler := &LocalModelNamespaceCacheReconciler{Client: cl, Scheme: scheme}
			_, errs[i] = reconciler.reconcileSharedPVC(context.Background(), cache.DeepCopy(), &corev1.ConfigMap{}, cacheConsumers{})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		// Only status-update conflicts are acceptable; they are retried by controller-runtime.
		if err != nil && !apierr.IsConflict(err) {
			t.Fatalf("worker %d error = %v", i, err)
		}
	}
	if jobs := listImportJobs(t, cl); len(jobs) != 1 {
		t.Fatalf("expected exactly one import Job across concurrent reconciles, got %d", len(jobs))
	}
}
