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
	"context"
	"crypto/rand"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
	"github.com/tidwall/gjson"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// _isInMesh is an auxiliary global variable for isInIstioMesh function.
var _isInMesh *bool

// isInIstioMesh checks if the InferenceGraph pod belongs to the mesh by
// checking the presence of the sidecar. It is known that when the sidecar
// is present, Envoy will be using port 15000 with standard HTTP. Thus, the
// presence of the sidecar is assumed if this port responds with an HTTP 200 status
// when doing a "GET /" request.
//
// The result of the check is cached in the _isInMesh global variable. Since a
// pod cannot be modified, a single check is enough for the whole life of the pod.
// The check cannot be done at start-up, because there is the possibility of
// false negatives, since there is no guarantee that the Istio sidecar has already
// started. So, the isInIstioMesh func should be used after the first inference
// request is received when it is guaranteed that the Istio sidecar is in ready state.
//
// Reference:
// - https://istio.io/latest/docs/ops/deployment/application-requirements/#ports-used-by-istio)
func isInIstioMesh() (bool, error) {
	if _isInMesh != nil {
		return *_isInMesh, nil
	}

	isInMesh := false
	client := http.Client{
		Timeout: time.Second * 3,
	}
	response, err := client.Get("http://localhost:15000")
	if err == nil {
		if response.StatusCode == http.StatusOK {
			isInMesh = true
		}
	} else if errors.Is(err, syscall.ECONNREFUSED) {
		// Assume no Istio sidecar. Thus, this pod is not
		// part of the mesh.
		err = nil
	}

	if response != nil && response.Body != nil {
		err = response.Body.Close()
	}

	_isInMesh = &isInMesh
	return *_isInMesh, err
}

func callService(serviceUrl string, input []byte, headers http.Header) ([]byte, int, error) {
	defer timeTrack(time.Now(), "step", serviceUrl)
	log.Info("Entering callService", "url", serviceUrl)

	parsedServiceUrl, parseServiceUrlErr := url.Parse(serviceUrl)
	if parseServiceUrlErr != nil {
		return nil, 500, parseServiceUrlErr
	}
	if parsedServiceUrl.Scheme == "https" {
		if isInMesh, isInMeshErr := isInIstioMesh(); isInMeshErr != nil {
			return nil, 500, isInMeshErr
		} else if isInMesh {
			// In this branch, it has been resolved that the Inference Graph is
			// part of the Istio mesh. In this case, even if the target service
			// is using HTTPS, it is better to use plain-text HTTP:
			// * If the target service is also part of the mesh, Istio will take
			//   care of properly applying TLS policies (e.g. mutual TLS).
			// * If the target service is _not_ part of the mesh, it still is better
			//   to let Istio manage TLS by configuring the sidecar to do TLS
			//   origination and prevent double TLS (see: https://istio.io/latest/docs/ops/common-problems/network-issues/#double-tls)
			//
			// If the Inference Graph is not part of the mesh, the indicated
			// schema is used.
			parsedServiceUrl.Scheme = "http"
			serviceUrl = parsedServiceUrl.String()

			log.Info("Using plain-text schema to let Istio manage TLS termination", "url", serviceUrl)
		}
	}

	req, err := http.NewRequest(http.MethodPost, serviceUrl, bytes.NewBuffer(input))
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

	var client *http.Client
	if routerTimeouts == nil || routerTimeouts.ServiceClient == nil {
		client = http.DefaultClient
	} else {
		client = &http.Client{
			Timeout: time.Duration(*routerTimeouts.ServiceClient) * time.Second,
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Error(err, "An error has occurred while calling service", "service", serviceUrl)
		return nil, 500, err
	}

	defer func() {
		if resp.Body != nil {
			err := resp.Body.Close()
			if err != nil {
				log.Error(err, "An error has occurred while closing the response body")
			}
		}
	}()

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
					return nil, 500, errors.New("invalid response")
				}
				// if the condition does not match for the step in the sequence we stop and return the response
				if !gjson.GetBytes(responseBytes, step.Condition).Exists() {
					return responseBytes, 200, nil
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

func getTimeout(value, defaultValue *int64) *int64 {
	if value != nil {
		return value
	}
	return defaultValue
}

func initTimeouts(graph v1alpha1.InferenceGraphSpec) {
	defaultServerRead := int64(constants.RouterTimeoutsServerRead)
	defaultServerWrite := int64(constants.RouterTimeoutServerWrite)
	defaultServerIdle := int64(constants.RouterTimeoutServerIdle)

	timeouts := &v1alpha1.InfereceGraphRouterTimeouts{
		ServerRead:    &defaultServerRead,
		ServerWrite:   &defaultServerWrite,
		ServerIdle:    &defaultServerIdle,
		ServiceClient: nil,
	}

	if graph.RouterTimeouts != nil {
		timeouts.ServerRead = getTimeout(graph.RouterTimeouts.ServerRead, &defaultServerRead)
		timeouts.ServerWrite = getTimeout(graph.RouterTimeouts.ServerWrite, &defaultServerWrite)
		timeouts.ServerIdle = getTimeout(graph.RouterTimeouts.ServerIdle, &defaultServerIdle)
		timeouts.ServiceClient = getTimeout(graph.RouterTimeouts.ServiceClient, nil)
	}

	routerTimeouts = timeouts
}

// Mainly used for kubernetes readiness probe. It responds with "503 shutting down" if server is shutting down,
// otherwise returns "200 OK".
func readyHandler(w http.ResponseWriter, req *http.Request) {
	if isShuttingDown {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

var (
	jsonGraph                                           = flag.String("graph-json", "", "serialized json graph def")
	inferenceGraph         *v1alpha1.InferenceGraphSpec = nil
	compiledHeaderPatterns []*regexp.Regexp
	isShuttingDown                                               = false
	drainSleepDuration                                           = 30 * time.Second
	routerTimeouts         *v1alpha1.InfereceGraphRouterTimeouts = nil
	log                                                          = logf.Log.WithName("InferenceGraphRouter")
	signalChan                                                   = make(chan os.Signal, 1)
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
	initTimeouts(*inferenceGraph)

	http.HandleFunc("/", graphHandler)
	http.HandleFunc(constants.RouterReadinessEndpoint, readyHandler)

	server := &http.Server{
		Addr:         ":" + strconv.Itoa(constants.RouterPort),
		Handler:      nil,                                                      // default server mux
		ReadTimeout:  time.Duration(*routerTimeouts.ServerRead) * time.Second,  // set the maximum duration for reading the entire request, including the body
		WriteTimeout: time.Duration(*routerTimeouts.ServerWrite) * time.Second, // set the maximum duration before timing out writes of the response
		IdleTimeout:  time.Duration(*routerTimeouts.ServerIdle) * time.Second,  // set the maximum amount of time to wait for the next request when keep-alives are enabled
	}

	go func() {
		err = server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(err, fmt.Sprintf("Failed to serve on address %v", server.Addr))
			os.Exit(1)
		}
	}()

	// Blocks until SIGTERM or SIGINT is received
	handleSignals(server)
}

func handleSignals(server *http.Server) {
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	sig := <-signalChan
	log.Info("Received shutdown signal", "signal", sig)
	// Fail the readiness probe
	isShuttingDown = true
	log.Info(fmt.Sprintf("Sleeping %v to allow K8s propagation of non-ready state", drainSleepDuration))
	// Sleep to give networking a little bit more time to remove the pod
	// from its configuration and propagate that to all loadbalancers and nodes.
	time.Sleep(drainSleepDuration)
	// Shut down the server gracefully
	if err := server.Shutdown(context.Background()); err != nil {
		log.Error(err, "Failed to shutdown the server gracefully")
		os.Exit(1)
	}
	log.Info("Server gracefully shutdown")
}
