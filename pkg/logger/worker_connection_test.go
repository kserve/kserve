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
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"go.uber.org/zap/zaptest"
)

// newCountingServer counts every transition into http.StateNew, i.e. every
// new TCP connection the server observes.
func newCountingServer(handler http.HandlerFunc) (*httptest.Server, *int64) {
	var newConns int64
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			atomic.AddInt64(&newConns, 1)
		}
	}
	srv.Start()
	return srv, &newConns
}

// TestWorkerHTTPConnectionReuse drives events through N long-lived Worker
// instances -- matching how StartDispatcher actually runs the worker pool,
// one goroutine per Worker processing its Work channel serially -- and
// counts how many new TCP connections the destination server sees.
//
// Before caching the client, sendHttpCloudEvent rebuilds a cloudevents.Client
// (and its http.Client/Transport) on every call, so this fails with roughly
// one new connection per event. After caching it, each worker opens one
// connection and reuses it for every subsequent event.
func TestWorkerHTTPConnectionReuse(t *testing.T) {
	for _, concurrency := range []int{5, 25} {
		t.Run(fmt.Sprintf("workers=%d", concurrency), func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)

			const eventsPerWorker = 20
			totalEvents := concurrency * eventsPerWorker

			srv, newConns := newCountingServer(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			defer srv.Close()

			target, err := url.Parse(srv.URL)
			g.Expect(err).ToNot(gomega.HaveOccurred())

			// Worker goroutines must not call g.Expect: a failed assertion
			// would FailNow from a goroutine other than the test's. Each one
			// records its first error instead, asserted below.
			sendErrs := make([]error, concurrency)
			var wg sync.WaitGroup
			for i := range concurrency {
				worker := NewWorker(0, nil, zaptest.NewLogger(t).Sugar())
				wg.Go(func() {
					for range eventsPerWorker {
						body := []byte("{}")
						req := LogRequest{
							Url:            target,
							Bytes:          &body,
							ContentType:    "application/json",
							ReqType:        CEInferenceRequest,
							Id:             "test-id",
							SourceUri:      target,
							OccurrenceTime: time.Now(),
						}
						if err := worker.sendHttpCloudEvent(req); err != nil {
							sendErrs[i] = err
							return
						}
					}
				})
			}
			wg.Wait()
			g.Expect(errors.Join(sendErrs...)).To(gomega.Succeed())

			observed := atomic.LoadInt64(newConns)
			t.Logf("concurrency=%d events=%d new_connections=%d", concurrency, totalEvents, observed)

			// One connection per long-lived worker, not per event.
			g.Expect(observed).To(gomega.BeNumerically("<=", int64(concurrency)+5))
		})
	}
}

// TestWorkerHTTPClientTimeoutFreesStuckWorker checks that the configured
// timeout reaches the HTTP client: a worker whose event hits an endpoint that
// never answers must give up after the timeout and deliver its next event,
// instead of being stranded.
func TestWorkerHTTPClientTimeoutFreesStuckWorker(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	const clientTimeout = 200 * time.Millisecond
	// Longer than the assertion below but finite, so that if the timeout is
	// not applied the test fails instead of hanging.
	const stuckFor = 5 * time.Second

	var requests, delivered int64
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt64(&requests, 1) == 1 {
			select {
			case <-release:
			case <-time.After(stuckFor):
			}
			return
		}
		atomic.AddInt64(&delivered, 1)
		w.WriteHeader(http.StatusOK)
	}))
	// srv.Close waits for in-flight handlers, so release must be closed first
	// (deferred calls run last-in first-out).
	defer srv.Close()
	defer close(release)

	target, err := url.Parse(srv.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	body := []byte("{}")
	req := LogRequest{
		Url:            target,
		Bytes:          &body,
		ContentType:    "application/json",
		ReqType:        CEInferenceRequest,
		Id:             "test-id",
		SourceUri:      target,
		OccurrenceTime: time.Now(),
	}
	worker := NewWorker(0, nil, zaptest.NewLogger(t).Sugar(), WithHTTPClientTimeout(clientTimeout))

	start := time.Now()
	_ = worker.sendHttpCloudEvent(req) // the stuck endpoint; its outcome is not what is under test
	g.Expect(time.Since(start)).To(gomega.BeNumerically("<", stuckFor/2),
		"the stuck request should be cut short by the client timeout")

	g.Expect(worker.sendHttpCloudEvent(req)).To(gomega.Succeed())
	g.Expect(atomic.LoadInt64(&delivered)).To(gomega.BeEquivalentTo(1),
		"the same worker should deliver its next event after the timeout")
}
