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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
