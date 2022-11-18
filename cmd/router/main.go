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
	"github.com/kserve/kserve/pkg/constants"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"math/rand"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	flag "github.com/spf13/pflag"
)

var log = logf.Log.WithName("InferenceGraphRouter")

func callService(serviceUrl string, input []byte, headers http.Header) ([]byte, int, error) {
	log.Info("Entered callService", "serviceUrl", serviceUrl)
	req, err := http.NewRequest("POST", serviceUrl, bytes.NewBuffer(input))
	for _, h := range headersToPropagate {
		if values, ok := headers[h]; ok {
			for _, v := range values {
				req.Header.Add(h, v)
			}
		}
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Error(err, "An error has occurred from service", "service", serviceUrl)
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err, "error while reading the response")
	}
	log.Info("Exiting callService", "serviceUrl", serviceUrl)
	return body, resp.StatusCode, err
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

func routeStep(nodeName string, graph v1alpha1.InferenceGraphSpec, input []byte, headers http.Header) ([]byte, int, error) {
	defer timeTrack(time.Now(), nodeName)
	currentNode := graph.Nodes[nodeName]

	if currentNode.RouterType == v1alpha1.Splitter {
		return executeStep(pickupRoute(currentNode.Steps), graph, input, headers)
	}
	if currentNode.RouterType == v1alpha1.Switch {
		route := pickupRouteByCondition(input, currentNode.Steps)
		if route == nil {
			// TODO Check what response to return if no matching route could be found. For now using 500
			// One idea is to return 400 saying no matching route was found. But check other 4xx too
			return input, 500, nil //TODO maybe should fail in this case?
		}
		return executeStep(route, graph, input, headers)
	}
	//if currentNode.RouterType == v1alpha1.Ensemble {
	//	ensembleRes := make([]chan map[string]interface{}, len(currentNode.Steps))
	//	errChan := make(chan error)
	//	for i := range currentNode.Steps {
	//		step := &currentNode.Steps[i]
	//		resultChan := make(chan map[string]interface{})
	//		ensembleRes[i] = resultChan
	//		go func() {
	//			output, err := executeStep(step, graph, input, headers)
	//			if err == nil {
	//				var res map[string]interface{}
	//				if err = json.Unmarshal(output, &res); err == nil {
	//					resultChan <- res
	//					return
	//				}
	//			}
	//			errChan <- err
	//		}()
	//	}
	//	// merge responses from parallel steps
	//	response := map[string]interface{}{}
	//	for i, resultChan := range ensembleRes {
	//		key := currentNode.Steps[i].StepName
	//		if key == "" {
	//			key = strconv.Itoa(i) // Use index if no step name
	//		}
	//		select {
	//		case response[key] = <-resultChan:
	//		case err := <-errChan:
	//			return nil, err
	//		}
	//	}
	//	return json.Marshal(response)
	//}
	if currentNode.RouterType == v1alpha1.Sequence {
		var statusCode int
		var responseBytes []byte
		var err error
		for i := range currentNode.Steps {
			step := &currentNode.Steps[i]
			request := input
			if step.Data == "$response" && i > 0 {
				request = responseBytes
			}

			if step.Condition != "" {
				if !gjson.ValidBytes(responseBytes) {
					return nil, 0, fmt.Errorf("invalid response")
				}
				// if the condition does not match for the step in the sequence we stop and return the response
				// TODO Check what response to return if condition at the very first node doesn't match
				// One idea is to return 400 saying no matching route was found. But check other 4xx too
				if !gjson.GetBytes(responseBytes, step.Condition).Exists() {
					return responseBytes, statusCode, nil
				}
			}
			if responseBytes, statusCode, err = executeStep(step, graph, request, headers); err != nil {
				return nil, 0, err
			}
			// Only if a step is a hard dependency, we will check for it's successfulness
			if step.Dependency == v1alpha1.Hard {
				log.Info("This step is a hard dependency")
				if !isSuccessFul(statusCode) {
					log.Info("And this hard dependency is unsuccessful", "statusCode", statusCode)
					// Stop the execution right away if step is hard and is unsuccessful
					return responseBytes, statusCode, nil
				}
			}
		}
		return responseBytes, statusCode, nil
	}
	log.Error(nil, "invalid route type", "type", currentNode.RouterType)
	return nil, 0, fmt.Errorf("invalid route type: %v", currentNode.RouterType)
}

func isSuccessFul(statusCode int) bool {
	if statusCode >= 200 && statusCode <= 299 {
		return true
	}
	return false
}

func executeStep(step *v1alpha1.InferenceStep, graph v1alpha1.InferenceGraphSpec, input []byte, headers http.Header) ([]byte, int, error) {
	if step.NodeName != "" {
		// when nodeName is specified make a recursive call for routing to next step
		return routeStep(step.NodeName, graph, input, headers)
	}
	return callService(step.ServiceURL, input, headers)
}

var inferenceGraph *v1alpha1.InferenceGraphSpec

func graphHandler(w http.ResponseWriter, req *http.Request) {
	inputBytes, _ := ioutil.ReadAll(req.Body)
	if response, statusCode, err := routeStep(v1alpha1.GraphRootNodeName, *inferenceGraph, inputBytes, req.Header); err != nil {
		log.Error(err, "failed to process request")
		w.WriteHeader(500) //TODO status code tbd
		w.Write([]byte(fmt.Sprintf("Failed to process request: %v", err)))
	} else {
		w.WriteHeader(statusCode)
		w.Write(response)
	}
}

var (
	jsonGraph          = flag.String("graph-json", "", "serialized json graph def")
	headersToPropagate = strings.Split(os.Getenv(constants.RouterHeadersPropagateEnvVar), ",")
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
