/*
Copyright 2019 The Knative Authors
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
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knative/serving/pkg/activator"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/queue"
)

func TestProbeHandler(t *testing.T) {
	examples := []struct {
		label          string
		headers        http.Header
		passed         bool
		expectedStatus int
		method         string
	}{{
		label:          "forward a normal POST request",
		headers:        http.Header{},
		passed:         true,
		expectedStatus: http.StatusOK,
		method:         http.MethodPost,
	}, {
		label:          "filter a POST request containing probe header, even if probe is for a different target",
		headers:        mapToHeader(map[string]string{network.ProbeHeaderName: queue.Name}),
		passed:         false,
		expectedStatus: http.StatusBadRequest,
		method:         http.MethodPost,
	}, {
		label:          "filter a POST request containing probe header",
		headers:        mapToHeader(map[string]string{network.ProbeHeaderName: activator.Name}),
		passed:         false,
		expectedStatus: http.StatusOK,
		method:         http.MethodPost,
	}, {
		label:          "forward a normal GET request",
		headers:        http.Header{},
		passed:         true,
		expectedStatus: http.StatusOK,
		method:         http.MethodGet,
	}, {
		label:          "filter a GET request containing probe header, with wrong target system",
		headers:        mapToHeader(map[string]string{network.ProbeHeaderName: "not-empty"}),
		passed:         false,
		expectedStatus: http.StatusBadRequest,
		method:         http.MethodGet,
	}, {
		label:          "filter a GET request containing probe header",
		headers:        mapToHeader(map[string]string{network.ProbeHeaderName: activator.Name}),
		passed:         false,
		expectedStatus: http.StatusOK,
		method:         http.MethodGet,
	}, {
		label:          "forward a request containing empty retry header",
		headers:        mapToHeader(map[string]string{network.ProbeHeaderName: ""}),
		passed:         true,
		expectedStatus: http.StatusOK,
		method:         http.MethodPost,
	}}

	for _, e := range examples {
		t.Run(e.label, func(t *testing.T) {
			wasPassed := false
			baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wasPassed = true
				w.WriteHeader(http.StatusOK)
			})
			handler := ProbeHandler{NextHandler: baseHandler}

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(e.method, "http://example.com", nil)
			req.Header = e.headers

			handler.ServeHTTP(resp, req)

			if wasPassed != e.passed {
				if !e.passed {
					t.Error("Request got passed to the next handler unexpectedly")
				} else {
					t.Error("Request was not passed to the next handler as expected")
				}
			}

			if resp.Code != e.expectedStatus {
				t.Errorf("Unexpected response status. Want %d, got %d", e.expectedStatus, resp.Code)
			}
		})
	}
}

func mapToHeader(m map[string]string) http.Header {
	h := http.Header{}
	for k, v := range m {
		h.Add(k, v)
	}
	return h
}
