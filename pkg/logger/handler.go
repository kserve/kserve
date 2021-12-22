/*
Copyright 2021 The KServe Authors.

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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/go-logr/logr"
	guuid "github.com/google/uuid"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"knative.dev/pkg/network"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type LoggerHandler struct {
	log              logr.Logger
	logUrl           *url.URL
	sourceUri        *url.URL
	logMode          v1beta1.LoggerType
	inferenceService string
	namespace        string
	component        string
	endpoint         string
	next             http.Handler
}

func New(logUrl *url.URL, sourceUri *url.URL, logMode v1beta1.LoggerType,
	inferenceService string, namespace string, endpoint string, component string, next http.Handler) http.Handler {
	logf.SetLogger(zap.New())
	return &LoggerHandler{
		log:              logf.Log.WithName("Logger"),
		logUrl:           logUrl,
		sourceUri:        sourceUri,
		logMode:          logMode,
		inferenceService: inferenceService,
		namespace:        namespace,
		component:        component,
		endpoint:         endpoint,
		next:             next,
	}
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
	if network.IsKubeletProbe(r) {
		if eh.next != nil {
			eh.next.ServeHTTP(w, r)
		}
		return
	}
	// Read Payload
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	// Get or Create an ID
	id := getOrCreateID(r)
	contentType := r.Header.Get("Content-Type")
	// log Request
	if eh.logMode == v1beta1.LogAll || eh.logMode == v1beta1.LogRequest {
		if err := QueueLogRequest(LogRequest{
			Url:              eh.logUrl,
			Bytes:            &body,
			ContentType:      contentType,
			ReqType:          InferenceRequest,
			Id:               id,
			SourceUri:        eh.sourceUri,
			InferenceService: eh.inferenceService,
			Namespace:        eh.namespace,
			Endpoint:         eh.endpoint,
			Component:        eh.component,
		}); err != nil {
			eh.log.Error(err, "Failed to log request")
		}
	}

	// Proxy Request
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	eh.next.ServeHTTP(rr, r)
	responseBody := rr.Body.Bytes()
	contentType = rr.Header().Get("Content-Type")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	// log response if OK
	if rr.Code == http.StatusOK {
		if eh.logMode == v1beta1.LogAll || eh.logMode == v1beta1.LogResponse {
			if err := QueueLogRequest(LogRequest{
				Url:              eh.logUrl,
				Bytes:            &responseBody,
				ContentType:      contentType,
				ReqType:          InferenceResponse,
				Id:               id,
				SourceUri:        eh.sourceUri,
				InferenceService: eh.inferenceService,
				Namespace:        eh.namespace,
				Endpoint:         eh.endpoint,
				Component:        eh.component,
			}); err != nil {
				eh.log.Error(err, "Failed to log response")
			}
		}
	} else {
		eh.log.Info("Failed to proxy request", "status code", rr.Code)
	}

	w.WriteHeader(rr.Code)
	_, err = w.Write(rr.Body.Bytes())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
