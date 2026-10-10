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
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestPreparationState(t *testing.T) {
	startedAt := metav1.Now()
	tests := []struct {
		name       string
		job        *batchv1.Job
		current    v1alpha1.KernelCacheNodePreparationState
		podFailure string
		wantState  v1alpha1.KernelCacheNodePreparationState
		wantReason string
	}{
		{
			name:       "no prefetch job",
			wantState:  v1alpha1.KernelCacheNodePreparationStatePending,
			wantReason: "waiting for the OCI prefetch Job",
		},
		{
			name:       "prefetch job is running",
			job:        &batchv1.Job{Status: batchv1.JobStatus{Active: 1, StartTime: &startedAt}},
			wantState:  v1alpha1.KernelCacheNodePreparationStatePulling,
			wantReason: "OCI artifact is being prefetched",
		},
		{
			name:       "prefetch pod image pull failed",
			job:        &batchv1.Job{Status: batchv1.JobStatus{Active: 1, StartTime: &startedAt}},
			podFailure: "Back-off pulling image",
			wantState:  v1alpha1.KernelCacheNodePreparationStateError,
			wantReason: "Back-off pulling image",
		},
		{
			name: "prefetch job completed",
			job: &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
			}}}},
			wantState:  v1alpha1.KernelCacheNodePreparationStateReady,
			wantReason: "OCI artifact was prefetched on the node",
		},
		{
			name: "prefetch job failed",
			job: &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "image pull failed",
			}}}},
			wantState:  v1alpha1.KernelCacheNodePreparationStateError,
			wantReason: "image pull failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, message := preparationState(test.job, test.current, test.podFailure)
			if state != test.wantState {
				t.Fatalf("expected state %q, got %q", test.wantState, state)
			}
			if test.wantReason != "" && message != test.wantReason {
				t.Fatalf("expected message %q, got %q", test.wantReason, message)
			}
		})
	}
}

// A deleted Job must not invalidate a cache that was already Ready.
func TestCachePreparationStateAfterJobCleanup(t *testing.T) {
	tests := []struct {
		name        string
		job         *batchv1.Job
		state       v1alpha1.KernelCacheNodePreparationState
		wantState   v1alpha1.KernelCacheNodePreparationState
		wantMessage string
	}{
		{
			name: "completed Job wins over missing Node image",
			job: &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
			}}}},
			state:       v1alpha1.KernelCacheNodePreparationStatePending,
			wantState:   v1alpha1.KernelCacheNodePreparationStateReady,
			wantMessage: "OCI artifact was prefetched on the node",
		},
		{
			name:        "ready cache remains ready after Job cleanup",
			state:       v1alpha1.KernelCacheNodePreparationStateReady,
			wantState:   v1alpha1.KernelCacheNodePreparationStateReady,
			wantMessage: "OCI artifact was prefetched on the node",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, message := cachePreparationState(test.job, v1alpha1.KernelCacheNodeCacheInfo{
				State: test.state,
			}, "")
			if state != test.wantState {
				t.Fatalf("expected state %q, got %q", test.wantState, state)
			}
			if message != test.wantMessage {
				t.Fatalf("expected message %q, got %q", test.wantMessage, message)
			}
		})
	}
}

func TestCacheImageValidationState(t *testing.T) {
	const imageReference = "registry.example.com/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	tests := []struct {
		name        string
		images      []string
		wantState   v1alpha1.KernelCacheNodePreparationState
		wantMessage string
	}{
		{
			name:        "image is present",
			images:      []string{imageReference},
			wantState:   v1alpha1.KernelCacheNodePreparationStateReady,
			wantMessage: "OCI artifact is present on the node",
		},
		{
			name:        "image is missing",
			wantState:   v1alpha1.KernelCacheNodePreparationStatePending,
			wantMessage: missingImageMessage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := &corev1.Node{Status: corev1.NodeStatus{Images: []corev1.ContainerImage{{Names: test.images}}}}
			state, message := cacheImageValidationState(v1alpha1.KernelCacheNodeCacheInfo{
				ImageReference: imageReference,
			}, node)
			if state != test.wantState {
				t.Fatalf("expected state %q, got %q", test.wantState, state)
			}
			if message != test.wantMessage {
				t.Fatalf("expected message %q, got %q", test.wantMessage, message)
			}
		})
	}
}
