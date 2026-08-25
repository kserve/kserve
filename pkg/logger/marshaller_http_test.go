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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

// createTestMarshallerHandler creates an inline test handler that mimics the JSON marshaller handler behavior.
// It accepts POST /marshal with []LogRequest JSON body and returns marshalled JSON with appropriate headers.
func createTestMarshallerHandler(extension string, statusCode int, slowResponse bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if slowResponse {
			time.Sleep(200 * time.Millisecond)
		}

		if statusCode != http.StatusOK {
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte("internal server error"))
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var batch []LogRequest
		if err := json.Unmarshal(body, &batch); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var responseData []byte
		if len(batch) == 1 {
			responseData, _ = json.Marshal(batch[0])
		} else {
			responseData, _ = json.Marshal(batch)
		}

		w.Header().Set("Content-Type", "application/json")
		if extension != "" {
			w.Header().Set("X-Log-Marshal-Extension", extension)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseData)
	}
}

func TestHTTPMarshallerSingleRecord(t *testing.T) {
	g := NewGomegaWithT(t)

	server := httptest.NewServer(createTestMarshallerHandler("json", http.StatusOK, false))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	marshaller := NewHTTPMarshaller(server.URL, client)

	batch := []LogRequest{
		{
			Id:               "test-id-1",
			InferenceService: "test-service",
			Namespace:        "default",
			Component:        "predictor",
			Endpoint:         "/v1/models/test:predict",
		},
	}

	response, err := marshaller.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response).ToNot(BeNil())
	g.Expect(response.Extension).To(Equal("json"))

	// Verify the response data matches expected JSON for single record
	var unmarshalled LogRequest
	err = json.Unmarshal(response.Data, &unmarshalled)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(unmarshalled.Id).To(Equal("test-id-1"))
	g.Expect(unmarshalled.InferenceService).To(Equal("test-service"))
	g.Expect(unmarshalled.Namespace).To(Equal("default"))
	g.Expect(unmarshalled.Component).To(Equal("predictor"))
}

func TestHTTPMarshallerMultipleRecords(t *testing.T) {
	g := NewGomegaWithT(t)

	server := httptest.NewServer(createTestMarshallerHandler("json", http.StatusOK, false))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	marshaller := NewHTTPMarshaller(server.URL, client)

	batch := []LogRequest{
		{
			Id:               "test-id-1",
			InferenceService: "test-service",
			Namespace:        "default",
		},
		{
			Id:               "test-id-2",
			InferenceService: "test-service-2",
			Namespace:        "kserve",
		},
	}

	response, err := marshaller.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response).ToNot(BeNil())
	g.Expect(response.Extension).To(Equal("json"))

	// Verify the response data is a JSON array
	var unmarshalled []LogRequest
	err = json.Unmarshal(response.Data, &unmarshalled)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(unmarshalled).To(HaveLen(2))
	g.Expect(unmarshalled[0].Id).To(Equal("test-id-1"))
	g.Expect(unmarshalled[1].Id).To(Equal("test-id-2"))
}

func TestHTTPMarshallerHTTPError(t *testing.T) {
	g := NewGomegaWithT(t)

	server := httptest.NewServer(createTestMarshallerHandler("json", http.StatusInternalServerError, false))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	marshaller := NewHTTPMarshaller(server.URL, client)

	batch := []LogRequest{
		{
			Id:               "test-id-1",
			InferenceService: "test-service",
		},
	}

	response, err := marshaller.Marshal(batch)
	g.Expect(err).To(HaveOccurred())
	g.Expect(response).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("500"))
}

func TestHTTPMarshallerCustomExtensionHeader(t *testing.T) {
	g := NewGomegaWithT(t)

	server := httptest.NewServer(createTestMarshallerHandler("parquet", http.StatusOK, false))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	marshaller := NewHTTPMarshaller(server.URL, client)

	batch := []LogRequest{
		{
			Id:               "test-id-1",
			InferenceService: "test-service",
		},
	}

	response, err := marshaller.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response).ToNot(BeNil())
	g.Expect(response.Extension).To(Equal("parquet"))
}

func TestHTTPMarshallerMissingExtensionHeader(t *testing.T) {
	g := NewGomegaWithT(t)

	// Empty string means no extension header will be set
	server := httptest.NewServer(createTestMarshallerHandler("", http.StatusOK, false))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	marshaller := NewHTTPMarshaller(server.URL, client)

	batch := []LogRequest{
		{
			Id:               "test-id-1",
			InferenceService: "test-service",
		},
	}

	response, err := marshaller.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response).ToNot(BeNil())
	g.Expect(response.Extension).To(Equal("json"))
}

func TestHTTPMarshallerTimeout(t *testing.T) {
	g := NewGomegaWithT(t)

	server := httptest.NewServer(createTestMarshallerHandler("json", http.StatusOK, true))
	defer server.Close()

	// Set a very short timeout
	client := &http.Client{Timeout: 50 * time.Millisecond}
	marshaller := NewHTTPMarshaller(server.URL, client)

	batch := []LogRequest{
		{
			Id:               "test-id-1",
			InferenceService: "test-service",
		},
	}

	response, err := marshaller.Marshal(batch)
	g.Expect(err).To(HaveOccurred())
	g.Expect(response).To(BeNil())
	// Error message should contain either "timeout" or "deadline exceeded"
	g.Expect(err.Error()).To(Or(ContainSubstring("timeout"), ContainSubstring("deadline exceeded")))
}

func TestHTTPMarshallerEmptyBatch(t *testing.T) {
	g := NewGomegaWithT(t)

	server := httptest.NewServer(createTestMarshallerHandler("json", http.StatusOK, false))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	marshaller := NewHTTPMarshaller(server.URL, client)

	batch := []LogRequest{}

	response, err := marshaller.Marshal(batch)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response).ToNot(BeNil())
	g.Expect(response.Extension).To(Equal("json"))

	// Verify the response data is an empty JSON array
	var unmarshalled []LogRequest
	err = json.Unmarshal(response.Data, &unmarshalled)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(unmarshalled).To(BeEmpty())
}

// Property-based testing: verify that the marshaller correctly handles batches of varying sizes
func TestHTTPMarshallerVariousBatchSizes(t *testing.T) {
	server := httptest.NewServer(createTestMarshallerHandler("json", http.StatusOK, false))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	marshaller := NewHTTPMarshaller(server.URL, client)

	// Property: For any batch size N, the unmarshalled response should contain exactly N records
	batchSizes := []int{1, 2, 5, 10, 50, 100}
	for _, size := range batchSizes {
		t.Run(fmt.Sprintf("BatchSize_%d", size), func(t *testing.T) {
			g := NewGomegaWithT(t)

			batch := make([]LogRequest, size)
			for i := range size {
				batch[i] = LogRequest{
					Id:               fmt.Sprintf("test-id-%d", i),
					InferenceService: fmt.Sprintf("test-service-%d", i),
				}
			}

			response, err := marshaller.Marshal(batch)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(response).ToNot(BeNil())
			g.Expect(response.Extension).To(Equal("json"))

			if size == 1 {
				// Single record should unmarshal as a single object
				var unmarshalled LogRequest
				err = json.Unmarshal(response.Data, &unmarshalled)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(unmarshalled.Id).To(Equal("test-id-0"))
			} else {
				// Multiple records should unmarshal as an array
				var unmarshalled []LogRequest
				err = json.Unmarshal(response.Data, &unmarshalled)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(unmarshalled).To(HaveLen(size))

				// Property: Each record ID should match the generated pattern
				for i, record := range unmarshalled {
					g.Expect(record.Id).To(Equal(fmt.Sprintf("test-id-%d", i)))
				}
			}
		})
	}
}
