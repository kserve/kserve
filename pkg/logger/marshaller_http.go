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
	"fmt"
	"io"
	"net/http"
)

// HTTPMarshaller is a pure HTTP client that implements the Marshaller interface.
// It POSTs []LogRequest as JSON to a configured URL and returns the marshalled response.
type HTTPMarshaller struct {
	url    string
	client *http.Client
}

// Compile-time check to ensure HTTPMarshaller implements Marshaller
var _ Marshaller = &HTTPMarshaller{}

// NewHTTPMarshaller creates a new HTTPMarshaller with the given URL and HTTP client.
// The client should be configured with an appropriate timeout.
func NewHTTPMarshaller(url string, client *http.Client) *HTTPMarshaller {
	return &HTTPMarshaller{
		url:    url,
		client: client,
	}
}

// Marshal sends a batch of LogRequests to the configured HTTP endpoint and returns the marshalled response.
// It POSTs the batch as JSON, reads the response body as MarshalResponse.Data,
// and reads the X-Log-Marshal-Extension header as MarshalResponse.Extension.
// If the X-Log-Marshal-Extension header is missing, it defaults to "json".
// HTTP errors (non-2xx status) are propagated as errors.
func (h *HTTPMarshaller) Marshal(batch []LogRequest) (*MarshalResponse, error) {
	// Marshal the batch to JSON
	requestBody, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch to JSON: %w", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest(http.MethodPost, h.url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body
	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Read the extension header, defaulting to "json" if missing
	extension := resp.Header.Get("X-Log-Marshal-Extension")
	if extension == "" {
		extension = "json"
	}

	return &MarshalResponse{
		Data:      responseData,
		Extension: extension,
	}, nil
}
