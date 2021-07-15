package router

import (
	"bytes"
	"encoding/json"
	v1alpha1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"io/ioutil"
	"knative.dev/pkg/apis"
	"log"
	"net/http"
)

func callService(serviceUrl *apis.URL, input []byte, res chan<- string) error {
	resp, err := http.Post(serviceUrl.String(), "application/json", bytes.NewBuffer(input))
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
	res <- string(body)
	return nil
}

func routeStep(nodeName string, currentStep v1alpha1.InferenceRouter, graph v1alpha1.InferenceGraphSpec, input []byte, res chan<- string) error {
	log.Printf("current step %v", nodeName)
	response := map[string]interface{}{}
	//For splitter and ABNTest call virtual service
	if currentStep.RouterType == v1alpha1.Splitter {
	} else if currentStep.RouterType == v1alpha1.Ensemble {
		ensembleRes := map[string]chan string{}
		for i := range currentStep.Routes {
			res := make(chan string)
			ensembleRes[currentStep.Routes[i].ServiceUrl.Host] = res
			go callService(currentStep.Routes[i].ServiceUrl, input, res)
		}

		for name, result := range ensembleRes {
			responseStr := <-result
			log.Printf("getting response back %v", responseStr)
			response[name] = responseStr
		}
	} else {
		result := make(chan string)
		go callService(currentStep.Routes[0].ServiceUrl, input, result)
		res := <- result
		response[currentStep.Routes[0].ServiceUrl.Host] = res
	}
	jsonRes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if len(currentStep.NextRoutes) == 0 {
		log.Printf("no next routes")
		res <- string(jsonRes)
		return nil
	}
	// process outgoing edges
	jobs := map[string]chan string{}
	for _, routeTo := range currentStep.NextRoutes {
		job := make(chan string)
		jobs[routeTo.NodeName] = job
		if router, ok := graph.Nodes[routeTo.NodeName]; ok {
			go routeStep(routeTo.NodeName, router, graph, jsonRes, job)
		}
	}
	responseForNextRoutes := map[string]interface{}{}
	for name, result := range jobs {
		responseStr := <-result
		log.Printf("getting response back for %s: %v", name, responseStr)
		responseForNextRoutes[name] = responseStr
	}

	jsonResNext, err := json.Marshal(responseForNextRoutes)
	if err != nil {
		return err
	}
	res <- string(jsonRes)
	log.Printf("returning response %v", string(jsonResNext))

	return nil
}

func main() {

}
