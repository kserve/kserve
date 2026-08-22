/*
Copyright 2026 The KServe Authors.

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

package aggregator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregatorHandlerModels(t *testing.T) {
	backendA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, PathModels, r.URL.Path)
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"model-a","object":"model"}]}`))
	}))
	defer backendA.Close()

	backendB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"model-b","object":"model"}]}`))
	}))
	defer backendB.Close()

	agg, err := New(Options{
		Discovery: StaticDiscovery{Backends: []Backend{
			{Name: "a", Namespace: "ns", URL: backendA.URL},
			{Name: "b", Namespace: "ns", URL: backendB.URL},
		}},
		Timeout: time.Second,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, PathModels, nil)
	rr := httptest.NewRecorder()
	agg.Handler().ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var list openAIModelList
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
}

func TestAggregatorHandlerHealthAllMustPass(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okServer.Close()
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer badServer.Close()

	agg, err := New(Options{
		Discovery: StaticDiscovery{Backends: []Backend{
			{Name: "ok", Namespace: "ns", URL: okServer.URL},
			{Name: "bad", Namespace: "ns", URL: badServer.URL},
		}},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, PathHealth, nil)
	rr := httptest.NewRecorder()
	agg.Handler().ServeHTTP(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestFanOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	results := FanOut(t.Context(), http.DefaultClient, []Backend{{Name: "a", URL: srv.URL}}, PathHealth, nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].OK())
	assert.Equal(t, "ok", string(results[0].Body))
}
