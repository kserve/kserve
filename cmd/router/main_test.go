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
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/wait"
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
		"X-Request-Id":  {"12345"},
		"X-B3-Trace-Id": {"67890"},
	}
	// Propagating multiple headers "Test-Header-Key"
	headersToPropagate := []string{"Test-Header-*", "Auth*", ".*Trace-Id.*"}
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
		"X-B3-Trace-Id": "67890",
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
			serviceStepDuration: 1 * time.Second,
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
			isShuttingDown = false

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

			// Retry until the server is up instead of a fixed sleep.
			client := &http.Client{}
			url := "http://localhost:" + strconv.Itoa(constants.RouterPort)
			var statusCode int
			var lastErr error
			err = wait.PollUntilContextTimeout(t.Context(), 50*time.Millisecond, 5*time.Second, true, func(ctx context.Context) (bool, error) {
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(nil))
				if err != nil {
					return false, err
				}
				resp, reqErr := client.Do(req)
				if resp != nil {
					defer resp.Body.Close()
					statusCode = resp.StatusCode
				}
				lastErr = reqErr
				if reqErr != nil && errors.Is(reqErr, syscall.ECONNREFUSED) {
					return false, nil
				}
				return true, nil
			})
			require.NoError(t, err, "server did not become ready")

			if testCase.expectError {
				require.Error(t, lastErr)
				assert.Contains(t, lastErr.Error(), "EOF")
			} else {
				require.NoError(t, lastErr)
				assert.Equal(t, http.StatusOK, statusCode)
			}
		})
	}
}

func TestPickupRouteNeverReturnsNil(t *testing.T) {
	w1, w2 := int64(20), int64(80)
	routes := []v1alpha1.InferenceStep{
		{
			StepName: "model1",
			InferenceTarget: v1alpha1.InferenceTarget{
				ServiceURL: "http://example.com/model1",
			},
			Weight: &w1,
		},
		{
			StepName: "model2",
			InferenceTarget: v1alpha1.InferenceTarget{
				ServiceURL: "http://example.com/model2",
			},
			Weight: &w2,
		},
	}

	for i := range 10000 {
		route := pickupRoute(routes)
		require.NotNil(t, route, "pickupRoute returned nil on iteration %d", i)
	}
}

func TestPickupRouteAlwaysReturnsRouteForAllRandValues(t *testing.T) {
	w1, w2 := int64(30), int64(70)
	routes := []v1alpha1.InferenceStep{
		{
			StepName: "model1",
			InferenceTarget: v1alpha1.InferenceTarget{
				ServiceURL: "http://example.com/model1",
			},
			Weight: &w1,
		},
		{
			StepName: "model2",
			InferenceTarget: v1alpha1.InferenceTarget{
				ServiceURL: "http://example.com/model2",
			},
			Weight: &w2,
		},
	}

	for i := range 10000 {
		route := pickupRoute(routes)
		require.NotNil(t, route, "pickupRoute returned nil on iteration %d", i)
	}
}

func TestCryptoRandIntUpperBoundWithFix(t *testing.T) {
	for i := range 100_000 {
		n, err := crand.Int(crand.Reader, big.NewInt(100))
		require.NoError(t, err)
		require.True(t, n.Int64() >= 0 && n.Int64() < 100,
			"rand.Int(100) returned out-of-range value %d on iteration %d", n.Int64(), i)
	}
}

const nestedGraphShape = "NestedSequenceEnsemble"

// newTestMux returns a mux configured with the same routes as the router server.
func newTestMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerHandlers(mux)
	return mux
}

// newUnexpectedStep starts a graph step that fails the test if it receives any request.
func newUnexpectedStep(t *testing.T) string {
	t.Helper()
	step := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		t.Errorf("graph step must not be called, got %s %s", req.Method, req.URL.Path)
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(step.Close)
	return step.URL
}

// newPredictionStep starts a graph step that answers every request with the given prediction.
func newPredictionStep(t *testing.T, prediction string) string {
	t.Helper()
	step := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if _, err := io.ReadAll(req.Body); err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		responseBytes, err := json.Marshal(map[string]interface{}{"predictions": prediction})
		if err != nil {
			t.Errorf("failed to marshal response: %v", err)
		}
		_, _ = rw.Write(responseBytes)
	}))
	t.Cleanup(step.Close)
	return step.URL
}

// useGraph sets the graph served by graphHandler for the duration of the test.
func useGraph(t *testing.T, graphSpec v1alpha1.InferenceGraphSpec) {
	t.Helper()
	previous := inferenceGraph
	inferenceGraph = &graphSpec
	t.Cleanup(func() { inferenceGraph = previous })
}

// setShuttingDown sets the router shutdown state for the duration of the test.
func setShuttingDown(t *testing.T, shuttingDown bool) {
	t.Helper()
	previous := isShuttingDown
	isShuttingDown = shuttingDown
	t.Cleanup(func() { isShuttingDown = previous })
}

// healthTestGraph builds a graph of the given shape whose two model steps call url1 and url2.
func healthTestGraph(t *testing.T, shape string, url1 string, url2 string) v1alpha1.InferenceGraphSpec {
	t.Helper()
	model1 := v1alpha1.InferenceStep{StepName: "model1", InferenceTarget: v1alpha1.InferenceTarget{ServiceURL: url1}}
	model2 := v1alpha1.InferenceStep{StepName: "model2", InferenceTarget: v1alpha1.InferenceTarget{ServiceURL: url2}}
	root := v1alpha1.InferenceRouter{RouterType: v1alpha1.InferenceRouterType(shape)}
	nodes := map[string]v1alpha1.InferenceRouter{}
	switch shape {
	case string(v1alpha1.Sequence):
		model2.Data = "$response"
	case string(v1alpha1.Splitter):
		model1.Weight = Int64Ptr(50)
		model2.Weight = Int64Ptr(50)
	case string(v1alpha1.Ensemble):
	case string(v1alpha1.Switch):
		model1.Condition = "instances"
		model2.Condition = "inputs"
	case nestedGraphShape:
		root.RouterType = v1alpha1.Sequence
		root.Steps = []v1alpha1.InferenceStep{
			{StepName: "child", InferenceTarget: v1alpha1.InferenceTarget{NodeName: "child"}},
		}
		nodes["child"] = v1alpha1.InferenceRouter{
			RouterType: v1alpha1.Ensemble,
			Steps:      []v1alpha1.InferenceStep{model1, model2},
		}
		nodes[v1alpha1.GraphRootNodeName] = root
		return v1alpha1.InferenceGraphSpec{Nodes: nodes}
	default:
		t.Fatalf("unknown graph shape %q", shape)
	}
	root.Steps = []v1alpha1.InferenceStep{model1, model2}
	nodes[v1alpha1.GraphRootNodeName] = root
	return v1alpha1.InferenceGraphSpec{Nodes: nodes}
}

func TestV2HealthEndpointsAreServedByRouter(t *testing.T) {
	readyPath, livePath := constants.RouterV2HealthReadyEndpoint, constants.RouterV2HealthLiveEndpoint
	readyBody, liveBody := `{"ready":true}`, `{"live":true}`
	testCases := []struct {
		name         string
		graphShape   string
		method       string
		path         string
		expectedBody string // empty when the response carries no body
	}{
		{name: "sequence node", graphShape: string(v1alpha1.Sequence), method: http.MethodGet, path: readyPath, expectedBody: readyBody},
		{name: "splitter node", graphShape: string(v1alpha1.Splitter), method: http.MethodGet, path: readyPath, expectedBody: readyBody},
		{name: "ensemble node", graphShape: string(v1alpha1.Ensemble), method: http.MethodGet, path: readyPath, expectedBody: readyBody},
		{name: "switch node", graphShape: string(v1alpha1.Switch), method: http.MethodGet, path: readyPath, expectedBody: readyBody},
		{name: "nested nodes", graphShape: nestedGraphShape, method: http.MethodGet, path: readyPath, expectedBody: readyBody},
		{name: "server live endpoint", graphShape: string(v1alpha1.Ensemble), method: http.MethodGet, path: livePath, expectedBody: liveBody},
		// The HTTP server omits the body of a HEAD response, so only the status is checked.
		{name: "head request", graphShape: string(v1alpha1.Ensemble), method: http.MethodHead, path: readyPath},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			setShuttingDown(t, false)
			useGraph(t, healthTestGraph(t, testCase.graphShape, newUnexpectedStep(t), newUnexpectedStep(t)))

			recorder := httptest.NewRecorder()
			newTestMux().ServeHTTP(recorder, httptest.NewRequest(testCase.method, testCase.path, nil))

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
			if testCase.expectedBody != "" {
				assert.JSONEq(t, testCase.expectedBody, recorder.Body.String())
			}
		})
	}
}

func TestV2HealthEndpointsWhileShuttingDown(t *testing.T) {
	testCases := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "server live stays live while draining",
			path:           constants.RouterV2HealthLiveEndpoint,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"live":true}`,
		},
		{
			name:           "server ready reports not ready",
			path:           constants.RouterV2HealthReadyEndpoint,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody:   `{"error":"Router is not ready","cause":"shutting down"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			setShuttingDown(t, true)
			useGraph(t, healthTestGraph(t, string(v1alpha1.Ensemble), newUnexpectedStep(t), newUnexpectedStep(t)))

			recorder := httptest.NewRecorder()
			newTestMux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.path, nil))

			assert.Equal(t, testCase.expectedStatus, recorder.Code)
			assert.JSONEq(t, testCase.expectedBody, recorder.Body.String())
		})
	}
}

func TestRequestsOutsideHealthRoutesStillRunGraph(t *testing.T) {
	testCases := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/"},
		{method: http.MethodPost, path: constants.RouterV2HealthReadyEndpoint},
		{method: http.MethodPut, path: constants.RouterV2HealthReadyEndpoint},
		{method: http.MethodPost, path: constants.RouterV2HealthLiveEndpoint},
		{method: http.MethodPost, path: "/v1/models/model:predict"},
		{method: http.MethodPost, path: "/v2/models/model/infer"},
		{method: http.MethodGet, path: "/"},
		{method: http.MethodGet, path: constants.RouterV2HealthReadyEndpoint + "/"},
	}
	expectedResponse := map[string]interface{}{
		"model1": map[string]interface{}{"predictions": "1"},
		"model2": map[string]interface{}{"predictions": "2"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.method+" "+testCase.path, func(t *testing.T) {
			setShuttingDown(t, false)
			useGraph(t, healthTestGraph(t, string(v1alpha1.Ensemble), newPredictionStep(t, "1"), newPredictionStep(t, "2")))

			request := httptest.NewRequest(testCase.method, testCase.path, bytes.NewBufferString(`{"instances":["test","test2"]}`))
			recorder := httptest.NewRecorder()
			newTestMux().ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var response map[string]interface{}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, expectedResponse, response)
		})
	}
}

func TestReadinessProbeEndpointUnchanged(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		shuttingDown   bool
		expectedStatus int
		expectedBody   string
	}{
		{name: "GET while serving", method: http.MethodGet, expectedStatus: http.StatusOK},
		{name: "POST while serving", method: http.MethodPost, expectedStatus: http.StatusOK},
		{name: "GET while shutting down", method: http.MethodGet, shuttingDown: true, expectedStatus: http.StatusServiceUnavailable, expectedBody: "shutting down\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			setShuttingDown(t, testCase.shuttingDown)
			useGraph(t, healthTestGraph(t, string(v1alpha1.Ensemble), newUnexpectedStep(t), newUnexpectedStep(t)))

			recorder := httptest.NewRecorder()
			newTestMux().ServeHTTP(recorder, httptest.NewRequest(testCase.method, constants.RouterReadinessEndpoint, nil))

			assert.Equal(t, testCase.expectedStatus, recorder.Code)
			assert.Equal(t, testCase.expectedBody, recorder.Body.String())
		})
	}
}
