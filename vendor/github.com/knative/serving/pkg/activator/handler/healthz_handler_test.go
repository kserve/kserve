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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	examples := []struct {
		name           string
		headers        http.Header
		passed         bool
		expectedStatus int
		check          func() error
	}{{
		name:           "forward non-kubelet request",
		headers:        mapToHeader(map[string]string{"User-Agent": "chromium/734.6.5"}),
		passed:         true,
		expectedStatus: http.StatusOK,
	}, {
		name:           "kubelet probe success",
		headers:        mapToHeader(map[string]string{"User-Agent": "kube-probe/something"}),
		passed:         false,
		expectedStatus: http.StatusOK,
		check:          func() error { return nil },
	}, {
		name:           "kubelet probe failure",
		headers:        mapToHeader(map[string]string{"User-Agent": "kube-probe/something"}),
		passed:         false,
		expectedStatus: http.StatusInternalServerError,
		check:          func() error { return errors.New("not ready") },
	}}

	for _, e := range examples {
		t.Run(e.name, func(t *testing.T) {
			wasPassed := false
			baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wasPassed = true
				w.WriteHeader(http.StatusOK)
			})
			handler := HealthHandler{HealthCheck: e.check, NextHandler: baseHandler}

			resp := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "http://example.com", nil)
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
