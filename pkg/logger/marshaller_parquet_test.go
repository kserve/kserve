/*
Copyright 2025 The KServe Authors.

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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/parquet-go/parquet-go"
)

func TestParquetMarshallerHandlerSingleRecord(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewParquetMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create a single LogRequest
	logReq := LogRequest{
		Url:              mustParseURL("http://example.com/predict"),
		Bytes:            ptrToBytes([]byte(`{"input": "test"}`)),
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

	// Verify response headers
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/octet-stream"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("parquet"))

	// Read response body
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())

	// Create Parquet reader
	reader := parquet.NewGenericReader[logRecord](bytes.NewReader(responseBody.Bytes()))
	defer reader.Close()

	// Verify NumRows
	g.Expect(reader.NumRows()).To(Equal(int64(1)))

	// Read the row
	rows := make([]logRecord, 1)
	n, err := reader.Read(rows)
	if err != nil && !errors.Is(err, io.EOF) {
		g.Expect(err).ToNot(HaveOccurred())
	}
	g.Expect(n).To(Equal(1))

	// Verify field values match toLogRecord()
	expectedRecord := toLogRecord(logReq)
	g.Expect(rows[0]).To(Equal(expectedRecord))
}

func TestParquetMarshallerHandlerMultipleRecords(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewParquetMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create multiple LogRequests
	logReq1 := LogRequest{
		Url:              mustParseURL("http://example.com/predict1"),
		Bytes:            ptrToBytes([]byte(`{"input": "test1"}`)),
		ContentType:      "application/json",
		ReqType:          "predict",
		Id:               "test-id-1",
		InferenceService: "service1",
		Namespace:        "default",
	}

	logReq2 := LogRequest{
		Url:              mustParseURL("http://example.com/predict2"),
		Bytes:            ptrToBytes([]byte(`{"input": "test2"}`)),
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

	// Verify response headers
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/octet-stream"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("parquet"))

	// Read response body
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())

	// Create Parquet reader
	reader := parquet.NewGenericReader[logRecord](bytes.NewReader(responseBody.Bytes()))
	defer reader.Close()

	// Verify NumRows
	g.Expect(reader.NumRows()).To(Equal(int64(2)))

	// Read both rows
	rows := make([]logRecord, 2)
	n, err := reader.Read(rows)
	if err != nil && !errors.Is(err, io.EOF) {
		g.Expect(err).ToNot(HaveOccurred())
	}
	g.Expect(n).To(Equal(2))

	// Verify both rows
	expectedRecord1 := toLogRecord(logReq1)
	expectedRecord2 := toLogRecord(logReq2)
	g.Expect(rows[0]).To(Equal(expectedRecord1))
	g.Expect(rows[1]).To(Equal(expectedRecord2))
}

func TestParquetMarshallerHandlerVaryingBatchSizes(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewParquetMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Property-based: test with different batch sizes
	batchSizes := []int{1, 2, 5, 10, 25}

	for _, size := range batchSizes {
		batch := make([]LogRequest, size)
		for i := range size {
			batch[i] = LogRequest{
				Url:              mustParseURL("http://example.com/predict"),
				Bytes:            ptrToBytes([]byte(`{"input": "test"}`)),
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
		g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/octet-stream"))
		g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("parquet"))

		// Read response body
		var responseBody bytes.Buffer
		_, err = responseBody.ReadFrom(resp.Body)
		g.Expect(err).ToNot(HaveOccurred())

		// Create Parquet reader
		reader := parquet.NewGenericReader[logRecord](bytes.NewReader(responseBody.Bytes()))
		defer reader.Close()

		// Verify NumRows matches batch size
		g.Expect(reader.NumRows()).To(Equal(int64(size)))
	}
}

func TestParquetMarshallerHandlerEmptyBatch(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewParquetMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Send empty batch
	emptyBatch := []LogRequest{}
	batchJSON, err := json.Marshal(emptyBatch)
	g.Expect(err).ToNot(HaveOccurred())

	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	// Verify response headers
	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/octet-stream"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("parquet"))

	// Read response body
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())

	// Create Parquet reader
	reader := parquet.NewGenericReader[logRecord](bytes.NewReader(responseBody.Bytes()))
	defer reader.Close()

	// Verify valid Parquet with NumRows == 0
	g.Expect(reader.NumRows()).To(Equal(int64(0)))
}

func TestParquetMarshallerHandlerMethodNotAllowed(t *testing.T) {
	handler := NewParquetMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	testCases := []struct {
		name   string
		method string
	}{
		{name: "GET", method: http.MethodGet},
		{name: "PUT", method: http.MethodPut},
		{name: "DELETE", method: http.MethodDelete},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			req, err := http.NewRequest(tc.method, server.URL+"/marshal", nil)
			g.Expect(err).ToNot(HaveOccurred())

			resp, err := http.DefaultClient.Do(req)
			g.Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Verify 405 Method Not Allowed
			g.Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
		})
	}
}

func TestParquetMarshallerHandlerInvalidJSON(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewParquetMarshallerHandler()
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

func TestParquetMarshallerHandlerVaryingRecordContent(t *testing.T) {
	handler := NewParquetMarshallerHandler()
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
			g.Expect(resp.Header.Get("Content-Type")).To(Equal("application/octet-stream"))
			g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("parquet"))

			// Read response body
			var responseBody bytes.Buffer
			_, err = responseBody.ReadFrom(resp.Body)
			g.Expect(err).ToNot(HaveOccurred())

			// Create Parquet reader
			reader := parquet.NewGenericReader[logRecord](bytes.NewReader(responseBody.Bytes()))
			defer reader.Close()

			// Verify 1 row
			g.Expect(reader.NumRows()).To(Equal(int64(1)))

			// Read and verify round-trip via toLogRecord()
			rows := make([]logRecord, 1)
			n, err := reader.Read(rows)
			if err != nil && !errors.Is(err, io.EOF) {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(n).To(Equal(1))

			expectedRecord := toLogRecord(tc.logRequest)
			g.Expect(rows[0]).To(Equal(expectedRecord))
		})
	}
}

func TestParquetMarshallerHandlerSchemaColumns(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewParquetMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create minimal batch
	batch := []LogRequest{{Id: "test"}}
	batchJSON, err := json.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())

	// Send POST request
	resp, err := http.Post(server.URL+"/marshal", "application/json", bytes.NewReader(batchJSON))
	g.Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()

	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))

	// Read response body
	var responseBody bytes.Buffer
	_, err = responseBody.ReadFrom(resp.Body)
	g.Expect(err).ToNot(HaveOccurred())

	// Create Parquet reader
	reader := parquet.NewGenericReader[logRecord](bytes.NewReader(responseBody.Bytes()))
	defer reader.Close()

	// Get schema
	schema := reader.Schema()

	// Verify column names match logRecordColumns()
	expectedColumns := logRecordColumns()
	fields := schema.Fields()
	g.Expect(fields).To(HaveLen(len(expectedColumns)))

	for i, expectedCol := range expectedColumns {
		g.Expect(fields[i].Name()).To(Equal(expectedCol))
	}
}
