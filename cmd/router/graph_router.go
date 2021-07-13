package router

import (
	"bytes"
	"encoding/json"
	kfserving "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
)

func callService(serviceUrl *url.URL, input []byte, res chan<- string) error {
	log.Printf("calling url %v", serviceUrl)
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

func routeStep(currentStep kfserving.InferenceRouter, graph kfserving.InferenceGraphSpec, input []byte, res chan<- string){
	log.Printf("calling for %v", currentStep.NextRoutes)
	result := make(chan string)
	//For splitter and ABNTest call virtual service
	go callService(currentStep.Routes[0].ServiceUrl, input, result)
	response := <-result
	jobs := map[string]chan string{}
	for _, routeTo := range currentStep.NextRoutes {
		job := make(chan string)
		jobs[routeTo.NodeName] = job
		if router, ok := graph.Nodes[routeTo.NodeName]; ok {
			go routeStep(router, graph, []byte(response), job)
		}
	}
	combined := map[string]interface{}{}
	for name, result := range jobs {
		responseStr := <-result
		log.Printf("getting response back %v", responseStr)
		combined[name] = responseStr
	}
	log.Printf("finishing jobs for %v", currentStep.NextRoutes)
	if len(jobs) == 0 {
		log.Printf("no next routes")
		res <- response
		log.Printf("returning response")
	} else {
		log.Printf("writing response to chan")
		jsonRes, _ := json.Marshal(combined)
		res <- string(jsonRes)
		log.Printf("returning response %v", string(jsonRes))
	}
	log.Printf("finishing for %v", currentStep.NextRoutes)
}


func main() {

}
