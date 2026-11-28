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

package llmisvc

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestAdapterWarmCopyTrackerUpdatesSnapshotsAndOperationResults(t *testing.T) {
	tracker := newAdapterWarmCopyTracker()
	known := []string{"adapter-a", "adapter-b"}

	tracker.UpdatePodSnapshot("pod-1", known, []string{"adapter-a"})
	if got := tracker.WarmCopyCount("adapter-a"); got != 1 {
		t.Fatalf("adapter-a warm copies = %d, want 1", got)
	}
	if got := tracker.WarmCopyCount("adapter-b"); got != 0 {
		t.Fatalf("adapter-b warm copies = %d, want 0", got)
	}

	// Reapplying a complete snapshot must be idempotent.
	tracker.UpdatePodSnapshot("pod-1", known, []string{"adapter-a"})
	if got := tracker.WarmCopyCount("adapter-a"); got != 1 {
		t.Fatalf("adapter-a warm copies after duplicate snapshot = %d, want 1", got)
	}

	tracker.UpdatePodSnapshot("pod-2", known, []string{"adapter-a"})
	if got := tracker.WarmCopyCount("adapter-a"); got != 2 {
		t.Fatalf("adapter-a warm copies after second pod = %d, want 2", got)
	}

	tracker.UpdatePodSnapshot("pod-1", known, nil)
	if got := tracker.WarmCopyCount("adapter-a"); got != 1 {
		t.Fatalf("adapter-a warm copies after unload snapshot = %d, want 1", got)
	}

	tracker.MarkPodUnknown("pod-2", known...)
	if got := tracker.WarmCopyCount("adapter-a"); got != 0 {
		t.Fatalf("adapter-a warm copies after unknown observation = %d, want 0", got)
	}
	if state, ok := tracker.Registration("adapter-a", "pod-2"); !ok || state != adapterRegistrationUnknown {
		t.Fatalf("pod-2 registration = (%v, %t), want unknown and present", state, ok)
	}

	tracker.RecordOperationResult("pod-1", "adapter-a", true)
	if got := tracker.WarmCopyCount("adapter-a"); got != 1 {
		t.Fatalf("adapter-a warm copies after successful load = %d, want 1", got)
	}
	tracker.RecordOperationResult("pod-1", "adapter-a", false)
	if got := tracker.WarmCopyCount("adapter-a"); got != 0 {
		t.Fatalf("adapter-a warm copies after successful unload = %d, want 0", got)
	}
}

func TestAdapterWarmCopyTrackerRemovesDeletedPods(t *testing.T) {
	tracker := newAdapterWarmCopyTracker()
	tracker.UpdatePodSnapshot("pod-1", []string{"adapter-a"}, []string{"adapter-a"})
	tracker.UpdatePodSnapshot("pod-2", []string{"adapter-a"}, []string{"adapter-a"})

	tracker.RemovePodsNotIn(map[types.UID]struct{}{"pod-2": {}})
	if got := tracker.WarmCopyCount("adapter-a"); got != 1 {
		t.Fatalf("adapter-a warm copies after pod removal = %d, want 1", got)
	}
	if _, ok := tracker.Registration("adapter-a", "pod-1"); ok {
		t.Fatal("deleted pod registration is still tracked")
	}
	if state, ok := tracker.Registration("adapter-a", "pod-2"); !ok || state != adapterRegistrationPresent {
		t.Fatalf("remaining pod registration = (%v, %t), want present and tracked", state, ok)
	}
}

func TestRegisteredAdapterNamesMatchesShortAndQualifiedAliases(t *testing.T) {
	qualifiedName := fullyQualifiedModelName("test-ns", "adapter-a")
	targets := []adapterWarmTarget{{
		name:    "adapter-a",
		aliases: []string{"adapter-a", qualifiedName},
	}}
	known := map[string]struct{}{"adapter-a": {}, "previous-adapter": {}}

	got := registeredAdapterNames("test-ns", targets, []string{
		qualifiedName,
		"previous-adapter",
		"base-model",
	}, known)
	want := []string{"adapter-a", "previous-adapter"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registered adapters = %v, want %v", got, want)
	}
}

func TestObserveAdapterPodsUpdatesWarmCopyCount(t *testing.T) {
	tracker := newAdapterWarmCopyTracker()
	known := []string{"adapter-a"}
	targets := []adapterWarmTarget{{
		name:    "adapter-a",
		aliases: []string{"adapter-a", fullyQualifiedModelName("test-ns", "adapter-a")},
	}}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "pod-1", UID: "pod-1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "127.0.0.1"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "pod-2", UID: "pod-2"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "127.0.0.2"},
		},
	}

	fetchModels := func(_ context.Context, pod *corev1.Pod) ([]string, error) {
		if pod.Name == "pod-1" {
			return []string{fullyQualifiedModelName("test-ns", "adapter-a")}, nil
		}
		return nil, nil
	}
	if errs := observeAdapterPods(context.Background(), "test-ns", pods, targets, known, tracker, fetchModels); len(errs) != 0 {
		t.Fatalf("observeAdapterPods returned errors: %v", errs)
	}
	if got := tracker.WarmCopyCount("adapter-a"); got != 1 {
		t.Fatalf("adapter-a warm copies = %d, want 1", got)
	}
	if state, ok := tracker.Registration("adapter-a", "pod-2"); !ok || state != adapterRegistrationAbsent {
		t.Fatalf("pod-2 registration = (%v, %t), want absent and tracked", state, ok)
	}

	fetchModels = func(_ context.Context, pod *corev1.Pod) ([]string, error) {
		if pod.Name == "pod-1" {
			return nil, context.DeadlineExceeded
		}
		return nil, nil
	}
	if errs := observeAdapterPods(context.Background(), "test-ns", pods, targets, known, tracker, fetchModels); len(errs) != 1 {
		t.Fatalf("observeAdapterPods errors = %d, want 1", len(errs))
	}
	if got := tracker.WarmCopyCount("adapter-a"); got != 0 {
		t.Fatalf("adapter-a warm copies after failed observation = %d, want 0", got)
	}
	if state, ok := tracker.Registration("adapter-a", "pod-1"); !ok || state != adapterRegistrationUnknown {
		t.Fatalf("pod-1 registration = (%v, %t), want unknown and tracked", state, ok)
	}
}

func TestFetchAdapterModelIDs(t *testing.T) {
	httpClient := &http.Client{Transport: adapterTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s, want GET /v1/models", request.Method, request.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"adapter-a"},{"id":""}]}`)),
			Header:     make(http.Header),
			Request:    request,
		}, nil
	})}
	got, err := fetchAdapterModelIDs(context.Background(), httpClient, "http://model-server/v1/models")
	if err != nil {
		t.Fatalf("fetchAdapterModelIDs returned error: %v", err)
	}
	want := []string{"adapter-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model IDs = %v, want %v", got, want)
	}
}

type adapterTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip adapterTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
