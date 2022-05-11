/*
Copyright 2022 The KServe Authors.

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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/tidwall/gjson"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"math/rand"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	flag "github.com/spf13/pflag"
)

var log = logf.Log.WithName("InferenceGraphRouter")

func callService(serviceUrl string, input []byte) ([]byte, error) {
	resp, err := http.Post(serviceUrl, "application/json", bytes.NewBuffer(input))
	if err != nil {
		log.Error(err, "An error has occurred from service", "service", serviceUrl)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err, "error while reading the response")
	}
	return body, err
}

func pickupRoute(routes []v1alpha1.InferenceStep) *v1alpha1.InferenceStep {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	//generate num [0,100)
	point := r.Intn(99)
	end := 0
	for _, route := range routes {
		end += int(*route.Weight)
		if point < end {
			return &route
		}
	}
	return nil
}

func pickupRouteByCondition(input []byte, routes []v1alpha1.InferenceStep) *v1alpha1.InferenceStep {
	if !gjson.ValidBytes(input) {
		return nil
	}
	for _, route := range routes {
		if gjson.GetBytes(input, route.Condition).Exists() {
			return &route
		}
	}
	return nil
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Info("elapsed time", "node", name, "time", elapsed)
}

func routeStep(nodeName string, graph v1alpha1.InferenceGraphSpec, input []byte) ([]byte, error) {
	log.Info("current step", "nodeName", nodeName)
	defer timeTrack(time.Now(), nodeName)
	currentNode := graph.Nodes[nodeName]

	if currentNode.RouterType == v1alpha1.Splitter {
		return executeStep(pickupRoute(currentNode.Routes), graph, input)
	}
	if currentNode.RouterType == v1alpha1.Switch {
		route := pickupRouteByCondition(input, currentNode.Routes)
		if route == nil {
			return input, nil //TODO maybe should fail in this case?
		}
		return executeStep(route, graph, input)
	}
	if currentNode.RouterType == v1alpha1.Ensemble {
		ensembleRes := make([]chan map[string]interface{}, len(currentNode.Routes))
		errChan := make(chan error)
		for i := range currentNode.Routes {
			step := &currentNode.Routes[i]
			resultChan := make(chan map[string]interface{})
			ensembleRes[i] = resultChan
			go func() {
				output, err := executeStep(step, graph, input)
				if err == nil {
					var res map[string]interface{}
					if err = json.Unmarshal(output, &res); err == nil {
						resultChan <- res
						return
					}
				}
				errChan <- err
			}()
		}
		// merge responses from parallel steps
		response := map[string]interface{}{}
		for i, resultChan := range ensembleRes {
			key := currentNode.Routes[i].StepName
			if key == "" {
				key = strconv.Itoa(i) // Use index if no step name
			}
			select {
			case response[key] = <-resultChan:
			case err := <-errChan:
				return nil, err
			}
		}
		return json.Marshal(response)
	}
	if currentNode.RouterType == v1alpha1.Sequence {
		var responseBytes []byte
		var err error
		for i := range currentNode.Routes {
			step := &currentNode.Routes[i]
			request := input
			if step.Data == "$response" && i > 0 {
				request = responseBytes
			}
			if responseBytes, err = executeStep(step, graph, request); err != nil {
				return nil, err
			}
		}
		return responseBytes, nil
	}
	log.Error(nil, "invalid route type", "type", currentNode.RouterType)
	return nil, fmt.Errorf("invalid route type: %v", currentNode.RouterType)
}

func executeStep(step *v1alpha1.InferenceStep, graph v1alpha1.InferenceGraphSpec, input []byte) ([]byte, error) {
	if step.NodeName != "" {
		// when nodeName is specified make a recursive call for routing to next step
		return routeStep(step.NodeName, graph, input)
	}
	return callService(step.ServiceUrl, input)
}

var inferenceGraph *v1alpha1.InferenceGraphSpec

func graphHandler(w http.ResponseWriter, req *http.Request) {
	inputBytes, _ := ioutil.ReadAll(req.Body)
	if response, err := routeStep(v1alpha1.GraphRootNodeName, *inferenceGraph, inputBytes); err != nil {
		log.Error(err, "failed to process request")
		w.WriteHeader(500) //TODO status code tbd
		w.Write([]byte(fmt.Sprintf("Failed to process request: %v", err)))
	} else {
		w.Write(response)
	}
}

var (
	jsonGraph = flag.String("graph-json", "", "serialized json graph def")
)

func main() {
	flag.Parse()
	logf.SetLogger(zap.New())
	inferenceGraph = &v1alpha1.InferenceGraphSpec{}
	err := json.Unmarshal([]byte(*jsonGraph), inferenceGraph)
	if err != nil {
		log.Error(err, "failed to unmarshall inference graph json")
		os.Exit(1)
	}

	http.HandleFunc("/", graphHandler)

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Error(err, "failed to listen on 8080")
		os.Exit(1)
	}
}
