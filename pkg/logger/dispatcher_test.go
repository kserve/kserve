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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/onsi/gomega"
	pkglogging "knative.dev/pkg/logging"
)

// TestStartDispatcherDoesNotRaceOnRepeatedCalls reproduces the race between a
// dispatcher goroutine from one StartDispatcher call still reading
// WorkQueue/WorkerQueue and a later call reassigning those package vars
// (e.g. a second call in the same process, as happens across tests). It
// keeps the first call's only worker busy on a stuck request so the
// dispatch goroutine for a second, unrelated event is left blocked on
// <-WorkerQueue, then starts a second dispatcher while that read is still
// in flight.
func TestStartDispatcherDoesNotRaceOnRepeatedCalls(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	logger, _ := pkglogging.NewLogger("", "INFO")

	block := make(chan struct{})
	stuckSvc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
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

	StartDispatcher(1, &MockStore{}, &ImmediateBatch{}, logger)
	g.Expect(QueueLogRequest(newRequest("req-busy"))).To(gomega.Succeed())
	// Let the single worker pick up the stuck request and a second event's
	// dispatch goroutine start blocking on <-WorkerQueue waiting for it.
	time.Sleep(20 * time.Millisecond)
	g.Expect(QueueLogRequest(newRequest("req-blocked"))).To(gomega.Succeed())
	time.Sleep(20 * time.Millisecond)

	// This reassigns the package-level WorkQueue/WorkerQueue vars while the
	// previous call's dispatch goroutine may still be reading them.
	StartDispatcher(1, &MockStore{}, &ImmediateBatch{}, logger)
	g.Expect(QueueLogRequest(newRequest("req-2"))).To(gomega.Succeed())
	time.Sleep(20 * time.Millisecond)
}
