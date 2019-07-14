/*
Copyright 2018 The Knative Authors

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

package util

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"

	"github.com/knative/serving/pkg/activator"
)

func TestHeaderPruning(t *testing.T) {
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(activator.RevisionHeaderName) != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Header.Get(activator.RevisionHeaderNamespace) != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}

	server := httptest.NewServer(handler)
	serverURL, _ := url.Parse(server.URL)

	defer server.Close()

	tests := []struct {
		name   string
		header string
	}{{
		name:   "revision name header",
		header: activator.RevisionHeaderName,
	}, {
		name:   "revision namespace header",
		header: activator.RevisionHeaderNamespace,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proxy := httputil.NewSingleHostReverseProxy(serverURL)
			SetupHeaderPruning(proxy)

			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
			req.Header.Set(test.header, "some-value")

			proxy.ServeHTTP(resp, req)

			if resp.Code != http.StatusOK {
				t.Errorf("expected header %q to be filtered", test.header)
			}
		})
	}
}
