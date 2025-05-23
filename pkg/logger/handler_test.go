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

package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/onsi/gomega"
	pkglogging "knative.dev/pkg/logging"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func TestLogger(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	predictorRequest := []byte(`{"instances":[[0,0,0]]}`)
	predictorResponse := []byte(`{"instances":[[4,5,6]]}`)

	responseChan := make(chan string)
	// Start a local HTTP server
	logSvc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		responseChan <- string(b)
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write([]byte(`ok`))
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}))
	// Close the server when test finishes
	defer logSvc.Close()

	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write(predictorResponse)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}))
	// Close the server when test finishes
	defer predictor.Close()

	reader := bytes.NewReader(predictorRequest)
	r := httptest.NewRequest(http.MethodPost, "http://a", reader)
	w := httptest.NewRecorder()
	logger, _ := pkglogging.NewLogger("", "INFO")
	logf.SetLogger(zap.New())
	logSvcUrl, err := url.Parse(logSvc.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	sourceUri, err := url.Parse("http://localhost:9081/")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	targetUri, err := url.Parse(predictor.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	StartDispatcher(5, logger)
	httpProxy := httputil.NewSingleHostReverseProxy(targetUri)
	oh := New(logSvcUrl, sourceUri, v1beta1.LogAll, "mymodel", "default", "default",
		"default", httpProxy, nil, "", nil, true)

	oh.ServeHTTP(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	b2, _ := io.ReadAll(resp.Body)
	g.Expect(b2).To(gomega.Equal(predictorResponse))
	// get logRequest
	<-responseChan
	// get logResponse
	<-responseChan
}

func TestLoggerWithMetadata(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	predictorRequest := []byte(`{"instances":[[0,0,0]]}`)
	predictorResponse := []byte(`{"instances":[[4,5,6]]}`)

	responseChan := make(chan string)
	// Start a local HTTP server
	logSvc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		responseChan <- string(b)
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write([]byte(`ok`))
		g.Expect(err).ToNot(gomega.HaveOccurred())

		// If this is request, check the metadata
		if req.Header["Ce-Type"][0] == "org.kubeflow.serving.inference.request" {
			metadata := map[string][]string{}

			json.Unmarshal([]byte(req.Header["Ce-Metadata"][0]), &metadata)

			g.Expect(metadata["Foo"]).To(gomega.Equal([]string{"bar"}))

			g.Expect(metadata["Fizz"]).To(gomega.Equal([]string{"buzz"}))
		}
	}))
	// Close the server when test finishes
	defer logSvc.Close()

	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write(predictorResponse)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}))
	// Close the server when test finishes
	defer predictor.Close()

	reader := bytes.NewReader(predictorRequest)
	r := httptest.NewRequest(http.MethodPost, "http://a", reader)
	r.Header.Add("Foo", "bar")
	r.Header.Add("Fizz", "buzz")
	w := httptest.NewRecorder()
	logger, _ := pkglogging.NewLogger("", "INFO")
	logf.SetLogger(zap.New())
	logSvcUrl, err := url.Parse(logSvc.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	sourceUri, err := url.Parse("http://localhost:9081/")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	targetUri, err := url.Parse(predictor.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	StartDispatcher(5, logger)
	httpProxy := httputil.NewSingleHostReverseProxy(targetUri)
	oh := New(logSvcUrl, sourceUri, v1beta1.LogAll, "mymodel", "default", "default",
		"default", httpProxy, []string{"Foo", "Fizz"}, "", nil, true)

	oh.ServeHTTP(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	b2, _ := io.ReadAll(resp.Body)
	g.Expect(b2).To(gomega.Equal(predictorResponse))
	// get logRequest
	<-responseChan
	// get logResponse
	<-responseChan
}

func TestLoggerWithAnnotation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	predictorRequest := []byte(`{"instances":[[0,0,0]]}`)
	predictorResponse := []byte(`{"instances":[[4,5,6]]}`)

	responseChan := make(chan string)
	// Start a local HTTP server
	logSvc := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		responseChan <- string(b)
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write([]byte(`ok`))
		g.Expect(err).ToNot(gomega.HaveOccurred())

		// If this is request, check the metadata
		if req.Header["Ce-Type"][0] == "org.kubeflow.serving.inference.request" {
			annotations := map[string]string{}

			json.Unmarshal([]byte(req.Header["Ce-Annotations"][0]), &annotations)

			g.Expect(annotations["Foo"]).To(gomega.Equal("Bar"))

			g.Expect(annotations["Fizz"]).To(gomega.Equal("Buzz"))
		}
	}))
	// Close the server when test finishes
	defer logSvc.Close()

	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(b).To(gomega.Or(gomega.Equal(predictorRequest), gomega.Equal(predictorResponse)))
		_, err = rw.Write(predictorResponse)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}))
	// Close the server when test finishes
	defer predictor.Close()

	reader := bytes.NewReader(predictorRequest)
	r := httptest.NewRequest(http.MethodPost, "http://a", reader)
	r.Header.Add("Foo", "bar")
	r.Header.Add("Fizz", "buzz")
	w := httptest.NewRecorder()
	logger, _ := pkglogging.NewLogger("", "INFO")
	logf.SetLogger(zap.New())
	logSvcUrl, err := url.Parse(logSvc.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	sourceUri, err := url.Parse("http://localhost:9081/")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	targetUri, err := url.Parse(predictor.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	StartDispatcher(5, logger)
	httpProxy := httputil.NewSingleHostReverseProxy(targetUri)
	oh := New(logSvcUrl, sourceUri, v1beta1.LogAll, "mymodel", "default", "default",
		"default", httpProxy, nil, "", map[string]string{"Foo": "Bar", "Fizz": "Buzz"}, true)

	oh.ServeHTTP(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	b2, _ := io.ReadAll(resp.Body)
	g.Expect(b2).To(gomega.Equal(predictorResponse))
	// get logRequest
	<-responseChan
	// get logResponse
	<-responseChan
}

func TestBadResponse(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	predictorRequest := []byte(`{"instances":[[0,0,0]]}`)
	predictorResponse := "BadRequest\n"

	// Start a local HTTP server
	predictor := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		b, err := io.ReadAll(req.Body)
		g.Expect(err).ToNot(gomega.HaveOccurred())
		g.Expect(b).To(gomega.Equal(predictorRequest))
		http.Error(rw, "BadRequest", http.StatusBadRequest)
	}))
	// Close the server when test finishes
	defer predictor.Close()

	reader := bytes.NewReader(predictorRequest)
	r := httptest.NewRequest(http.MethodPost, "http://a", reader)
	w := httptest.NewRecorder()
	logger, _ := pkglogging.NewLogger("", "INFO")
	logf.SetLogger(zap.New())
	logSvcUrl, err := url.Parse("http://loggersvc")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	sourceUri, err := url.Parse("http://localhost:9081/")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	targetUri, err := url.Parse(predictor.URL)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	StartDispatcher(1, logger)
	httpProxy := httputil.NewSingleHostReverseProxy(targetUri)
	oh := New(logSvcUrl, sourceUri, v1beta1.LogAll, "mymodel", "default", "default",
		"default", httpProxy, nil, "", nil, true)

	oh.ServeHTTP(w, r)
	g.Expect(w.Code).To(gomega.Equal(400))
	g.Expect(w.Body.String()).To(gomega.Equal(predictorResponse))
}
