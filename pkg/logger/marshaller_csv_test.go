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
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCSVMarshallerHandlerSingleRecord(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewCSVMarshallerHandler()
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
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("text/csv"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("csv"))

	// Parse CSV response
	csvReader := csv.NewReader(resp.Body)
	records, err := csvReader.ReadAll()
	g.Expect(err).ToNot(HaveOccurred())

	// Verify header row + 1 data row
	g.Expect(records).To(HaveLen(2))

	// Verify header matches logRecordColumns()
	expectedHeader := logRecordColumns()
	g.Expect(records[0]).To(Equal(expectedHeader))

	// Verify data row matches toLogRecord() output
	logRecord := toLogRecord(logReq)
	expectedData := logRecordToStrings(logRecord)
	g.Expect(records[1]).To(Equal(expectedData))
}

func TestCSVMarshallerHandlerMultipleRecords(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewCSVMarshallerHandler()
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
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("text/csv"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("csv"))

	// Parse CSV response
	csvReader := csv.NewReader(resp.Body)
	records, err := csvReader.ReadAll()
	g.Expect(err).ToNot(HaveOccurred())

	// Verify header + 2 data rows
	g.Expect(records).To(HaveLen(3))

	// Verify header
	expectedHeader := logRecordColumns()
	g.Expect(records[0]).To(Equal(expectedHeader))

	// Verify first data row
	logRecord1 := toLogRecord(logReq1)
	expectedData1 := logRecordToStrings(logRecord1)
	g.Expect(records[1]).To(Equal(expectedData1))

	// Verify second data row
	logRecord2 := toLogRecord(logReq2)
	expectedData2 := logRecordToStrings(logRecord2)
	g.Expect(records[2]).To(Equal(expectedData2))
}

func TestCSVMarshallerHandlerVaryingBatchSizes(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewCSVMarshallerHandler()
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
		g.Expect(resp.Header.Get("Content-Type")).To(Equal("text/csv"))
		g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("csv"))

		// Parse CSV response
		csvReader := csv.NewReader(resp.Body)
		records, err := csvReader.ReadAll()
		g.Expect(err).ToNot(HaveOccurred())

		// Verify row count = size + 1 (header)
		g.Expect(records).To(HaveLen(size + 1))

		// Verify header row
		expectedHeader := logRecordColumns()
		g.Expect(records[0]).To(Equal(expectedHeader))
	}
}

func TestCSVMarshallerHandlerEmptyBatch(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewCSVMarshallerHandler()
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
	g.Expect(resp.Header.Get("Content-Type")).To(Equal("text/csv"))
	g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("csv"))

	// Parse CSV response
	csvReader := csv.NewReader(resp.Body)
	records, err := csvReader.ReadAll()
	g.Expect(err).ToNot(HaveOccurred())

	// Empty batch should return header row only
	g.Expect(records).To(HaveLen(1))

	// Verify header
	expectedHeader := logRecordColumns()
	g.Expect(records[0]).To(Equal(expectedHeader))
}

func TestCSVMarshallerHandlerMethodNotAllowed(t *testing.T) {
	handler := NewCSVMarshallerHandler()
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

func TestCSVMarshallerHandlerInvalidJSON(t *testing.T) {
	g := NewGomegaWithT(t)

	handler := NewCSVMarshallerHandler()
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

func TestCSVMarshallerHandlerVaryingRecordContent(t *testing.T) {
	handler := NewCSVMarshallerHandler()
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
			g.Expect(resp.Header.Get("Content-Type")).To(Equal("text/csv"))
			g.Expect(resp.Header.Get("X-Log-Marshal-Extension")).To(Equal("csv"))

			// Parse CSV response
			csvReader := csv.NewReader(resp.Body)
			records, err := csvReader.ReadAll()
			g.Expect(err).ToNot(HaveOccurred())

			// Verify header + 1 data row
			g.Expect(records).To(HaveLen(2))

			// Verify round-trip correctness via toLogRecord()
			logRecord := toLogRecord(tc.logRequest)
			expectedData := logRecordToStrings(logRecord)
			g.Expect(records[1]).To(Equal(expectedData))
		})
	}
}

func TestCSVMarshallerHandlerSpecialCharacters(t *testing.T) {
	handler := NewCSVMarshallerHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test with metadata containing commas, quotes, and newlines in JSON values
	testCases := []struct {
		name       string
		logRequest LogRequest
	}{
		{
			name: "metadata with commas",
			logRequest: LogRequest{
				Id:       "comma-test",
				Metadata: map[string][]string{"header": {"value1,value2", "value3"}},
			},
		},
		{
			name: "metadata with quotes",
			logRequest: LogRequest{
				Id:       "quote-test",
				Metadata: map[string][]string{"header": {`value with "quotes"`}},
			},
		},
		{
			name: "metadata with newlines",
			logRequest: LogRequest{
				Id:       "newline-test",
				Metadata: map[string][]string{"header": {"value1\nvalue2"}},
			},
		},
		{
			name: "annotations with special chars",
			logRequest: LogRequest{
				Id:          "annotation-special",
				Annotations: map[string]string{"key1": `value with "quotes", commas, and newlines\n`},
			},
		},
		{
			name: "combined special characters",
			logRequest: LogRequest{
				Id:          "combined",
				Metadata:    map[string][]string{"h1": {`val1,"val2"`, "val3\nval4"}},
				Annotations: map[string]string{"a1": `{"nested": "json,with\"special"}`},
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

			// Parse CSV response
			csvReader := csv.NewReader(resp.Body)
			records, err := csvReader.ReadAll()
			g.Expect(err).ToNot(HaveOccurred())

			// Verify CSV round-trip: the CSV parser should correctly handle special characters
			g.Expect(records).To(HaveLen(2))

			// Verify data matches expected output from toLogRecord()
			logRecord := toLogRecord(tc.logRequest)
			expectedData := logRecordToStrings(logRecord)
			g.Expect(records[1]).To(Equal(expectedData))
		})
	}
}
