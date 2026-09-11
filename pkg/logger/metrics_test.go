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

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	pkglogging "knative.dev/pkg/logging"
)

func histogramSampleCount(t *testing.T, reqType string) uint64 {
	t.Helper()
	m := &dto.Metric{}
	if err := DeliveryDuration.WithLabelValues(reqType).(prometheus.Metric).Write(m); err != nil {
		t.Fatalf("failed to read DeliveryDuration: %v", err)
	}
	return m.GetHistogram().GetSampleCount()
}

func newTestWorker(t *testing.T) *Worker {
	logger, _ := pkglogging.NewLogger("", "INFO")
	return &Worker{Log: logger}
}

func newTestLogRequest(target string) LogRequest {
	logUrl, _ := url.Parse(target)
	body := []byte("payload")
	return LogRequest{
		Url:            logUrl,
		SourceUri:      logUrl,
		Bytes:          &body,
		Id:             "req-1",
		ReqType:        CEInferenceRequest,
		OccurrenceTime: time.Now(),
	}
}

func TestSendHttpCloudEventMetricsOnSuccess(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	svc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))
	defer svc.Close()

	sentBefore := testutil.ToFloat64(EventsSentTotal.WithLabelValues(CEInferenceRequest))
	failedBefore := testutil.ToFloat64(EventsFailedTotal.WithLabelValues(CEInferenceRequest))
	deliveryCountBefore := histogramSampleCount(t, CEInferenceRequest)

	w := newTestWorker(t)
	g.Expect(w.sendHttpCloudEvent(newTestLogRequest(svc.URL))).To(gomega.Succeed())

	g.Expect(testutil.ToFloat64(EventsSentTotal.WithLabelValues(CEInferenceRequest))).To(gomega.Equal(sentBefore + 1))
	g.Expect(testutil.ToFloat64(EventsFailedTotal.WithLabelValues(CEInferenceRequest))).To(gomega.Equal(failedBefore))
	g.Expect(histogramSampleCount(t, CEInferenceRequest)).To(gomega.Equal(deliveryCountBefore + 1))
}

func TestSendHttpCloudEventMetricsOnNonOKStatus(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	svc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	defer svc.Close()

	sentBefore := testutil.ToFloat64(EventsSentTotal.WithLabelValues(CEInferenceRequest))
	failedBefore := testutil.ToFloat64(EventsFailedTotal.WithLabelValues(CEInferenceRequest))

	w := newTestWorker(t)
	g.Expect(w.sendHttpCloudEvent(newTestLogRequest(svc.URL))).To(gomega.Succeed())

	g.Expect(testutil.ToFloat64(EventsSentTotal.WithLabelValues(CEInferenceRequest))).To(gomega.Equal(sentBefore))
	g.Expect(testutil.ToFloat64(EventsFailedTotal.WithLabelValues(CEInferenceRequest))).To(gomega.Equal(failedBefore + 1))
}

func TestSendHttpCloudEventMetricsOnConnectionFailure(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	svc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {}))
	svc.Close() // closed before use, so the send can never connect

	failedBefore := testutil.ToFloat64(EventsFailedTotal.WithLabelValues(CEInferenceRequest))

	w := newTestWorker(t)
	g.Expect(w.sendHttpCloudEvent(newTestLogRequest(svc.URL))).To(gomega.Succeed())

	g.Expect(testutil.ToFloat64(EventsFailedTotal.WithLabelValues(CEInferenceRequest))).To(gomega.Equal(failedBefore + 1))
}

func TestQueueLogRequestUpdatesWorkQueueDepth(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	original := WorkQueue
	defer func() { WorkQueue = original }()
	WorkQueue = make(chan LogRequest, 2)

	req := newTestLogRequest("http://127.0.0.1:0")

	g.Expect(QueueLogRequest(req)).To(gomega.Succeed())
	g.Expect(testutil.ToFloat64(WorkQueueDepth)).To(gomega.Equal(float64(1)))

	g.Expect(QueueLogRequest(req)).To(gomega.Succeed())
	g.Expect(testutil.ToFloat64(WorkQueueDepth)).To(gomega.Equal(float64(2)))
}
