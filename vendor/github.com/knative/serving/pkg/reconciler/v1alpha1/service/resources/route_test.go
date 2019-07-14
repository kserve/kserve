/*
Copyright 2018 The Knative Authors

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

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/service/resources/names"
)

func TestRouteRunLatest(t *testing.T) {
	s := createServiceWithRunLatest()
	testConfigName := names.Configuration(s)
	r, err := MakeRoute(s)
	if err != nil {
		t.Errorf("UnExpected error: %v", err)
	}
	if got, want := r.Name, testServiceName; got != want {
		t.Errorf("Expected %q for service name got %q", want, got)
	}
	if got, want := r.Namespace, testServiceNamespace; got != want {
		t.Errorf("Expected %q for service namespace got %q", want, got)
	}
	if got, want := len(r.Spec.Traffic), 1; got != want {
		t.Fatalf("Expected %d traffic targets got %d", want, got)
	}
	wantT := []v1alpha1.TrafficTarget{{
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:           100,
			ConfigurationName: testConfigName,
		},
	}}
	if got, want := r.Spec.Traffic, wantT; !cmp.Equal(got, want) {
		t.Errorf("Traffic mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)

	wantL := map[string]string{
		testLabelKey:            testLabelValueRunLatest,
		serving.ServiceLabelKey: testServiceName,
	}
	if got, want := r.Labels, wantL; !cmp.Equal(got, want) {
		t.Errorf("Labels mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
}

func TestRoutePinned(t *testing.T) {
	s := createServiceWithPinned()
	r, err := MakeRoute(s)
	if err != nil {
		t.Errorf("Expected nil for err got %q", err)
	}
	if got, want := r.Name, testServiceName; got != want {
		t.Errorf("Expected %q for service name got %q", want, got)
	}
	if got, want := r.Namespace, testServiceNamespace; got != want {
		t.Errorf("Expected %q for service namespace got %q", want, got)
	}
	if got, want := len(r.Spec.Traffic), 1; got != want {
		t.Fatalf("Expected %d traffic targets, got %d", want, got)
	}
	wantT := []v1alpha1.TrafficTarget{{
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:      100,
			RevisionName: testRevisionName,
		},
	}}
	if got, want := r.Spec.Traffic, wantT; !cmp.Equal(got, want) {
		t.Errorf("Traffic mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)

	wantL := map[string]string{
		testLabelKey:            testLabelValuePinned,
		serving.ServiceLabelKey: testServiceName,
	}
	if got, want := r.Labels, wantL; !cmp.Equal(got, want) {
		t.Errorf("Labels mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
}

func TestRouteReleaseSingleRevision(t *testing.T) {
	const numRevisions = 1
	s := createServiceWithRelease(numRevisions, 0 /*no rollout*/)
	testConfigName := names.Configuration(s)
	r, err := MakeRoute(s)
	if err != nil {
		t.Errorf("Expected nil for err got %q", err)
	}
	if got, want := r.Name, testServiceName; got != want {
		t.Errorf("Expected %q for service name got %q", want, got)
	}
	if got, want := r.Namespace, testServiceNamespace; got != want {
		t.Errorf("Expected %q for service namespace got %q", want, got)
	}
	wantT := []v1alpha1.TrafficTarget{{
		Name: v1alpha1.CurrentTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:      100,
			RevisionName: testRevisionName,
		},
	}, {
		Name: v1alpha1.LatestTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			ConfigurationName: testConfigName,
		},
	}}
	if got, want := r.Spec.Traffic, wantT; !cmp.Equal(got, want) {
		t.Errorf("Traffic mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)
	wantL := map[string]string{
		testLabelKey:            testLabelValueRelease,
		serving.ServiceLabelKey: testServiceName,
	}
	if got, want := r.Labels, wantL; !cmp.Equal(got, want) {
		t.Errorf("Labels mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
}

func TestRouteLatestRevisionSplit(t *testing.T) {
	const (
		rolloutPercent = 42
		currentPercent = 100 - rolloutPercent
	)
	s := createServiceWithRelease(2 /*num revisions*/, rolloutPercent)
	s.Spec.Release.Revisions = []string{v1alpha1.ReleaseLatestRevisionKeyword, "juicy-revision"}
	testConfigName := names.Configuration(s)
	r, err := MakeRoute(s)
	if err != nil {
		t.Errorf("Expected nil for err got %q", err)
	}
	if got, want := r.Name, testServiceName; got != want {
		t.Errorf("Expected %q for service name got %q", want, got)
	}
	if got, want := r.Namespace, testServiceNamespace; got != want {
		t.Errorf("Expected %q for service namespace got %q", want, got)
	}
	wantT := []v1alpha1.TrafficTarget{{
		Name: v1alpha1.CurrentTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:           currentPercent,
			ConfigurationName: testConfigName,
		},
	}, {
		Name: v1alpha1.CandidateTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:      rolloutPercent,
			RevisionName: "juicy-revision",
		},
	}, {
		Name: v1alpha1.LatestTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			ConfigurationName: testConfigName,
		},
	}}
	if got, want := r.Spec.Traffic, wantT; !cmp.Equal(got, want) {
		t.Errorf("Traffic mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)

	wantL := map[string]string{
		testLabelKey:            testLabelValueRelease,
		serving.ServiceLabelKey: testServiceName,
	}
	if got, want := r.Labels, wantL; !cmp.Equal(got, want) {
		t.Errorf("Labels mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
}
func TestRouteLatestRevisionSplitCandidate(t *testing.T) {
	const (
		rolloutPercent = 42
		currentPercent = 100 - rolloutPercent
	)
	s := createServiceWithRelease(2 /*num revisions*/, rolloutPercent)
	s.Spec.Release.Revisions = []string{"squishy-revision", v1alpha1.ReleaseLatestRevisionKeyword}
	testConfigName := names.Configuration(s)
	r, err := MakeRoute(s)
	if err != nil {
		t.Errorf("Expected nil for err got %q", err)
	}
	if got, want := r.Name, testServiceName; got != want {
		t.Errorf("Expected %q for service name got %q", want, got)
	}
	if got, want := r.Namespace, testServiceNamespace; got != want {
		t.Errorf("Expected %q for service namespace got %q", want, got)
	}
	wantT := []v1alpha1.TrafficTarget{{
		Name: v1alpha1.CurrentTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:      currentPercent,
			RevisionName: "squishy-revision",
		},
	}, {
		Name: v1alpha1.CandidateTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:           rolloutPercent,
			ConfigurationName: testConfigName,
		},
	}, {
		Name: v1alpha1.LatestTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			ConfigurationName: testConfigName,
		},
	}}
	if got, want := r.Spec.Traffic, wantT; !cmp.Equal(got, want) {
		t.Errorf("Traffic mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)

	wantL := map[string]string{
		testLabelKey:            testLabelValueRelease,
		serving.ServiceLabelKey: testServiceName,
	}
	if got, want := r.Labels, wantL; !cmp.Equal(got, want) {
		t.Errorf("Labels mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
}
func TestRouteLatestRevisionNoSplit(t *testing.T) {
	s := createServiceWithRelease(1 /*num revisions*/, 0 /*unused*/)
	s.Spec.Release.Revisions = []string{v1alpha1.ReleaseLatestRevisionKeyword}
	testConfigName := names.Configuration(s)
	r, err := MakeRoute(s)

	if err != nil {
		t.Errorf("Expected nil for err got %q", err)
	}
	if got, want := r.Name, testServiceName; got != want {
		t.Errorf("Expected %q for service name got %q", want, got)
	}
	if got, want := r.Namespace, testServiceNamespace; got != want {
		t.Errorf("Expected %q for service namespace got %q", want, got)
	}
	// Should have 2 named traffic targets (current, latest)
	wantT := []v1alpha1.TrafficTarget{{
		Name: v1alpha1.CurrentTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:           100,
			ConfigurationName: testConfigName,
		},
	}, {
		Name: v1alpha1.LatestTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			ConfigurationName: testConfigName,
		},
	}}
	if got, want := r.Spec.Traffic, wantT; !cmp.Equal(got, want) {
		t.Errorf("Traffic mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)

	wantL := map[string]string{
		testLabelKey:            testLabelValueRelease,
		serving.ServiceLabelKey: testServiceName,
	}
	if got, want := r.Labels, wantL; !cmp.Equal(got, want) {
		t.Errorf("Labels mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
}

func TestRouteReleaseTwoRevisions(t *testing.T) {
	const (
		currentPercent = 52
		numRevisions   = 2
	)
	s := createServiceWithRelease(numRevisions, 100-currentPercent)
	testConfigName := names.Configuration(s)
	r, err := MakeRoute(s)
	if err != nil {
		t.Errorf("Expected nil for err got %q", err)
	}
	if got, want := r.Name, testServiceName; got != want {
		t.Errorf("Expected %q for service name got %q", want, got)
	}
	if got, want := r.Namespace, testServiceNamespace; got != want {
		t.Errorf("Expected %q for service namespace got %q", want, got)
	}
	// Should have 3 named traffic targets (current, candidate, latest)
	wantT := []v1alpha1.TrafficTarget{{
		Name: v1alpha1.CurrentTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:      currentPercent,
			RevisionName: testRevisionName,
		},
	}, {
		Name: v1alpha1.CandidateTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			Percent:      100 - currentPercent,
			RevisionName: testCandidateRevisionName,
		},
	}, {
		Name: v1alpha1.LatestTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			ConfigurationName: testConfigName,
		},
	}}
	if got, want := r.Spec.Traffic, wantT; !cmp.Equal(got, want) {
		t.Errorf("Traffic mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
	expectOwnerReferencesSetCorrectly(t, r.OwnerReferences)
	wantL := map[string]string{
		testLabelKey:            testLabelValueRelease,
		serving.ServiceLabelKey: testServiceName,
	}
	if got, want := r.Labels, wantL; !cmp.Equal(got, want) {
		t.Errorf("Labels mismatch: diff (-got, +want): %s", cmp.Diff(got, want))
	}
}

// MakeRoute is not called with a ManualType service, but if it is
// called it should produce a route with no traffic targets
func TestRouteManual(t *testing.T) {
	s := createServiceWithManual()
	r, err := MakeRoute(s)
	if r != nil {
		t.Errorf("Expected nil for r got %q", err)
	}
	if err == nil {
		t.Error("Expected err got nil")
	}
}
