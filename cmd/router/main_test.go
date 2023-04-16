package main

import (
	"encoding/json"
	"fmt"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/stretchr/testify/assert"
	"io"
	"knative.dev/pkg/apis"
	"net/http"
	"net/http/httptest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
)

func init() {
	logf.SetLogger(zap.New())
}

func TestSimpleModelChainer(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		if err != nil {
			return
		}
		response := map[string]interface{}{"predictions": "1"}
		responseBytes, err := json.Marshal(response)
		_, err = rw.Write(responseBytes)
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
		response := map[string]interface{}{"predictions": "2"}
		responseBytes, err := json.Marshal(response)
		_, err = rw.Write(responseBytes)
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
		"instances": []string{
			"test",
			"test2",
		},
	}
	jsonBytes, _ := json.Marshal(input)
	headers := http.Header{
		"Authorization": {"Bearer Token"},
	}

	res, err := routeStep("root", graphSpec, jsonBytes, headers)
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	expectedResponse := map[string]interface{}{
		"predictions": "2",
	}
	fmt.Printf("final response:%v\n", response)
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
		_, err = rw.Write(responseBytes)
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
		response := map[string]interface{}{"predictions": "2"}
		responseBytes, err := json.Marshal(response)
		_, err = rw.Write(responseBytes)
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
	res, err := routeStep("root", graphSpec, jsonBytes, headers)
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	expectedResponse := map[string]interface{}{
		"model1": map[string]interface{}{
			"predictions": "1",
		},
		"model2": map[string]interface{}{
			"predictions": "2",
		},
	}
	fmt.Printf("final response:%v\n", response)
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
		_, err = rw.Write(responseBytes)
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
		_, err = rw.Write(responseBytes)
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
		_, err = rw.Write(responseBytes)
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
		_, err = rw.Write(responseBytes)
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
	res, err := routeStep("root", graphSpec, jsonBytes, headers)
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
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
	fmt.Printf("final response:%v\n", response)
	assert.Equal(t, expectedModel3Response, response["model3"])
	assert.Equal(t, expectedModel4Response, response["model4"])
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
		for _, h := range headersToPropagate {
			response[h] = req.Header[h][0]
		}
		responseBytes, err := json.Marshal(response)
		_, err = rw.Write(responseBytes)
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
	headersToPropagate = []string{}
	res, err := callService(model1Url.String(), jsonBytes, headers)
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	expectedResponse := map[string]interface{}{
		"predictions": "1",
	}
	fmt.Printf("final response:%v\n", response)
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
		for _, h := range headersToPropagate {
			response[h] = req.Header[h][0]
		}
		responseBytes, err := json.Marshal(response)
		_, err = rw.Write(responseBytes)
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
	headersToPropagate = []string{"Test-Header-Key"}
	res, err := callService(model1Url.String(), jsonBytes, headers)
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	expectedResponse := map[string]interface{}{
		"predictions":     "1",
		"Test-Header-Key": "Test-Header-Value",
	}
	fmt.Printf("final response:%v\n", response)
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
		for _, h := range headersToPropagate {
			response[h] = req.Header[h][0]
		}
		responseBytes, err := json.Marshal(response)
		_, err = rw.Write(responseBytes)
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
	headersToPropagate = []string{"Test-Header-Key", "Authorization"}
	res, err := callService(model1Url.String(), jsonBytes, headers)
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	expectedResponse := map[string]interface{}{
		"predictions":     "1",
		"Test-Header-Key": "Test-Header-Value",
		"Authorization":   "Bearer Token",
	}
	fmt.Printf("final response:%v\n", response)
	assert.Equal(t, expectedResponse, response)
}
