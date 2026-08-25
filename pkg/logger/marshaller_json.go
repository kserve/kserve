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
	"net/http"
)

// jsonMarshallerHandler implements http.Handler to marshal LogRequest batches to JSON.
type jsonMarshallerHandler struct{}

// NewJSONMarshallerHandler creates a new HTTP handler that marshals LogRequest batches to JSON.
// It accepts POST requests with a JSON array of LogRequest objects and returns:
// - For single-record batches (len == 1): the marshalled LogRequest object
// - For multi-record batches: a JSON array of marshalled LogRequest objects
// - For empty batches: an empty JSON array
//
// The handler sets the following response headers:
// - Content-Type: application/json
// - X-Log-Marshal-Extension: json
//
// Error responses:
// - 405 Method Not Allowed: for non-POST requests
// - 400 Bad Request: for invalid JSON input
func NewJSONMarshallerHandler() http.Handler {
	return &jsonMarshallerHandler{}
}

func (h *jsonMarshallerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Log-Marshal-Extension", "json")

	// Marshal the response
	var responseBytes []byte
	var err error

	if len(batch) == 1 {
		// Single record: marshal the single LogRequest object
		responseBytes, err = json.Marshal(batch[0])
	} else {
		// Multiple or empty: marshal the entire batch array
		responseBytes, err = json.Marshal(batch)
	}

	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(responseBytes)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
	}
}
