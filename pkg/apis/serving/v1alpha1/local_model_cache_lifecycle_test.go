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

package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReadyConditionObservedGeneration(t *testing.T) {
	cache := &LocalModelNamespaceCache{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	status := &cache.Status

	status.MarkNotReady(ReasonImportPending, "pending", 1)
	cond := status.GetCondition(LocalModelCacheReady)
	if cond == nil {
		t.Fatal("expected Ready condition")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("status = %v, want False", cond.Status)
	}
	if cond.ObservedGeneration != 1 {
		t.Fatalf("observedGeneration = %d, want 1", cond.ObservedGeneration)
	}
	if cache.IsReady() {
		t.Fatal("IsReady should be false")
	}
	firstTransition := cond.LastTransitionTime

	// Reason/message change at same status must not bump lastTransitionTime.
	status.MarkNotReady(ReasonImportRunning, "running", 2)
	cond = status.GetCondition(LocalModelCacheReady)
	if cond.Reason != ReasonImportRunning {
		t.Fatalf("reason = %q, want %q", cond.Reason, ReasonImportRunning)
	}
	if cond.ObservedGeneration != 2 {
		t.Fatalf("observedGeneration = %d, want 2", cond.ObservedGeneration)
	}
	if !cond.LastTransitionTime.Equal(&firstTransition) {
		t.Fatal("lastTransitionTime should not change when only reason/message change")
	}

	// Status change (False -> True) bumps lastTransitionTime.
	cache.Generation = 3
	status.MarkReady(3)
	cond = status.GetCondition(LocalModelCacheReady)
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("status = %v, want True", cond.Status)
	}
	if !cache.IsReady() {
		t.Fatal("IsReady should be true")
	}
	if cond.Reason != ReasonImportSucceeded {
		t.Fatalf("reason = %q, want %q", cond.Reason, ReasonImportSucceeded)
	}
}

func TestLocalModelNamespaceCacheIsReadyForCurrentGeneration(t *testing.T) {
	cache := &LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
	}
	cache.Status.MarkReady(1)

	if cache.IsReady() {
		t.Fatal("cache must not be ready when the Ready condition is stale")
	}

	cache.Status.MarkReady(2)
	if !cache.IsReady() {
		t.Fatal("cache should be ready when Ready=True for the current generation")
	}
}
