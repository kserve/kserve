package router

import (
	"encoding/json"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	model1Url, err := url.Parse(model1.URL)
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
	model2Url, err := url.Parse(model2.URL)
	if err != nil {
		t.Fatalf("Failed to parse model url")
	}
	defer model2.Close()

	graphSpec := v1alpha1.InferenceGraphSpec{
		Nodes: map[string]v1alpha1.InferenceRouter{
			"root": {
				RouterType: v1alpha1.Splitter,
				Routes: []v1alpha1.InferenceRoute{
					{
						ServiceUrl: model1Url,
					},
				},
				NextRoutes: []v1alpha1.RouteTo{
					{
						NodeName: "model2",
						Data:     "$response",
					},
				},
			},
			"model2": {
				RouterType: v1alpha1.Splitter,
				Routes: []v1alpha1.InferenceRoute{
					{
						ServiceUrl: model2Url,
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
	result := make(chan string)
	go routeStep("root", graphSpec.Nodes["root"], graphSpec, jsonBytes, result)
	res := <-result
	print(res)
}
