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
	goerrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/pkg/errors"

	"github.com/tidwall/gjson"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"crypto/rand"
	"math/big"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	flag "github.com/spf13/pflag"
)

var log = logf.Log.WithName("InferenceGraphRouter")

func callService(serviceUrl string, input []byte, headers http.Header) ([]byte, int, error) {
	defer timeTrack(time.Now(), "step", serviceUrl)
	log.Info("Entering callService", "url", serviceUrl)
	req, err := http.NewRequest("POST", serviceUrl, bytes.NewBuffer(input))
	if err != nil {
		log.Error(err, "An error occurred while preparing request object with serviceUrl.", "serviceUrl", serviceUrl)
		return nil, 500, err
	}

	// To avoid headers matched more than one time which will lead to duplication of header values
	matchedHeaders := map[string]bool{}
	var headersToPropagate []string
	for _, p := range compiledHeaderPatterns {
		for h, values := range headers {
			if _, ok := matchedHeaders[h]; !ok && p.MatchString(h) {
				matchedHeaders[h] = true
				headersToPropagate = append(headersToPropagate, h)
				for _, v := range values {
					req.Header.Add(h, v)
				}
			}
		}
	}
	log.Info("These headers will be propagated by the router to all the steps", "headers", headersToPropagate)
	if val := req.Header.Get("Content-Type"); val == "" {
		req.Header.Add("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Error(err, "An error has occurred while calling service", "service", serviceUrl)
		return nil, 500, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error(err, "An error has occurred while closing the response body")
		}
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err, "Error while reading the response")
	}
	return body, resp.StatusCode, err
}

func pickupRoute(routes []v1alpha1.InferenceStep) *v1alpha1.InferenceStep {
	randomNumber, err := rand.Int(rand.Reader, big.NewInt(101))
	if err != nil {
		panic(err)
	}
	// generate num [0,100)
	point := int(randomNumber.Int64())
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

func timeTrack(start time.Time, nodeOrStep string, name string) {
	elapsed := time.Since(start)
	log.Info("elapsed time", nodeOrStep, name, "time", elapsed)
}

type EnsembleStepOutput struct {
	StepResponse   map[string]interface{}
	StepStatusCode int
}

// See if reviewer suggests a better name for this function
func handleSplitterORSwitchNode(route *v1alpha1.InferenceStep, graph v1alpha1.InferenceGraphSpec, input []byte, headers http.Header) ([]byte, int, error) {
	var statusCode int
	var responseBytes []byte
	var err error
	stepType := "serviceUrl"
	if route.NodeName != "" {
		stepType = "node"
	}
	log.Info("Starting execution of step", "type", stepType, "stepName", route.StepName)
	if responseBytes, statusCode, err = executeStep(route, graph, input, headers); err != nil {
		return nil, 500, err
	}

	if route.Dependency == v1alpha1.Hard && !isSuccessFul(statusCode) {
		log.Info("This step is a hard dependency and it is unsuccessful", "stepName", route.StepName, "statusCode", statusCode)
	}
	return responseBytes, statusCode, nil
}

func routeStep(nodeName string, graph v1alpha1.InferenceGraphSpec, input []byte, headers http.Header) ([]byte, int, error) {
	defer timeTrack(time.Now(), "node", nodeName)
	currentNode := graph.Nodes[nodeName]

	if currentNode.RouterType == v1alpha1.Splitter {
		route := pickupRoute(currentNode.Steps)
		return handleSplitterORSwitchNode(route, graph, input, headers)
	}
	if currentNode.RouterType == v1alpha1.Switch {
		var err error
		route := pickupRouteByCondition(input, currentNode.Steps)
		if route == nil {
			errorMessage := "None of the routes matched with the switch condition"
			err = errors.New(errorMessage)
			log.Error(err, errorMessage)
			return nil, 404, err
		}
		return handleSplitterORSwitchNode(route, graph, input, headers)
	}
	if currentNode.RouterType == v1alpha1.Ensemble {
		ensembleRes := make([]chan EnsembleStepOutput, len(currentNode.Steps))
		errChan := make(chan error)
		for i := range currentNode.Steps {
			step := &currentNode.Steps[i]
			stepType := "serviceUrl"
			if step.NodeName != "" {
				stepType = "node"
			}
			log.Info("Starting execution of step", "type", stepType, "stepName", step.StepName)
			resultChan := make(chan EnsembleStepOutput)
			ensembleRes[i] = resultChan
			go func() {
				output, statusCode, err := executeStep(step, graph, input, headers)
				if err == nil {
					var res map[string]interface{}
					if err = json.Unmarshal(output, &res); err == nil {
						resultChan <- EnsembleStepOutput{
							StepResponse:   res,
							StepStatusCode: statusCode,
						}
						return
					}
				}
				errChan <- err
			}()
		}
		// merge responses from parallel steps
		response := map[string]interface{}{}
		ensembleStepOutput := EnsembleStepOutput{}
		for i, resultChan := range ensembleRes {
			key := currentNode.Steps[i].StepName
			if key == "" {
				key = strconv.Itoa(i) // Use index if no step name
			}
			select {
			case ensembleStepOutput = <-resultChan:
				if !isSuccessFul(ensembleStepOutput.StepStatusCode) && currentNode.Steps[i].Dependency == v1alpha1.Hard {
					log.Info("This step is a hard dependency and it is unsuccessful", "stepName", currentNode.Steps[i].StepName, "statusCode", ensembleStepOutput.StepStatusCode)
					stepResponse, _ := json.Marshal(ensembleStepOutput.StepResponse) // TODO check if you need err handling for Marshalling
					return stepResponse, ensembleStepOutput.StepStatusCode, nil      // First failed hard dependency will decide the response and response code for ensemble node
				} else {
					response[key] = ensembleStepOutput.StepResponse
				}
			case err := <-errChan:
				return nil, 500, err
			}
		}
		// return json.Marshal(response)
		combinedResponse, _ := json.Marshal(response) // TODO check if you need err handling for Marshalling
		return combinedResponse, 200, nil
	}
	if currentNode.RouterType == v1alpha1.Sequence {
		var statusCode int
		var responseBytes []byte
		var err error
		for i := range currentNode.Steps {
			step := &currentNode.Steps[i]
			stepType := "serviceUrl"
			if step.NodeName != "" {
				stepType = "node"
			}
			log.Info("Starting execution of step", "type", stepType, "stepName", step.StepName)

			request := input
			if step.Data == "$response" && i > 0 {
				request = responseBytes
			}

			if step.Condition != "" {
				if !gjson.ValidBytes(responseBytes) {
					return nil, 500, fmt.Errorf("invalid response")
				}
				// if the condition does not match for the step in the sequence we stop and return the response
				if !gjson.GetBytes(responseBytes, step.Condition).Exists() {
					return responseBytes, 500, nil
				}
			}
			if responseBytes, statusCode, err = executeStep(step, graph, request, headers); err != nil {
				return nil, 500, err
			}
			/*
			   Only if a step is a hard dependency, we will check for its success.
			*/
			if step.Dependency == v1alpha1.Hard {
				if !isSuccessFul(statusCode) {
					log.Info("This step is a hard dependency and it is unsuccessful", "stepName", step.StepName, "statusCode", statusCode)
					// Stop the execution of sequence right away if step is a hard dependency and is unsuccessful
					return responseBytes, statusCode, nil
				}
			}
		}

		return responseBytes, statusCode, nil
	}
	log.Error(nil, "invalid route type", "type", currentNode.RouterType)
	return nil, 500, fmt.Errorf("invalid route type: %v", currentNode.RouterType)
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

func prepareErrorResponse(err error, errorMessage string) []byte {
	igRoutingErr := &InferenceGraphRoutingError{
		errorMessage,
		fmt.Sprintf("%v", err),
	}
	errorResponseBytes, err := json.Marshal(igRoutingErr)
	if err != nil {
		log.Error(err, "marshalling error")
	}
	return errorResponseBytes
}

var inferenceGraph *v1alpha1.InferenceGraphSpec

func graphHandler(w http.ResponseWriter, req *http.Request) {
	inputBytes, _ := io.ReadAll(req.Body)
	if response, statusCode, err := routeStep(v1alpha1.GraphRootNodeName, *inferenceGraph, inputBytes, req.Header); err != nil {
		log.Error(err, "failed to process request")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if _, err := w.Write(prepareErrorResponse(err, "Failed to process request")); err != nil {
			log.Error(err, "failed to write graphHandler response")
		}
	} else {
		if json.Valid(response) {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(statusCode)
		if _, err := w.Write(response); err != nil {
			log.Error(err, "failed to write graphHandler response")
		}
	}
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	var allErrors []error
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		c, err := regexp.Compile(p)
		if err != nil {
			allErrors = append(allErrors, errors.Wrap(err, fmt.Sprintf("failed to compile pattern %q", p)))
		} else {
			compiled = append(compiled, c)
		}
	}
	return compiled, goerrors.Join(allErrors...)
}

var (
	jsonGraph              = flag.String("graph-json", "", "serialized json graph def")
	compiledHeaderPatterns []*regexp.Regexp
)

func main() {
	flag.Parse()
	logf.SetLogger(zap.New())
	if headersToPropagateEnvVar, ok := os.LookupEnv(constants.RouterHeadersPropagateEnvVar); ok {
		var err error
		log.Info("The headers that will match these patterns will be propagated by the router to all the steps",
			"headersToPropagateEnvVar", headersToPropagateEnvVar)
		compiledHeaderPatterns, err = compilePatterns(strings.Split(headersToPropagateEnvVar, ","))
		if err != nil {
			log.Error(err, "Failed to compile some header patterns")
		}
	}
	inferenceGraph = &v1alpha1.InferenceGraphSpec{}
	err := json.Unmarshal([]byte(*jsonGraph), inferenceGraph)
	if err != nil {
		log.Error(err, "failed to unmarshall inference graph json")
		os.Exit(1)
	}

	http.HandleFunc("/", graphHandler)

	server := &http.Server{
		Addr:         ":8080",                        // specify the address and port
		Handler:      http.HandlerFunc(graphHandler), // specify your HTTP handler
		ReadTimeout:  time.Minute,                    // set the maximum duration for reading the entire request, including the body
		WriteTimeout: time.Minute,                    // set the maximum duration before timing out writes of the response
		IdleTimeout:  3 * time.Minute,                // set the maximum amount of time to wait for the next request when keep-alives are enabled
	}
	err = server.ListenAndServe()

	if err != nil {
		log.Error(err, "failed to listen on 8080")
		os.Exit(1)
	}
}
