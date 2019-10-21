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

package inferencelogger

import (
	"bytes"
	"github.com/go-logr/logr"
	guuid "github.com/google/uuid"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
)

type loggerHandler struct {
	log       logr.Logger
	svcPort   string
	logUrl    *url.URL
	sourceUri *url.URL
	logType   v1alpha2.InferenceLoggerType
	sample    float64
	modelUri  *url.URL
}

func New(log logr.Logger, svcPort string, logUrl *url.URL, sourceUri *url.URL, logType v1alpha2.InferenceLoggerType, sample float64, modelUri *url.URL) http.Handler {
	return &loggerHandler{
		log:       log,
		svcPort:   svcPort,
		logUrl:    logUrl,
		sourceUri: sourceUri,
		logType:   logType,
		sample:    sample,
		modelUri:  modelUri,
	}
}

func (eh *loggerHandler) post(url *url.URL, body []byte, contentType string) ([]byte, error) {
	eh.log.Info("Calling server", "url", url.String(), "contentType", contentType)
	response, err := http.Post(url.String(), contentType, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if err = response.Body.Close(); err != nil {
		return nil, err
	}
	return b, nil
}

func (eh *loggerHandler) callService(b []byte, r *http.Request) ([]byte, string, error) {
	url := &url.URL{
		Scheme: "http",
		Host:   "0.0.0.0:" + eh.svcPort,
		Path:   r.URL.Path,
	}
	eh.log.Info("Calling server", "url", url.String())
	response, err := http.Post(url.String(), r.Header.Get("Content-Type"), bytes.NewReader(b))
	if err != nil {
		return nil, "", err
	}
	rb, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, "", err
	}
	if err = response.Body.Close(); err != nil {
		return nil, "", err
	}
	return rb, response.Header.Get("Content-Type"), nil
}

func getOrCreateID(r *http.Request) string {
	id := r.Header.Get("Ce-Id")
	if id == "" {
		id = guuid.New().String()
	}
	return id
}

// call svc and add send request/responses to logUrl
func (eh *loggerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read Payload
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		eh.log.Error(err, "Failed to read request payload")
	}

	emitEvent := true
	if eh.sample < 1.0 && rand.Float64() > eh.sample {
		eh.log.Info("Skipping emitting a log event")
		emitEvent = false
	}

	// Get or Create an ID
	id := getOrCreateID(r)

	// log Request
	if emitEvent && (eh.logType == v1alpha2.InferenceLogBoth || eh.logType == v1alpha2.InferenceLogRequest) {
		err = QueueLogRequest(LogRequest{
			url:         eh.logUrl,
			b:           &b,
			contentType: r.Header.Get("Content-Type"),
			reqType:     InferenceRequest,
			id:          id,
			sourceUri:   eh.sourceUri,
			modelUri:    eh.modelUri,
		})
		if err != nil {
			eh.log.Error(err, "Failed to log request")
		}
	}

	// Call service
	b, respContentType, err := eh.callService(b, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// log response
	if emitEvent && (eh.logType == v1alpha2.InferenceLogBoth || eh.logType == v1alpha2.InferenceLogresponse) {
		err = QueueLogRequest(LogRequest{
			url:         eh.logUrl,
			b:           &b,
			contentType: respContentType,
			reqType:     InferenceResponse,
			id:          id,
			sourceUri:   eh.sourceUri,
			modelUri:    eh.modelUri,
		})
		if err != nil {
			eh.log.Error(err, "Failed to log response")
		}
	}

	// Write final response
	if respContentType != "" {
		w.Header().Set("Content-Type", respContentType)
	}
	_, err = w.Write(b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
