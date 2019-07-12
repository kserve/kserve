/*
Copyright 2019 The Knative Authors

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
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/knative/serving/pkg/queue/stats"
)

func TestNewRequestMetricHandler_failure(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var r stats.StatsReporter
	_, err := NewRequestMetricHandler(baseHandler, r)
	if err == nil {
		t.Error("should get error when StatsReporter is empty")
	}
}

func TestRequestMetricHandler(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r := &fakeStatsReporter{}
	handler, err := NewRequestMetricHandler(baseHandler, r)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewBufferString("test"))
	handler.ServeHTTP(resp, req)

	// Serve one request, should get 1 request count and none zero latency
	if got, want := r.lastRespCode, http.StatusOK; got != want {
		t.Errorf("response code got %v, want %v", got, want)
	}
	if got, want := r.lastReqCount, 1; got != int64(want) {
		t.Errorf("request count got %v, want %v", got, want)
	}
	if r.lastReqLatency == 0 {
		t.Errorf("request latency got %v, want larger than 0", r.lastReqLatency)
	}
}

func TestRequestMetricHandler_PanickingHandler(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("no!")
	})
	r := &fakeStatsReporter{}
	handler, err := NewRequestMetricHandler(baseHandler, r)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewBufferString("test"))
	defer func() {
		err := recover()
		if err == nil {
			t.Error("want ServeHTTP to panic, got nothing.")
		}

		// Serve one request, should get 1 request count and none zero latency
		if got, want := r.lastRespCode, http.StatusInternalServerError; got != want {
			t.Errorf("response code got %v, want %v", got, want)
		}
		if got, want := r.lastReqCount, 1; got != int64(want) {
			t.Errorf("request count got %v, want %v", got, want)
		}
		if r.lastReqLatency == 0 {
			t.Errorf("request latency got %v, want larger than 0", r.lastReqLatency)
		}
	}()
	handler.ServeHTTP(resp, req)

}

// fakeStatsReporter just record the last stat it received.
type fakeStatsReporter struct {
	lastRespCode   int
	lastReqCount   int64
	lastReqLatency time.Duration
}

func (r *fakeStatsReporter) ReportRequestCount(responseCode int, v int64) error {
	r.lastRespCode = responseCode
	r.lastReqCount = v
	return nil
}

func (r *fakeStatsReporter) ReportResponseTime(responseCode int, d time.Duration) error {
	r.lastRespCode = responseCode
	r.lastReqLatency = d
	return nil
}
