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

package logger

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	pkglogging "knative.dev/pkg/logging"
)

// TestQueueLogRequestDropsWhenFull verifies that QueueLogRequest never blocks
// the caller (the inference request path): once WorkQueue is full it must
// reject the request immediately instead of waiting for room.
func TestQueueLogRequestDropsWhenFull(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	original := WorkQueue
	defer func() { WorkQueue = original }()
	WorkQueue = make(chan LogRequest, 2)

	body := []byte("payload")
	req := LogRequest{Id: "req", Bytes: &body}

	g.Expect(QueueLogRequest(req)).To(gomega.Succeed())
	g.Expect(QueueLogRequest(req)).To(gomega.Succeed())

	done := make(chan error, 1)
	go func() { done <- QueueLogRequest(req) }()

	select {
	case err := <-done:
		g.Expect(err).To(gomega.HaveOccurred())
	case <-time.After(time.Second):
		t.Fatal("QueueLogRequest blocked instead of dropping the request once the queue was full")
	}
}

// TestDispatcherDoesNotLeakGoroutinesWhenWorkersAreBusy reproduces the
// scenario in which a slow/stuck log endpoint causes every worker to stay
// busy indefinitely. The dispatcher must not spawn a new goroutine per
// queued event while waiting for a worker to free up - doing so grows
// memory without bound until the process is OOM-killed.
func TestDispatcherDoesNotLeakGoroutinesWhenWorkersAreBusy(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	logger, _ := pkglogging.NewLogger("", "INFO")

	block := make(chan struct{})
	stuckSvc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		<-block // simulate a downstream log endpoint that never drains
	}))
	// stuckSvc.Close() waits for in-flight connections to finish, so block
	// must be closed first to unstick the handler above.
	defer stuckSvc.Close()
	defer close(block)

	logUrl, err := url.Parse(stuckSvc.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// A single worker so it's immediately saturated by the first event.
	StartDispatcher(1, LoggerWorkerQueueSize, &MockStore{}, &ImmediateBatch{}, logger)

	// Let the dispatcher/worker goroutines settle before measuring the baseline.
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	const numRequests = 50
	body := []byte("payload")
	for i := range numRequests {
		g.Expect(QueueLogRequest(LogRequest{
			Url:            logUrl,
			SourceUri:      logUrl,
			Bytes:          &body,
			Id:             fmt.Sprintf("req-%d", i),
			ReqType:        CEInferenceRequest,
			OccurrenceTime: time.Now(),
		})).To(gomega.Succeed())
	}

	// Give any (buggy) per-request goroutines time to spawn.
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()

	g.Expect(after-before).To(gomega.BeNumerically("<", numRequests/2),
		"expected dispatcher to bound goroutine growth while workers are busy, got %d new goroutines for %d requests", after-before, numRequests)
}

// TestStartDispatcherDoesNotRaceOnRepeatedCalls reproduces the race between a
// dispatcher goroutine from one StartDispatcher call still reading
// WorkQueue/WorkerQueue and a later call reassigning those package vars
// (e.g. a second call in the same process, as happens across tests). It
// keeps the first call's only worker busy on a stuck request so the
// dispatcher goroutine, which blocks on the worker queue for a second,
// unrelated event, is mid-read when a second dispatcher is started.
func TestStartDispatcherDoesNotRaceOnRepeatedCalls(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	logger, _ := pkglogging.NewLogger("", "INFO")

	workerBusy := make(chan struct{})
	block := make(chan struct{})
	var busyOnce sync.Once
	stuckSvc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		busyOnce.Do(func() { close(workerBusy) })
		<-block // never respond, so the only worker stays busy
	}))
	defer stuckSvc.Close()
	defer close(block)

	logUrl, err := url.Parse(stuckSvc.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	body := []byte("payload")
	newRequest := func(id string) LogRequest {
		return LogRequest{
			Url:            logUrl,
			SourceUri:      logUrl,
			Bytes:          &body,
			Id:             id,
			ReqType:        CEInferenceRequest,
			OccurrenceTime: time.Now(),
		}
	}

	StartDispatcher(1, LoggerWorkerQueueSize, &MockStore{}, &ImmediateBatch{}, logger)
	firstQueue := WorkQueue
	g.Expect(QueueLogRequest(newRequest("req-busy"))).To(gomega.Succeed())
	// The only worker is now stuck on the first request.
	g.Eventually(workerBusy).Should(gomega.BeClosed())

	g.Expect(QueueLogRequest(newRequest("req-blocked"))).To(gomega.Succeed())
	// The dispatcher has taken the second event off the queue and is now
	// waiting on the worker queue for a free worker.
	g.Eventually(func() int { return len(firstQueue) }).Should(gomega.BeZero())

	// This reassigns the package-level WorkQueue/WorkerQueue vars while the
	// previous call's dispatcher goroutine may still be reading them.
	StartDispatcher(1, LoggerWorkerQueueSize, &MockStore{}, &ImmediateBatch{}, logger)
	g.Expect(QueueLogRequest(newRequest("req-2"))).To(gomega.Succeed())
	time.Sleep(20 * time.Millisecond)
}
