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

	"github.com/onsi/gomega"
	pkglogging "knative.dev/pkg/logging"
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

// Tests that requests for different models are never sent to the predictor in one batch
func TestBatcherMixedPaths(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	logger, _ := pkglogging.NewLogger("", "INFO")

	// The predictor answers every instance with the path it was called on.
	// Assertions must stay on the test goroutine, so the handler only records errors.
	var mu sync.Mutex
	var predictorErrs []error
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		var request Request
		err := json.NewDecoder(req.Body).Decode(&request)
		if err == nil {
			predictions := make([]interface{}, len(request.Instances))
			for i := range predictions {
				predictions[i] = req.URL.Path
			}
			err = json.NewEncoder(rw).Encode(PredictionResponse{Predictions: predictions})
		}
		if err != nil {
			mu.Lock()
			predictorErrs = append(predictorErrs, err)
			mu.Unlock()
			rw.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer predictor.Close()
	predictorSvcUrl, err := url.Parse(predictor.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	httpProxy := httputil.NewSingleHostReverseProxy(predictorSvcUrl)
	batchHandler := New(32, 500, httpProxy, logger)

	paths := []string{"/v1/models/model-a:predict", "/v1/models/model-b:predict"}
	bodies := make([][]byte, 4)
	var wg sync.WaitGroup
	for i := range bodies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reader := bytes.NewReader([]byte(`{"instances": [[1, 2], [3, 4]]}`))
			r := httptest.NewRequest(http.MethodPost, paths[i%2], reader)
			w := httptest.NewRecorder()
			batchHandler.ServeHTTP(w, r)
			bodies[i] = w.Body.Bytes()
		}(i)
	}
	wg.Wait()

	g.Expect(predictorErrs).To(gomega.BeEmpty())
	for i, body := range bodies {
		var res Response
		g.Expect(json.Unmarshal(body, &res)).To(gomega.Succeed())
		g.Expect(res.Message).To(gomega.BeEmpty())
		g.Expect(res.Predictions).To(gomega.Equal([]interface{}{paths[i%2], paths[i%2]}))
	}
}
