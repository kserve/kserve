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

package queue

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/serving/pkg/autoscaler"
)

const (
	podName = "pod"
)

func TestNoData(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)

	got := s.report(now)
	want := &autoscaler.Stat{
		Time:                      &now,
		PodName:                   podName,
		AverageConcurrentRequests: 0.0,
		RequestCount:              0,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestSingleRequestWholeTime(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)

	s.requestStart(now)
	now = now.Add(1 * time.Second)
	s.requestEnd(now)

	got := s.report(now)

	want := &autoscaler.Stat{
		Time:                      &now,
		PodName:                   podName,
		AverageConcurrentRequests: 1.0,
		RequestCount:              1,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestSingleRequestHalfTime(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)

	s.requestStart(now)
	now = now.Add(1 * time.Second)
	s.requestEnd(now)
	now = now.Add(1 * time.Second)
	got := s.report(now)

	want := &autoscaler.Stat{
		Time:                      &now,
		PodName:                   podName,
		AverageConcurrentRequests: 0.5,
		RequestCount:              1,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestVeryShortLivedRequest(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)

	s.requestStart(now)
	now = now.Add(10 * time.Millisecond)
	s.requestEnd(now)

	now = now.Add(990 * time.Millisecond) // make the second full
	got := s.report(now)

	want := &autoscaler.Stat{
		Time:                      &now,
		PodName:                   podName,
		AverageConcurrentRequests: float64(10) / float64(1000),
		RequestCount:              1,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestMultipleRequestsWholeTime(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)

	s.requestStart(now)
	now = now.Add(300 * time.Millisecond)
	s.requestEnd(now)

	s.requestStart(now)
	now = now.Add(300 * time.Millisecond)
	s.requestEnd(now)

	s.requestStart(now)
	now = now.Add(400 * time.Millisecond)
	s.requestEnd(now)

	got := s.report(now)

	want := &autoscaler.Stat{
		Time:                      &now,
		PodName:                   podName,
		AverageConcurrentRequests: 1.0,
		RequestCount:              3,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestMultipleRequestsInterleaved(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)

	s.requestStart(now)
	now = now.Add(100 * time.Millisecond)
	s.requestStart(now)
	now = now.Add(500 * time.Millisecond)
	s.requestEnd(now)
	now = now.Add(400 * time.Millisecond)
	s.requestEnd(now)

	got := s.report(now)

	want := &autoscaler.Stat{
		Time:                      &now,
		PodName:                   podName,
		AverageConcurrentRequests: 1.5,
		RequestCount:              2,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestOneRequestAcrossReportings(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)

	s.requestStart(now)
	now = now.Add(1 * time.Second)
	got1 := s.report(now)
	want1 := &autoscaler.Stat{
		Time:                      got1.Time,
		PodName:                   podName,
		AverageConcurrentRequests: 1.0,
		RequestCount:              1,
	}

	now = now.Add(500 * time.Millisecond)
	s.requestEnd(now)
	now = now.Add(500 * time.Millisecond)
	got2 := s.report(now)
	want2 := &autoscaler.Stat{
		Time:                      &now,
		PodName:                   podName,
		AverageConcurrentRequests: 0.5,
		RequestCount:              0,
	}

	if diff := cmp.Diff(want1, got1); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
	if diff := cmp.Diff(want2, got2); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestOneProxiedRequest(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)
	s.proxiedStart(now)
	now = now.Add(1 * time.Second)
	got := s.report(now)
	want := &autoscaler.Stat{
		Time:                             &now,
		PodName:                          podName,
		AverageConcurrentRequests:        1.0,
		AverageProxiedConcurrentRequests: 1.0,
		RequestCount:                     1,
		ProxiedRequestCount:              1,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestOneEndedProxiedRequest(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)
	s.proxiedStart(now)
	now = now.Add(500 * time.Millisecond)
	s.proxiedEnd(now)
	now = now.Add(500 * time.Millisecond)
	got := s.report(now)
	want := &autoscaler.Stat{
		Time:                             &now,
		PodName:                          podName,
		AverageConcurrentRequests:        0.5,
		AverageProxiedConcurrentRequests: 0.5,
		RequestCount:                     1,
		ProxiedRequestCount:              1,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

func TestTwoRequestsOneProxied(t *testing.T) {
	now := time.Now()
	s := newTestStats(now)
	s.proxiedStart(now)
	now = now.Add(500 * time.Millisecond)
	s.proxiedEnd(now)
	s.requestStart(now)
	now = now.Add(500 * time.Millisecond)
	got := s.report(now)
	want := &autoscaler.Stat{
		Time:                             &now,
		PodName:                          podName,
		AverageConcurrentRequests:        1.0,
		AverageProxiedConcurrentRequests: 0.5,
		RequestCount:                     2,
		ProxiedRequestCount:              1,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Unexpected stat (-want +got): %v", diff)
	}
}

// Test type to hold the bi-directional time channels
type testStats struct {
	Stats
	reportBiChan chan time.Time
}

func newTestStats(now time.Time) *testStats {
	reportBiChan := make(chan time.Time)
	ch := Channels{
		ReqChan:    make(chan ReqEvent),
		ReportChan: (<-chan time.Time)(reportBiChan),
		StatChan:   make(chan *autoscaler.Stat),
	}
	s := NewStats(podName, ch, now)
	t := &testStats{
		Stats:        *s,
		reportBiChan: reportBiChan,
	}
	return t
}

func (s *testStats) requestStart(now time.Time) {
	s.ch.ReqChan <- ReqEvent{Time: now, EventType: ReqIn}
}

func (s *testStats) requestEnd(now time.Time) {
	s.ch.ReqChan <- ReqEvent{Time: now, EventType: ReqOut}
}

func (s *testStats) proxiedStart(now time.Time) {
	s.ch.ReqChan <- ReqEvent{Time: now, EventType: ProxiedIn}
}

func (s *testStats) proxiedEnd(now time.Time) {
	s.ch.ReqChan <- ReqEvent{Time: now, EventType: ProxiedOut}
}

func (s *testStats) report(now time.Time) *autoscaler.Stat {
	s.reportBiChan <- now
	return <-s.ch.StatChan
}
