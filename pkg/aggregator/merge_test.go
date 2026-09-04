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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeModels(t *testing.T) {
	results := []BackendResult{
		{
			Backend:    Backend{Name: "a", Namespace: "ns"},
			StatusCode: 200,
			Body:       []byte(`{"object":"list","data":[{"id":"model-a","object":"model"}]}`),
		},
		{
			Backend:    Backend{Name: "b", Namespace: "ns"},
			StatusCode: 200,
			Body:       []byte(`{"object":"list","data":[{"id":"model-b","object":"model"}]}`),
		},
		{
			Backend:    Backend{Name: "c", Namespace: "ns"},
			StatusCode: 500,
			Body:       []byte(`error`),
		},
	}

	body, status, err := MergeModels(results)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)

	var list struct {
		Data []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	require.Len(t, list.Data, 2)
	assert.Equal(t, "model-a", list.Data[0].ID)
	assert.Equal(t, "ns/a", list.Data[0].Source)
	assert.Equal(t, "model-b", list.Data[1].ID)
}

func TestMergeModelsPreservesBackendFields(t *testing.T) {
	results := []BackendResult{
		{
			Backend:    Backend{Name: "a", Namespace: "ns"},
			StatusCode: 200,
			Body:       []byte(`{"object":"list","data":[{"id":"llama","object":"model","max_model_len":4096}]}`),
		},
	}
	body, status, err := MergeModels(results)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `"id":"llama"`)
	assert.Contains(t, string(body), `"max_model_len":4096`)
	assert.Contains(t, string(body), `"source":"ns/a"`)
}

func TestMergeModelsAllFailed(t *testing.T) {
	results := []BackendResult{
		{Backend: Backend{Name: "a"}, Err: assert.AnError},
	}
	_, status, err := MergeModels(results)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, status)
}

func TestMergeModelsInvalidJSONIsNotSuccess(t *testing.T) {
	results := []BackendResult{
		{
			Backend:    Backend{Name: "a", Namespace: "ns"},
			StatusCode: 200,
			Body:       []byte(`not-json`),
		},
	}
	_, status, err := MergeModels(results)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadGateway, status)
}

func TestMergeHealth(t *testing.T) {
	ok := []BackendResult{
		{Backend: Backend{Name: "a", Namespace: "ns"}, StatusCode: 200, Body: []byte("ok")},
		{Backend: Backend{Name: "b", Namespace: "ns"}, StatusCode: 200, Body: []byte("ok")},
	}
	body, status, err := MergeHealth(ok)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `"status":"healthy"`)

	bad := []BackendResult{
		{Backend: Backend{Name: "a", Namespace: "ns"}, StatusCode: 200},
		{Backend: Backend{Name: "b", Namespace: "ns"}, StatusCode: 503},
	}
	_, status, err = MergeHealth(bad)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, status)
}

func TestMergeMetricsInjectsLabel(t *testing.T) {
	results := []BackendResult{
		{
			Backend:    Backend{Name: "a", Namespace: "ns"},
			StatusCode: 200,
			Body:       []byte("http_requests_total{code=\"200\"} 1\nup 1\n"),
		},
	}
	body, status, err := MergeMetrics(results)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `backend="ns/a"`)
	assert.Contains(t, string(body), `http_requests_total{code="200",backend="ns/a"} 1`)
	assert.Contains(t, string(body), `up{backend="ns/a"} 1`)
}

func TestMergeLoad(t *testing.T) {
	results := []BackendResult{
		{
			Backend:    Backend{Name: "a", Namespace: "ns"},
			StatusCode: 200,
			Body:       []byte(`{"running":1}`),
		},
		{
			Backend: Backend{Name: "b", Namespace: "ns"},
			Err:     assert.AnError,
		},
	}
	body, status, err := MergeLoad(results)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `"ns/a"`)
	assert.NotContains(t, string(body), `"ns/b"`)
}
