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
	"github.com/onsi/gomega"
	"io/ioutil"
	pkglogging "knative.dev/pkg/logging"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
)

func serveRequest(batchHandler *BatchHandler, wg *sync.WaitGroup, index int) {
	defer wg.Done()
	instances := fmt.Sprintf("{\"instances\": [[%d, %d, %d]]}", index, index, index)
	predictorRequest := []byte(instances)
	reader := bytes.NewReader(predictorRequest)
	path := "/v1/models/test:predict"
	r := httptest.NewRequest("POST", path, reader)
	w := httptest.NewRecorder()
	batchHandler.ServeHTTP(w, r)

	b2, _ := ioutil.ReadAll(w.Result().Body)
	var res Response
	_ = json.Unmarshal(b2, &res)
	fmt.Printf("Got response %v\n", res)
}

func TestBatcher(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	logger, _ := pkglogging.NewLogger("", "INFO")

	responseChan := make(chan Response)
	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := ioutil.ReadAll(req.Body)
		g.Expect(err).To(gomega.BeNil())
		var request Request
		err = json.Unmarshal(b, &request)
		g.Expect(err).To(gomega.BeNil())
		logger.Infof("Get request %v", string(b))
		response := Response{
			Predictions: request.Instances,
		}
		responseChan <- response
		responseBytes, err := json.Marshal(response)
		g.Expect(err).To(gomega.BeNil())
		_, err = rw.Write(responseBytes)
		g.Expect(err).To(gomega.BeNil())
	}))
	// Close the server when test finishes
	defer predictor.Close()
	predictorSvcUrl, err := url.Parse(predictor.URL)
	logger.Infof("predictor url %s", predictorSvcUrl)
	g.Expect(err).To(gomega.BeNil())
	httpProxy := httputil.NewSingleHostReverseProxy(predictorSvcUrl)
	batchHandler := New(32, 50, httpProxy, logger)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go serveRequest(batchHandler, &wg, i)
	}
	//var responseBytes []byte
	<-responseChan
	wg.Wait()
}
