/*
Copyright 2019 kubeflow.org.

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

package logger

import (
	"bytes"
	"fmt"
	"github.com/go-logr/logr"
	guuid "github.com/google/uuid"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"io/ioutil"
	"net/http"
	"net/url"
)

type LoggerHandler struct {
	log       logr.Logger
	svcHost   string
	svcPort   string
	logUrl    *url.URL
	sourceUri *url.URL
	logMode   v1alpha2.LoggerMode
	modelId   string
	namespace string
	endpoint  string
}

func New(log logr.Logger, svcHost string, svcPort string, logUrl *url.URL, sourceUri *url.URL, logMode v1alpha2.LoggerMode, modelId string, namespace string, endpoint string) http.Handler {
	return &LoggerHandler{
		log:       log,
		svcHost:   svcHost,
		svcPort:   svcPort,
		logUrl:    logUrl,
		sourceUri: sourceUri,
		logMode:   logMode,
		modelId:   modelId,
		namespace: namespace,
		endpoint:  endpoint,
	}
}

func (eh *LoggerHandler) callService(b []byte, r *http.Request) ([]byte, *string, *int, error) {
	url := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%s", eh.svcHost, eh.svcPort),
		Path:   r.URL.Path,
	}
	eh.log.Info("Calling server", "url", url.String())
	response, err := http.Post(url.String(), r.Header.Get("Content-Type"), bytes.NewReader(b))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("while calling post: %s", err)
	}
	rb, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("while reading response body: %s", err)
	}
	if err := response.Body.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("while closing response body: %s", err)
	}
	contentType := response.Header.Get("Content-Type")
	statusCode := response.StatusCode
	return rb, &contentType, &statusCode, nil
}

func getOrCreateID(r *http.Request) string {
	id := r.Header.Get(CloudEventsIdHeader)
	if id == "" {
		id = guuid.New().String()
	}
	return id
}

// call svc and add send request/responses to logUrl
func (eh *LoggerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read Payload
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		eh.log.Error(err, "Failed to read request payload")
	}

	// Get or Create an ID
	id := getOrCreateID(r)

	// log Request
	if eh.logMode == v1alpha2.LogAll || eh.logMode == v1alpha2.LogRequest {
		if err := QueueLogRequest(LogRequest{
			Url:         eh.logUrl,
			Bytes:       &b,
			ContentType: "application/json", // Always JSON at present
			ReqType:     InferenceRequest,
			Id:          id,
			SourceUri:   eh.sourceUri,
			ModelId:     eh.modelId,
			Namespace:   eh.namespace,
			Endpoint:    eh.endpoint,
		}); err != nil {
			eh.log.Error(err, "Failed to log request")
		}
	}

	// Call service
	b, respContentType, statusCode, err := eh.callService(b, r)
	// Error in internal calling of service. Non 200 returns code from service will not cause an error.
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// log response if OK
	if *statusCode == http.StatusOK {
		if eh.logMode == v1alpha2.LogAll || eh.logMode == v1alpha2.LogResponse {
			if err := QueueLogRequest(LogRequest{
				Url:         eh.logUrl,
				Bytes:       &b,
				ContentType: "application/json", // Always JSON at present
				ReqType:     InferenceResponse,
				Id:          id,
				SourceUri:   eh.sourceUri,
				ModelId:     eh.modelId,
				Namespace:   eh.namespace,
				Endpoint:    eh.endpoint,
			}); err != nil {
				eh.log.Error(err, "Failed to log response")
			}
		}
	} else {
		eh.log.Info("Bad call to service.", "status code", *statusCode)
	}

	// Write final response
	if *respContentType != "" {
		w.Header().Set("Content-Type", *respContentType)
	}
	w.WriteHeader(*statusCode)
	_, err = w.Write(b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
