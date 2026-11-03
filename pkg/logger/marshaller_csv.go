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
)

// csvMarshallerHandler implements http.Handler to marshal LogRequest batches to CSV.
type csvMarshallerHandler struct{}

// NewCSVMarshallerHandler creates a new HTTP handler that marshals LogRequest batches to CSV.
// It accepts POST requests with a JSON array of LogRequest objects and returns CSV output with:
// - A header row containing column names from logRecordColumns()
// - Data rows converted via toLogRecord() and logRecordToStrings()
//
// The handler sets the following response headers:
// - Content-Type: text/csv
// - X-Log-Marshal-Extension: csv
//
// Error responses:
// - 405 Method Not Allowed: for non-POST requests
// - 400 Bad Request: for invalid JSON input
// - 500 Internal Server Error: for CSV writing errors
func NewCSVMarshallerHandler() http.Handler {
	return &csvMarshallerHandler{}
}

func (h *csvMarshallerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	// Create buffer and CSV writer
	var buf bytes.Buffer
	csvWriter := csv.NewWriter(&buf)

	// Write header row
	header := logRecordColumns()
	if err := csvWriter.Write(header); err != nil {
		http.Error(w, "Failed to write CSV header", http.StatusInternalServerError)
		return
	}

	// Write data rows
	for _, logReq := range batch {
		record := toLogRecord(logReq)
		row := logRecordToStrings(record)
		if err := csvWriter.Write(row); err != nil {
			http.Error(w, "Failed to write CSV row", http.StatusInternalServerError)
			return
		}
	}

	// Flush the writer and check for errors
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		http.Error(w, "Failed to flush CSV writer", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("X-Log-Marshal-Extension", "csv")

	// Write response
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
	}
}
