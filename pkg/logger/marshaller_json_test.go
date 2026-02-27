/*
Copyright 2024 The KServe Authors.

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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	. "github.com/onsi/gomega"
)

func TestJSONMarshallerHandlerSingleRecord(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create a single LogRequest
	testURL, _ := url.Parse("http://example.com/predict")
	testBytes := []byte(`{"input": "test"}`)
	logReq := LogRequest{
		Url:              testURL,
		Bytes:            &testBytes,
		ContentType:      "application/json",
		ReqType:          "predict",
		Id:               "test-id-123",
		InferenceService: "test-service",
		Namespace:        "default",
		Component:        "predictor",
		Endpoint:         "/v1/predict",
		Metadata:         map[string][]string{"header1": {"value1"}},
		Annotations:      map[string]string{"key1": "value1"},
	}

	// Create batch with single record
	batch := []LogRequest{logReq}
	batchJSON, err := json.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())

	// Send POST request
	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify response
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("json"))

	// Verify response body is single marshalled object, NOT array
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())

	// Expected output is json.Marshal(logReq), not json.Marshal([]LogRequest{logReq})
	expectedJSON, err := json.Marshal(logReq)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(responseBody.Bytes()).To(MatchJSON(expectedJSON))
}

func TestJSONMarshallerHandlerMultipleRecords(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create multiple LogRequests
	testURL1, _ := url.Parse("http://example.com/predict1")
	testBytes1 := []byte(`{"input": "test1"}`)
	logReq1 := LogRequest{
		Url:              testURL1,
		Bytes:            &testBytes1,
		ContentType:      "application/json",
		ReqType:          "predict",
		Id:               "test-id-1",
		InferenceService: "service1",
		Namespace:        "default",
	}

	testURL2, _ := url.Parse("http://example.com/predict2")
	testBytes2 := []byte(`{"input": "test2"}`)
	logReq2 := LogRequest{
		Url:              testURL2,
		Bytes:            &testBytes2,
		ContentType:      "application/json",
		ReqType:          "explain",
		Id:               "test-id-2",
		InferenceService: "service2",
		Namespace:        "production",
	}

	// Create batch with multiple records
	batch := []LogRequest{logReq1, logReq2}
	batchJSON, err := json.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())

	// Send POST request
	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify response
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("json"))

	// Verify response body is array of marshalled objects
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())

	expectedJSON, err := json.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(responseBody.Bytes()).To(MatchJSON(expectedJSON))
}

func TestJSONMarshallerHandlerVaryingBatchSizes(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Property-based: test with different batch sizes
	batchSizes := []int{1, 2, 5, 10, 25}

	for _, size := range batchSizes {
		batch := make([]LogRequest, size)
		for i := range size {
			testURL, _ := url.Parse("http://example.com/predict")
			testBytes := []byte(`{"input": "test"}`)
			batch[i] = LogRequest{
				Url:              testURL,
				Bytes:            &testBytes,
				ContentType:      "application/json",
				Id:               string(rune('a' + i)),
				InferenceService: "test-service",
				Namespace:        "default",
			}
		}

		batchJSON, err := json.Marshal(batch)
		g.Expect(err).ToNot(HaveOccurred())

		resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
		g.Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
		g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
		g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("json"))

		var responseBody bytes.Buffer
		_, err = responseBody.ReadFrom(resp.Body)
		g.Expect(err).ToNot(HaveOccurred())

		// For single record, expect single object; for multiple, expect array
		if size == 1 {
			expectedJSON, err := json.Marshal(batch[0])
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(responseBody.Bytes()).To(MatchJSON(expectedJSON))
		} else {
			expectedJSON, err := json.Marshal(batch)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(responseBody.Bytes()).To(MatchJSON(expectedJSON))
		}
	}
}

func TestJSONMarshallerHandlerContentTypeHeader(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Minimal valid batch
	batch := []LogRequest{{Id: "test"}}
	batchJSON, err := json.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())

	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify Content-Type header
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
}

func TestJSONMarshallerHandlerExtensionHeader(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Minimal valid batch
	batch := []LogRequest{{Id: "test"}}
	batchJSON, err := json.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())

	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify X-Log-Marshal-Extension header
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("json"))
}

func TestJSONMarshallerHandlerMethodNotAllowedGET(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send GET request
	resp, err := http.Get(server.URL + "/marshal")
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify 405 Method Not Allowed
	g.Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
}

func TestJSONMarshallerHandlerMethodNotAllowedPUT(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send PUT request
	req, err := http.NewRequest(http.MethodPut, server.URL+"/marshal", nil)
	g.Expect(err).ToNot(HaveOccurred())

	resp, err := http.DefaultClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify 405 Method Not Allowed
	g.Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
}

func TestJSONMarshallerHandlerMethodNotAllowedDELETE(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send DELETE request
	req, err := http.NewRequest(http.MethodDelete, server.URL+"/marshal", nil)
	g.Expect(err).ToNot(HaveOccurred())

	resp, err := http.DefaultClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify 405 Method Not Allowed
	g.Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
}

func TestJSONMarshallerHandlerInvalidJSON(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send invalid JSON
	invalidJSON := []byte(`{invalid json}`)
	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(invalidJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify 400 Bad Request
	g.Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}

func TestJSONMarshallerHandlerEmptyBatch(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send empty batch
	emptyBatch := []LogRequest{}
	batchJSON, err := json.Marshal(emptyBatch)
	g.Expect(err).ToNot(HaveOccurred())

	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Empty batch should return empty JSON array
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("json"))

	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())

	expectedJSON, err := json.Marshal(emptyBatch)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(responseBody.Bytes()).To(MatchJSON(expectedJSON))
}

func TestJSONMarshallerHandlerVaryingRecordContent(t *testing.T) {
	handler := NewJSONMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Property-based: test with different record content configurations
	testCases := []struct {
		name       string
		logRequest LogRequest
	}{
		{
			name: "minimal fields",
			logRequest: LogRequest{
				Id: "minimal",
			},
		},
		{
			name: "with URL and bytes",
			logRequest: LogRequest{
				Url:   mustParseURL("http://example.com"),
				Bytes: ptrToBytes([]byte(`{"key": "value"}`)),
				Id:    "with-url-bytes",
			},
		},
		{
			name: "with metadata and annotations",
			logRequest: LogRequest{
				Id:          "with-meta",
				Metadata:    map[string][]string{"h1": {"v1", "v2"}},
				Annotations: map[string]string{"a1": "av1"},
			},
		},
		{
			name: "with TLS settings",
			logRequest: LogRequest{
				Id:            "with-tls",
				CertName:      "my-cert",
				TlsSkipVerify: true,
			},
		},
		{
			name: "fully populated",
			logRequest: LogRequest{
				Url:              mustParseURL("http://example.com/predict"),
				Bytes:            ptrToBytes([]byte(`{"input": "data"}`)),
				ContentType:      "application/json",
				ReqType:          "predict",
				Id:               "full",
				SourceUri:        mustParseURL("http://source.com"),
				InferenceService: "my-service",
				Namespace:        "production",
				Component:        "predictor",
				Endpoint:         "/v1/models/my-model:predict",
				Metadata:         map[string][]string{"header": {"value"}},
				Annotations:      map[string]string{"key": "value"},
				CertName:         "cert",
				TlsSkipVerify:    false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			batch := []LogRequest{tc.logRequest}
			batchJSON, err := json.Marshal(batch)
			g.Expect(err).ToNot(HaveOccurred())

			resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
			g.Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			g.Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var responseBody bytes.Buffer
			_, err = responseBody.ReadFrom(resp.Body)
			g.Expect(err).ToNot(HaveOccurred())

			// Single record should be marshalled as object, not array
			expectedJSON, err := json.Marshal(tc.logRequest)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(responseBody.Bytes()).To(MatchJSON(expectedJSON))
		})
	}
}

// Helper functions
func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func ptrToBytes(b []byte) *[]byte {
	return &b
}
