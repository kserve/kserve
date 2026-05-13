/*
Copyright 2023 The KServe Authors.

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

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	pkgtest "github.com/kserve/kserve/pkg/testing"
)

func init() {
	pkgtest.SetupTestLogger()
}

func Int64Ptr(i int64) *int64 {
	return &i
}

func TestSimpleModelChainer(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		var request map[string]interface{}
		raw_request, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		err = json.Unmarshal(raw_request, &request)
		if err != nil {
			return
		}
		_, ok := request["instances"]
		assert.True(t, ok)

		response := map[string]interface{}{"predictions": "1"}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()
	model2 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		var request map[string]interface{}
		raw_request, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		err = json.Unmarshal(raw_request, &request)
		if err != nil {
			return
		}
		_, ok := request["predictions"]
		assert.True(t, ok)

		response := map[string]interface{}{"predictions": "2"}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model2Url, err := apis.ParseURL(model2.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model2.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model1",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model1Url.String(),
						},
						MapPredictionsToInstances: true,
					},
					{
						StepName: "model2",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model2Url.String(),
						},
						Data: "$response",
					},
				},
			},
		},
	}
	input := map[string]interface{}{
		"predictions": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization": {"Bearer Token"},
	}

	res, _, err := routeStep("root", graphSpec, jsonBytes, headers)
	if err != nil {
		t.Fatalf("routeStep failed: %v", err)
	}
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	expectedResponse := map[string]interface{}{
		"predictions": "2",
	}
	t.Logf("final response:%v", response)
	assert.Equal(t, expectedResponse, response)
}

func TestSimpleModelEnsemble(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		response := map[string]interface{}{"predictions": "1"}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()
	model2 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		response := map[string]interface{}{"predictions": "2"}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model2Url, err := apis.ParseURL(model2.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model2.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Ensemble,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model1",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model1Url.String(),
						},
					},
					{
						StepName: "model2",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model2Url.String(),
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{
		"instances": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization": {"Bearer Token"},
	}
	res, _, err := routeStep("root", graphSpec, jsonBytes, headers)
	if err != nil {
		t.Fatalf("routeStep failed: %v", err)
	}
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	expectedResponse := map[string]interface{}{
		"model1": map[string]interface{}{
			"predictions": "1",
		},
		"model2": map[string]interface{}{
			"predictions": "2",
		},
	}
	t.Logf("final response:%v", response)
	assert.Equal(t, expectedResponse, response)
}

func TestInferenceGraphWithCondition(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		response := map[string]interface{}{
			"predictions": []map[string]interface{}{
				{
					"label": "cat",
					"score": []float32{
						0.1, 0.9,
					},
				},
			},
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()
	model2 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		response := map[string]interface{}{
			"predictions": []map[string]interface{}{
				{
					"label": "dog",
					"score": []float32{
						0.8, 0.2,
					},
				},
			},
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model2Url, err := apis.ParseURL(model2.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model2.Close()

	// Start a local HTTP server
	model3 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		response := map[string]interface{}{
			"predictions": []map[string]interface{}{
				{
					"label": "beagle",
					"score": []float32{
						0.1, 0.9,
					},
				},
			},
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model3Url, err := apis.ParseURL(model3.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model3.Close()
	model4 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		response := map[string]interface{}{
			"predictions": []map[string]interface{}{
				{
					"label": "poodle",
					"score": []float32{
						0.8, 0.2,
					},
				},
			},
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model4Url, err := apis.ParseURL(model4.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model4.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "step1",
						InferenceTarget: v1alpha1.InferenceTarget{
							NodeName: "animal-categorize",
						},
					},
					{
						StepName: "step2",
						InferenceTarget: v1alpha1.InferenceTarget{
							NodeName: "breed-categorize",
						},
						Condition: "predictions.#(label==\"dog\")",
					},
				},
			},
			"animal-categorize": {
				RouterType: v1alpha1.Switch,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model1",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model1Url.String(),
						},
						Condition: "instances.#(modelId==\"1\")",
					},
					{
						StepName: "model2",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model2Url.String(),
						},
						Condition: "instances.#(modelId==\"2\")",
					},
				},
			},
			"breed-categorize": {
				RouterType: v1alpha1.Ensemble,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model3",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model3Url.String(),
						},
					},
					{
						StepName: "model4",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model4Url.String(),
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{
		"instances": []map[string]string{
			{"modelId": "2"},
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization": {"Bearer Token"},
	}
	res, _, err := routeStep("root", graphSpec, jsonBytes, headers)
	if err != nil {
		t.Fatalf("routeStep failed: %v", err)
	}
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	expectedModel3Response := map[string]interface{}{
		"predictions": []interface{}{
			map[string]interface{}{
				"label": "beagle",
				"score": []interface{}{
					0.1, 0.9,
				},
			},
		},
	}

	expectedModel4Response := map[string]interface{}{
		"predictions": []interface{}{
			map[string]interface{}{
				"label": "poodle",
				"score": []interface{}{
					0.8, 0.2,
				},
			},
		},
	}
	t.Logf("final response:%v", response)
	assert.Equal(t, expectedModel3Response, response["model3"])
	assert.Equal(t, expectedModel4Response, response["model4"])
}

func TestInferenceGraphSequenceWithUnmetCondition(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		response := map[string]interface{}{
			"predictions": []map[string]interface{}{
				{
					"label": "cat",
					"score": []float32{
						0.8, 0.2,
					},
				},
			},
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "step1",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model1Url.String(),
						},
					},
					{
						StepName: "step2",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: "http://dummy", // Because in this test, this step won't be run.
						},
						Condition: "predictions.#(label==\"dog\")",
					},
				},
			},
		},
	}
	input := map[string]interface{}{
		"instances": []map[string]string{
			{"modelId": "1"},
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}
	res, statusCode, err := routeStep("root", graphSpec, jsonBytes, headers)
	if err != nil {
		t.Fatalf("routeStep failed: %v", err)
	}

	// Despite the condition for step2 is unmet, a 200 status code is expected.
	assert.Equal(t, http.StatusOK, statusCode)

	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	expectedResponse := map[string]interface{}{
		"predictions": []interface{}{
			map[string]interface{}{
				"label": "cat",
				"score": []interface{}{
					0.8, 0.2,
				},
			},
		},
	}
	t.Logf("final response:%v", response)
	assert.Equal(t, expectedResponse, response)
}

func TestCallServiceWhenNoneHeadersToPropagateIsEmpty(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		// Putting headers as part of response so that we can assert the headers' presence later
		response := make(map[string]interface{})
		response["predictions"] = "1"
		matchedHeaders := map[string]bool{}
		for _, p := range compiledHeaderPatterns {
			for h, values := range req.Header {
				if _, ok := matchedHeaders[h]; !ok && p.MatchString(h) {
					matchedHeaders[h] = true
					response[h] = values[0]
				}
			}
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()

	input := map[string]interface{}{
		"instances": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization":   {"Bearer Token"},
		"Test-Header-Key": {"Test-Header-Value"},
	}
	// Propagating no header
	compiledHeaderPatterns = []*regexp.Regexp{}
	res, _, err := callService(model1Url.String(), jsonBytes, headers)
	if err != nil {
		t.Fatalf("callService failed: %v", err)
	}
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	expectedResponse := map[string]interface{}{
		"predictions": "1",
	}
	t.Logf("final response:%v", response)
	assert.Equal(t, expectedResponse, response)
}

func TestCallServiceWhen1HeaderToPropagate(t *testing.T) {
	// Start a local HTTP serverq
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		// Putting headers as part of response so that we can assert the headers' presence later
		response := make(map[string]interface{})
		response["predictions"] = "1"
		matchedHeaders := map[string]bool{}
		for _, p := range compiledHeaderPatterns {
			for h, values := range req.Header {
				if _, ok := matchedHeaders[h]; !ok && p.MatchString(h) {
					matchedHeaders[h] = true
					response[h] = values[0]
				}
			}
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()

	input := map[string]interface{}{
		"instances": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization":   {"Bearer Token"},
		"Test-Header-Key": {"Test-Header-Value"},
	}
	// Propagating only 1 header "Test-Header-Key"
	headersToPropagate := []string{"Test-Header-Key"}
	compiledHeaderPatterns, err = compilePatterns(headersToPropagate)
	require.NoError(t, err)

	res, _, err := callService(model1Url.String(), jsonBytes, headers)
	require.NoError(t, err)

	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	require.NoError(t, err)

	expectedResponse := map[string]interface{}{
		"predictions":     "1",
		"Test-Header-Key": "Test-Header-Value",
	}
	t.Logf("final response:%v", response)
	assert.Equal(t, expectedResponse, response)
}

func TestCallServiceWhenMultipleHeadersToPropagate(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		// Putting headers as part of response so that we can assert the headers' presence later
		response := make(map[string]interface{})
		response["predictions"] = "1"
		matchedHeaders := map[string]bool{}
		for _, p := range compiledHeaderPatterns {
			for h, values := range req.Header {
				if _, ok := matchedHeaders[h]; !ok && p.MatchString(h) {
					matchedHeaders[h] = true
					response[h] = values[0]
				}
			}
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()

	input := map[string]interface{}{
		"instances": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization":   {"Bearer Token"},
		"Test-Header-Key": {"Test-Header-Value"},
	}
	// Propagating multiple headers "Test-Header-Key"
	headersToPropagate := []string{"Test-Header-Key", "Authorization"}
	compiledHeaderPatterns, err = compilePatterns(headersToPropagate)
	require.NoError(t, err)

	res, _, err := callService(model1Url.String(), jsonBytes, headers)
	if err != nil {
		t.Fatalf("callService failed: %v", err)
	}
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	expectedResponse := map[string]interface{}{
		"predictions":     "1",
		"Test-Header-Key": "Test-Header-Value",
		"Authorization":   "Bearer Token",
	}
	t.Logf("final response:%v", response)
	assert.Equal(t, expectedResponse, response)
}

func TestMalformedURL(t *testing.T) {
	malformedURL := "http://single-1.default.{$your-domain}/switch"
	_, response, err := callService(malformedURL, []byte{}, http.Header{})
	require.Error(t, err)
	require.Equal(t, 500, response)
}

func TestCallServiceWhenMultipleHeadersToPropagateUsingPatterns(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		// Putting headers as part of response so that we can assert the headers' presence later
		response := make(map[string]interface{})
		response["predictions"] = "1"
		matchedHeaders := map[string]bool{}
		for _, p := range compiledHeaderPatterns {
			for h, values := range req.Header {
				if _, ok := matchedHeaders[h]; !ok && p.MatchString(h) {
					matchedHeaders[h] = true
					response[h] = values[0]
				}
			}
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()

	input := map[string]interface{}{
		"instances": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization": {"Bearer Token"},
		"Test-Header-1": {"Test-Header-1"},
		"Test-Header-2": {"Test-Header-2"},
		"Test-Header-3": {"Test-Header-3"},
	}
	// Propagating multiple headers "Test-Header-Key"
	headersToPropagate := []string{"Test-Header-*", "Auth*"}
	compiledHeaderPatterns, err = compilePatterns(headersToPropagate)
	require.NoError(t, err)

	res, _, err := callService(model1Url.String(), jsonBytes, headers)
	if err != nil {
		t.Fatalf("callService failed: %v", err)
	}
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	expectedResponse := map[string]interface{}{
		"predictions":   "1",
		"Test-Header-1": "Test-Header-1",
		"Test-Header-2": "Test-Header-2",
		"Test-Header-3": "Test-Header-3",
		"Authorization": "Bearer Token",
	}
	t.Logf("final response:%v", response)
	require.Equal(t, expectedResponse, response)
}

func TestCallServiceWhenMultipleHeadersToPropagateUsingInvalidPattern(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		// Putting headers as part of response so that we can assert the headers' presence later
		response := make(map[string]interface{})
		response["predictions"] = "1"
		matchedHeaders := map[string]bool{}
		for _, p := range compiledHeaderPatterns {
			for h, values := range req.Header {
				if _, ok := matchedHeaders[h]; !ok && p.MatchString(h) {
					matchedHeaders[h] = true
					response[h] = values[0]
				}
			}
		}
		responseBytes, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		_, err = rw.Write(responseBytes)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model1.Close()

	input := map[string]interface{}{
		"instances": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization": {"Bearer Token"},
		"Test-Header-1": {"Test-Header-1"},
		"Test-Header-2": {"Test-Header-2"},
		"Test-Header-3": {"Test-Header-3"},
	}
	// Using invalid regex pattern
	headersToPropagate := []string{"Test-Header-[0-9", "Auth*"}
	compiledHeaderPatterns, err = compilePatterns(headersToPropagate)
	require.Error(t, err)

	res, _, err := callService(model1Url.String(), jsonBytes, headers)
	if err != nil {
		t.Fatalf("callService failed: %v", err)
	}
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	// Invalid pattern should be ignored.
	expectedResponse := map[string]interface{}{
		"predictions":   "1",
		"Authorization": "Bearer Token",
	}
	t.Logf("final response:%v", response)
	require.Equal(t, expectedResponse, response)
}

func TestServerTimeout(t *testing.T) {
	testCases := []struct {
		name                string
		serverTimeout       *int64
		serviceStepDuration time.Duration
		expectError         bool
	}{
		{
			name:                "default",
			serverTimeout:       nil,
			serviceStepDuration: 1 * time.Millisecond,
			expectError:         false,
		},
		{
			name:                "timeout",
			serverTimeout:       Int64Ptr(1),
			serviceStepDuration: 500 * time.Millisecond,
			expectError:         true,
		},
		{
			name:                "success",
			serverTimeout:       Int64Ptr(2),
			serviceStepDuration: 500 * time.Millisecond,
			expectError:         false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			drainSleepDuration = 0 * time.Millisecond // instant shutdown

			// Setup and start dummy models
			model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				_, err := io.ReadAll(req.Body)
				if err != nil {
					return
				}
				time.Sleep(testCase.serviceStepDuration)
				response := map[string]interface{}{"predictions": "1"}
				responseBytes, _ := json.Marshal(response)
				_, _ = rw.Write(responseBytes)
			}))
			model1Url, err := apis.ParseURL(model1.URL)
			if err != nil {
				t.Fatalf("Failed to parse model url")
			}
			defer model1.Close()

			model2 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				_, err := io.ReadAll(req.Body)
				if err != nil {
					return
				}
				time.Sleep(testCase.serviceStepDuration)
				response := map[string]interface{}{"predictions": "2"}
				responseBytes, _ := json.Marshal(response)
				_, _ = rw.Write(responseBytes)
			}))
			model2Url, err := apis.ParseURL(model2.URL)
			if err != nil {
				t.Fatalf("Failed to parse model url")
			}
			defer model2.Close()

			// Create InferenceGraph
			graphSpec := v1alpha1.InferenceGraphSpec{
				Nodes: map[string]v1alpha1.InferenceRouter{
					"root": {
						RouterType: v1alpha1.Sequence,
						Steps: []v1alpha1.InferenceStep{
							{
								StepName: "model1",
								InferenceTarget: v1alpha1.InferenceTarget{
									ServiceURL: model1Url.String(),
								},
							},
							{
								StepName: "model2",
								InferenceTarget: v1alpha1.InferenceTarget{
									ServiceURL: model2Url.String(),
								},
								Data: "$response",
							},
						},
					},
				},
			}
			if testCase.serverTimeout != nil {
				timeout := *testCase.serverTimeout
				graphSpec.RouterTimeouts = &v1alpha1.InfereceGraphRouterTimeouts{
					ServerRead:  &timeout,
					ServerWrite: &timeout,
					ServerIdle:  &timeout,
				}
			}
			jsonBytes, _ := json.Marshal(graphSpec)
			*jsonGraph = string(jsonBytes)

			// Start InferenceGraph router server in a separate goroutine
			go func() {
				main()
			}()
			t.Cleanup(func() {
				http.DefaultServeMux = http.NewServeMux() // reset http handlers
				signalChan <- syscall.SIGTERM             // shutdown the server
				time.Sleep(100 * time.Millisecond)        // wait for server to release port before next subtest
			})

			// Call the InferenceGraph
			client := &http.Client{}
			time.Sleep(1 * time.Second) // prevent race condition
			req, _ := http.NewRequest(http.MethodPost, "http://localhost:"+strconv.Itoa(constants.RouterPort), bytes.NewBuffer(nil))
			resp, err := client.Do(req)
			if resp != nil {
				defer resp.Body.Close()
			}

			if testCase.expectError {
				assert.Contains(t, err.Error(), "EOF")
			} else {
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			}
		})
	}
}

func TestRetryOnTransient500(t *testing.T) {
	// Model returns 500 on the first two calls, then 200 on the third.
	callCount := 0
	model := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.ReadAll(req.Body)
		callCount++
		if callCount <= 2 {
			rw.WriteHeader(http.StatusInternalServerError)
			response := map[string]interface{}{"error": "transient failure"}
			responseBytes, _ := json.Marshal(response)
			_, _ = rw.Write(responseBytes)
			return
		}
		response := map[string]interface{}{"predictions": "success"}
		responseBytes, _ := json.Marshal(response)
		_, _ = rw.Write(responseBytes)
	}))
	modelUrl, err := apis.ParseURL(model.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: modelUrl.String(),
						},
						Retry: &v1alpha1.RetryConfig{
							MaxRetries:               3,
							InitialDelayMilliseconds: 10,
							MaxDelayMilliseconds:     50,
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{"instances": []string{"test"}}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}

	res, statusCode, err := routeStep("root", graphSpec, jsonBytes, headers)
	require.NoError(t, err)
	assert.Equal(t, 200, statusCode)

	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	require.NoError(t, err)
	assert.Equal(t, "success", response["predictions"])
	assert.Equal(t, 3, callCount, "expected 2 failures + 1 success = 3 total calls")
}

func TestNoRetryOn4xx(t *testing.T) {
	// Model always returns 400 — should NOT be retried.
	callCount := 0
	model := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.ReadAll(req.Body)
		callCount++
		rw.WriteHeader(http.StatusBadRequest)
		response := map[string]interface{}{"error": "bad request"}
		responseBytes, _ := json.Marshal(response)
		_, _ = rw.Write(responseBytes)
	}))
	modelUrl, err := apis.ParseURL(model.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: modelUrl.String(),
						},
						Retry: &v1alpha1.RetryConfig{
							MaxRetries:               3,
							InitialDelayMilliseconds: 10,
							MaxDelayMilliseconds:     50,
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{"instances": []string{"test"}}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}

	res, statusCode, err := routeStep("root", graphSpec, jsonBytes, headers)
	require.NoError(t, err)
	assert.Equal(t, 400, statusCode)

	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	require.NoError(t, err)
	assert.Equal(t, "bad request", response["error"])
	assert.Equal(t, 1, callCount, "4xx should not be retried")
}

func TestRetryExhausted(t *testing.T) {
	// Model always returns 503 — retries should be exhausted.
	callCount := 0
	model := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.ReadAll(req.Body)
		callCount++
		rw.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{"error": "service unavailable"}
		responseBytes, _ := json.Marshal(response)
		_, _ = rw.Write(responseBytes)
	}))
	modelUrl, err := apis.ParseURL(model.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: modelUrl.String(),
						},
						Retry: &v1alpha1.RetryConfig{
							MaxRetries:               2,
							InitialDelayMilliseconds: 10,
							MaxDelayMilliseconds:     50,
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{"instances": []string{"test"}}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}

	res, statusCode, err := routeStep("root", graphSpec, jsonBytes, headers)
	require.NoError(t, err)
	assert.Equal(t, 503, statusCode)

	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	require.NoError(t, err)
	assert.Equal(t, "service unavailable", response["error"])
	assert.Equal(t, 3, callCount, "expected 1 initial + 2 retries = 3 total calls")
}

func TestRetryWithNoConfig(t *testing.T) {
	// Model returns 500 — without retry config, should fail on first attempt.
	callCount := 0
	model := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.ReadAll(req.Body)
		callCount++
		rw.WriteHeader(http.StatusInternalServerError)
		response := map[string]interface{}{"error": "server error"}
		responseBytes, _ := json.Marshal(response)
		_, _ = rw.Write(responseBytes)
	}))
	modelUrl, err := apis.ParseURL(model.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: modelUrl.String(),
						},
						// No Retry config
					},
				},
			},
		},
	}
	input := map[string]interface{}{"instances": []string{"test"}}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}

	_, statusCode, err := routeStep("root", graphSpec, jsonBytes, headers)
	require.NoError(t, err)
	assert.Equal(t, 500, statusCode)
	assert.Equal(t, 1, callCount, "without retry config should only call once")
}

func TestEnsembleRetryOnTransientFailure(t *testing.T) {
	// Model 1 returns 503 on first call, then 200. Model 2 always returns 200.
	model1CallCount := 0
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.ReadAll(req.Body)
		model1CallCount++
		if model1CallCount <= 1 {
			rw.WriteHeader(http.StatusServiceUnavailable)
			response := map[string]interface{}{"error": "not ready"}
			responseBytes, _ := json.Marshal(response)
			_, _ = rw.Write(responseBytes)
			return
		}
		response := map[string]interface{}{"predictions": "model1"}
		responseBytes, _ := json.Marshal(response)
		_, _ = rw.Write(responseBytes)
	}))
	model1Url, err := apis.ParseURL(model1.URL)
	require.NoError(t, err)
	defer model1.Close()

	model2 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, _ = io.ReadAll(req.Body)
		response := map[string]interface{}{"predictions": "model2"}
		responseBytes, _ := json.Marshal(response)
		_, _ = rw.Write(responseBytes)
	}))
	model2Url, err := apis.ParseURL(model2.URL)
	require.NoError(t, err)
	defer model2.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Ensemble,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model1",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model1Url.String(),
						},
						Retry: &v1alpha1.RetryConfig{
							MaxRetries:               3,
							InitialDelayMilliseconds: 10,
							MaxDelayMilliseconds:     50,
						},
					},
					{
						StepName: "model2",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: model2Url.String(),
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{"instances": []string{"test"}}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}

	res, statusCode, err := routeStep("root", graphSpec, jsonBytes, headers)
	require.NoError(t, err)
	assert.Equal(t, 200, statusCode)

	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	require.NoError(t, err)

	// Both models should be in the ensemble response
	model1Resp, ok := response["model1"].(map[string]interface{})
	assert.True(t, ok, "model1 should be in ensemble response")
	assert.Equal(t, "model1", model1Resp["predictions"])

	model2Resp, ok := response["model2"].(map[string]interface{})
	assert.True(t, ok, "model2 should be in ensemble response")
	assert.Equal(t, "model2", model2Resp["predictions"])

	assert.Equal(t, 2, model1CallCount, "model1 should have been called twice (1 failure + 1 success)")
}

// TestGraphLevelDefaultRetry verifies that steps without per-step retry inherit the graph's defaultRetry.
func TestGraphLevelDefaultRetry(t *testing.T) {
	callCount := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"predictions": "ok"}`))
	}))
	defer model.Close()
	modelUrl, err := apis.ParseURL(model.URL)
	require.NoError(t, err)

	graphSpec := v1alpha1.InferenceGraphSpec{
		DefaultRetry: &v1alpha1.RetryConfig{
			MaxRetries:               3,
			InitialDelayMilliseconds: 10,
			MaxDelayMilliseconds:     50,
		},
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: modelUrl.String(),
						},
						// No per-step retry — should inherit graph-level default
					},
				},
			},
		},
	}
	input := map[string]interface{}{"instances": []string{"test"}}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}

	res, statusCode, err := routeStep("root", graphSpec, jsonBytes, headers)
	require.NoError(t, err)
	assert.Equal(t, 200, statusCode)

	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response["predictions"])
	assert.Equal(t, 3, callCount, "should have been called 3 times (2 failures + 1 success via graph-level retry)")
}

// TestStepRetryOverridesGraphDefault verifies that per-step retry takes precedence over graph defaultRetry.
func TestStepRetryOverridesGraphDefault(t *testing.T) {
	callCount := 0
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// Always return 503 so we can count retry attempts
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error": "unavailable"}`))
	}))
	defer model.Close()
	modelUrl, err := apis.ParseURL(model.URL)
	require.NoError(t, err)

	graphSpec := v1alpha1.InferenceGraphSpec{
		DefaultRetry: &v1alpha1.RetryConfig{
			MaxRetries:               5,
			InitialDelayMilliseconds: 10,
			MaxDelayMilliseconds:     50,
		},
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Sequence,
				Steps: []v1alpha1.InferenceStep{
					{
						StepName: "model",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceURL: modelUrl.String(),
						},
						// Per-step retry overrides graph default
						Retry: &v1alpha1.RetryConfig{
							MaxRetries:               2,
							InitialDelayMilliseconds: 10,
							MaxDelayMilliseconds:     50,
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{"instances": []string{"test"}}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{}

	_, statusCode, _ := routeStep("root", graphSpec, jsonBytes, headers)
	assert.Equal(t, 503, statusCode)
	// 1 initial + 2 retries = 3 total (step-level maxRetries=2, NOT graph-level 5)
	assert.Equal(t, 3, callCount, "should use step-level retry (2) not graph-level (5)")
}
