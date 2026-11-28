/*
Copyright 2021 The KServe Authors.

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

package batcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/onsi/gomega"
	pkglogging "knative.dev/pkg/logging"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	kfslogger "github.com/kserve/kserve/pkg/logger"
)

func serveRequest(batchHandler *BatchHandler, wg *sync.WaitGroup, index int) {
	defer wg.Done()
	instances := fmt.Sprintf("{\"instances\": [[%d, %d, %d]]}", index, index, index)
	predictorRequest := []byte(instances)
	reader := bytes.NewReader(predictorRequest)
	path := "/v1/models/test:predict"
	r := httptest.NewRequest(http.MethodPost, path, reader)
	w := httptest.NewRecorder()
	batchHandler.ServeHTTP(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	b2, _ := io.ReadAll(resp.Body)
	var res Response
	_ = json.Unmarshal(b2, &res)
}

func TestBatcher(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	logger, _ := pkglogging.NewLogger("", "INFO")

	responseChan := make(chan Response)
	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		var request Request
		err = json.Unmarshal(b, &request)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		logger.Infof("Get request %v", string(b))
		response := Response{
			Predictions: request.Instances,
		}
		responseChan <- response
		responseBytes, err := json.Marshal(response)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		_, err = rw.Write(responseBytes)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}))
	// Close the server when test finishes
	defer predictor.Close()
	predictorSvcUrl, err := url.Parse(predictor.URL)
	logger.Infof("predictor url %s", predictorSvcUrl)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	httpProxy := httputil.NewSingleHostReverseProxy(predictorSvcUrl)
	batchHandler := New(32, 50, httpProxy, logger)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go serveRequest(batchHandler, &wg, i)
	}
	// var responseBytes []byte
	<-responseChan
	wg.Wait()
}

// Tests batcher when inference response code is other than 200
func TestBatcherFail(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	logger, _ := pkglogging.NewLogger("", "INFO")

	responseChan := make(chan Response)
	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		var request Request
		err = json.Unmarshal(b, &request)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		logger.Infof("Get request %v", string(b))
		response := Response{}
		responseChan <- response
		responseBytes, err := json.Marshal(response)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		rw.WriteHeader(http.StatusInternalServerError)
		_, err = rw.Write(responseBytes)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}))
	// Close the server when test finishes
	defer predictor.Close()
	predictorSvcUrl, err := url.Parse(predictor.URL)
	logger.Infof("predictor url %s", predictorSvcUrl)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	httpProxy := httputil.NewSingleHostReverseProxy(predictorSvcUrl)
	batchHandler := New(32, 50, httpProxy, logger)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go serveRequest(batchHandler, &wg, i)
	}
	// var responseBytes []byte
	<-responseChan
	wg.Wait()
}

// Tests default max batch size and max latency
func TestBatcherDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	logger, _ := pkglogging.NewLogger("", "INFO")

	responseChan := make(chan Response)
	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		var request Request
		err = json.Unmarshal(b, &request)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		logger.Infof("Get request %v", string(b))
		response := Response{
			Predictions: request.Instances,
		}
		responseChan <- response
		responseBytes, err := json.Marshal(response)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		rw.WriteHeader(http.StatusInternalServerError)
		_, err = rw.Write(responseBytes)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}))
	// Close the server when test finishes
	defer predictor.Close()
	predictorSvcUrl, err := url.Parse(predictor.URL)
	logger.Infof("predictor url %s", predictorSvcUrl)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	httpProxy := httputil.NewSingleHostReverseProxy(predictorSvcUrl)
	batchHandler := New(-1, -1, httpProxy, logger)
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go serveRequest(batchHandler, &wg, i)
	}
	// var responseBytes []byte
	<-responseChan
	wg.Wait()
	g.Expect(batchHandler.MaxBatchSize).To(gomega.Equal(MaxBatchSize))
	g.Expect(batchHandler.MaxLatency).To(gomega.Equal(MaxLatency))
}

// Tests that every client of a batch gets the status of the batch's outcome
func TestBatcherResponseStatus(t *testing.T) {
	testCases := map[string]struct {
		predictorStatus     int
		predictorBody       string
		expectedStatus      int
		expectedMessage     gomega.OmegaMatcher
		expectedPredictions gomega.OmegaMatcher
	}{
		"predictor success": {
			predictorStatus:     http.StatusOK,
			predictorBody:       `{"predictions": [1, 2]}`,
			expectedStatus:      http.StatusOK,
			expectedMessage:     gomega.BeEmpty(),
			expectedPredictions: gomega.HaveLen(1),
		},
		"predictor server error": {
			predictorStatus:     http.StatusServiceUnavailable,
			predictorBody:       "no healthy upstream",
			expectedStatus:      http.StatusServiceUnavailable,
			expectedMessage:     gomega.Equal("no healthy upstream"),
			expectedPredictions: gomega.BeEmpty(),
		},
		"predictor rejects the batch": {
			predictorStatus:     http.StatusBadRequest,
			predictorBody:       "invalid instances",
			expectedStatus:      http.StatusBadRequest,
			expectedMessage:     gomega.Equal("invalid instances"),
			expectedPredictions: gomega.BeEmpty(),
		},
		"predictor response is not valid JSON": {
			predictorStatus:     http.StatusOK,
			predictorBody:       "not json",
			expectedStatus:      http.StatusInternalServerError,
			expectedMessage:     gomega.ContainSubstring("invalid character"),
			expectedPredictions: gomega.BeEmpty(),
		},
		"predictor returns fewer predictions than instances": {
			predictorStatus:     http.StatusOK,
			predictorBody:       `{"predictions": [1]}`,
			expectedStatus:      http.StatusInternalServerError,
			expectedMessage:     gomega.Equal("size of prediction is not equal to the size of instances"),
			expectedPredictions: gomega.BeEmpty(),
		},
	}

	logger, _ := pkglogging.NewLogger("", "INFO")
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)

			predictor := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				rw.WriteHeader(tc.predictorStatus)
				_, _ = rw.Write([]byte(tc.predictorBody))
			})
			batchHandler := New(2, 5000, predictor, logger)

			// A batch of two requests, so that it is flushed by size.
			recorders := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
			var wg sync.WaitGroup
			for _, w := range recorders {
				wg.Go(func() {
					r := httptest.NewRequest(http.MethodPost, "/v1/models/test:predict", bytes.NewReader([]byte(`{"instances": [[1]]}`)))
					batchHandler.ServeHTTP(w, r)
				})
			}
			wg.Wait()

			for _, w := range recorders {
				g.Expect(w.Code).To(gomega.Equal(tc.expectedStatus))
				var res Response
				g.Expect(json.Unmarshal(w.Body.Bytes(), &res)).To(gomega.Succeed())
				g.Expect(res.Message).To(tc.expectedMessage)
				g.Expect(res.Predictions).To(tc.expectedPredictions)
			}
		})
	}
}

// Tests that the inference logger, which wraps the batcher in the agent, logs batched responses
func TestBatcherResponseIsLogged(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	logger, _ := pkglogging.NewLogger("", "INFO")
	predictor := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte(`{"predictions": [1]}`))
	})
	logUrl, err := url.Parse("http://logger.example.com")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	handler := kfslogger.New(logUrl, logUrl, v1beta1.LogResponse, "isvc", "default", "default", "predictor",
		New(1, 5000, predictor, logger), nil, "", nil, false)

	r := httptest.NewRequest(http.MethodPost, "/v1/models/test:predict", bytes.NewReader([]byte(`{"instances": [[1]]}`)))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	g.Expect(w.Code).To(gomega.Equal(http.StatusOK))
	var logged kfslogger.LogRequest
	g.Eventually(kfslogger.WorkQueue, time.Second).Should(gomega.Receive(&logged))
	g.Expect(logged.ReqType).To(gomega.Equal(kfslogger.CEInferenceResponse))
}
