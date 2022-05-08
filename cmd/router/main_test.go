package main

import (
	"encoding/json"
	"fmt"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"io/ioutil"
	"knative.dev/pkg/apis"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSimpleModelChainer(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := ioutil.ReadAll(req.Body)
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
		_, err := ioutil.ReadAll(req.Body)
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
				Routes: []v1alpha1.InferenceStep{
					{
						StepName: "model1",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceUrl: model1Url.String(),
						},
					},
					{
						StepName: "model2",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceUrl: model2Url.String(),
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
	result := make(chan []byte)
	go routeStep("root", graphSpec.Nodes["root"], graphSpec, jsonBytes, result)
	res := <-result
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	fmt.Printf("%v", response)
}

func TestSimpleModelEnsemble(t *testing.T) {
	// Start a local HTTP server
	model1 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := ioutil.ReadAll(req.Body)
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
		_, err := ioutil.ReadAll(req.Body)
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
				Routes: []v1alpha1.InferenceStep{
					{
						StepName: "model1",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceUrl: model1Url.String(),
						},
					},
					{
						StepName: "model2",
						InferenceTarget: v1alpha1.InferenceTarget{
							ServiceUrl: model2Url.String(),
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
	result := make(chan []byte)
	go routeStep("root", graphSpec.Nodes["root"], graphSpec, jsonBytes, result)
	res := <-result
	var response map[string]interface{}
	err = json.Unmarshal(res, &response)
	fmt.Printf("final response:%v", response)
}
