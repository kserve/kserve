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
	"net/http"

	"github.com/parquet-go/parquet-go"
)

// parquetMarshallerHandler implements http.Handler to marshal LogRequest batches to Parquet.
type parquetMarshallerHandler struct{}

// NewParquetMarshallerHandler creates a new HTTP handler that marshals LogRequest batches to Parquet.
// It accepts POST requests with a JSON array of LogRequest objects and returns Parquet binary output with:
// - Schema defined by the logRecord struct with parquet tags
// - Data rows converted via toLogRecord()
//
// The handler sets the following response headers:
// - Content-Type: application/octet-stream
// - X-Log-Marshal-Extension: parquet
//
// Error responses:
// - 405 Method Not Allowed: for non-POST requests
// - 400 Bad Request: for invalid JSON input
// - 500 Internal Server Error: for Parquet writing errors
func NewParquetMarshallerHandler() http.Handler {
	return &parquetMarshallerHandler{}
}

func (h *parquetMarshallerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode the batch of LogRequests
	var batch []LogRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&batch); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create buffer and Parquet writer
	var buf bytes.Buffer
	writer := parquet.NewGenericWriter[logRecord](&buf)

	// Convert batch to logRecords
	if len(batch) > 0 {
		records := make([]logRecord, len(batch))
		for i, logReq := range batch {
			records[i] = toLogRecord(logReq)
		}

		// Write all records
		if _, err := writer.Write(records); err != nil {
			http.Error(w, "Failed to write Parquet records", http.StatusInternalServerError)
			return
		}
	}

	// Close the writer to flush the Parquet footer
	if err := writer.Close(); err != nil {
		http.Error(w, "Failed to close Parquet writer", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Log-Marshal-Extension", "parquet")

	// Write response
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}
