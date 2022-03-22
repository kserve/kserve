package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"math/rand"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	flag "github.com/spf13/pflag"
)

func callService(serviceUrl string, input []byte, res chan<- []byte) error {
	resp, err := http.Post(serviceUrl, "application/json", bytes.NewBuffer(input))
	if err != nil {
		log.Fatalf("An error has occured %v", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("error while reading the response %v", err)
		return err
	}
	res <- body
	return nil
}

func pickupRoute(routes []v1alpha1.InferenceRoute) *v1alpha1.InferenceRoute {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	//generate num [0,100)
	point := r.Intn(99)
	start, end := 0, 0

	for _, route := range routes {
		start += end
		end += int(*route.Weight)
		if point >= start && point < end {
			return &route
		}
	}
	return nil
}

func routeStep(nodeName string, currentStep v1alpha1.InferenceRouter, graph v1alpha1.InferenceGraphSpec, input []byte, res chan<- []byte) error {
	log.Printf("current step %v", nodeName)
	response := map[string]interface{}{}
	//For splitter and ABNTest call virtual service
	if currentStep.RouterType == v1alpha1.Splitter {
		route := pickupRoute(currentStep.Routes)
		if route == nil {
			return fmt.Errorf("The pickuped servcie is nil.")
		}
		result := make(chan []byte)
		go callService(route.ServiceUrl, input, result)
		responseBytes := <-result
		var res map[string]interface{}
		json.Unmarshal(responseBytes, &res)
		response = res
	} else if currentStep.RouterType == v1alpha1.Switch {

	} else if currentStep.RouterType == v1alpha1.Ensemble {
		ensembleRes := map[string]chan []byte{}
		for i := range currentStep.Routes {
			res := make(chan []byte)
			ensembleRes[currentStep.Routes[i].ServiceUrl] = res
			go callService(currentStep.Routes[i].ServiceUrl, input, res)
		}

		for name, result := range ensembleRes {
			responseBytes := <-result

			var res map[string]interface{}
			json.Unmarshal(responseBytes, &res)
			log.Printf("getting response back %v", res)
			response[name] = res
		}
	} else { //routeType == Single
		result := make(chan []byte)
		go callService(currentStep.Routes[0].ServiceUrl, input, result)
		responseBytes := <-result
		var res map[string]interface{}
		json.Unmarshal(responseBytes, &res)
		response = res
	}
	jsonRes, err := json.Marshal(response)
	if err != nil {
		return err
	}

	if len(currentStep.NextRoutes) == 0 {
		log.Printf("no next routes")
		res <- jsonRes
		return nil
	}
	// process outgoing edges
	jobs := map[string]chan []byte{}
	for _, routeTo := range currentStep.NextRoutes {
		job := make(chan []byte)
		jobs[routeTo.NodeName] = job
		if router, ok := graph.Nodes[routeTo.NodeName]; ok {
			if routeTo.Data == "$request" {
				go routeStep(routeTo.NodeName, router, graph, input, job)
			} else {
				go routeStep(routeTo.NodeName, router, graph, jsonRes, job)
			}
		}
	}
	responseForNextRoutes := map[string]interface{}{}
	for name, result := range jobs {
		responseBytes := <-result
		var res map[string]interface{}
		json.Unmarshal(responseBytes, &res)
		log.Printf("getting response back for %s: %v", name, res)

		responseForNextRoutes[name] = res
	}

	jsonResNext, err := json.Marshal(responseForNextRoutes)
	if err != nil {
		return err
	}
	res <- jsonResNext
	log.Printf("returning response %v", string(jsonResNext))

	return nil
}

var inferenceGraph *v1alpha1.InferenceGraphSpec

func graphHandler(w http.ResponseWriter, req *http.Request) {
	inputBytes, _ := ioutil.ReadAll(req.Body)
	res := make(chan []byte)
	rootNodes := []string{}
	for name, _ := range inferenceGraph.Nodes {
		rootNodes = append(rootNodes, name)
	}
	go routeStep(rootNodes[0], inferenceGraph.Nodes[rootNodes[0]], *inferenceGraph, inputBytes, res)
	response := <-res
	w.Write(response)
}

var (
	jsonGraph = flag.String("graph-json", "", "serialized json graph def")
)

func main() {
	flag.Parse()
	inferenceGraph = &v1alpha1.InferenceGraphSpec{}
	err := json.Unmarshal([]byte(*jsonGraph), inferenceGraph)
	if err != nil {
		log.Fatalf("failed to unmarshall inference graph json %v", err)
		os.Exit(1)
	}

	http.HandleFunc("/", graphHandler)

	log.Fatal(http.ListenAndServe(":8080", nil))
}
