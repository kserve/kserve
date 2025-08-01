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
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/url"
	"slices"

	"github.com/go-logr/logr"
	guuid "github.com/google/uuid"
	"knative.dev/pkg/network"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// loggingResponseWriter is a wrapper around an http.ResponseWriter that logs the response body
// It implements the http.ResponseWriter and http.Flusher interfaces
type loggingResponseWriter struct {
	http.ResponseWriter // the original http.ResponseWriter
	http.Flusher
	statusCode     int
	responseBuffer *bytes.Buffer // buffer to store the response body for logging
	log            logr.Logger
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.responseBuffer.Write(b)
	if err != nil {
		w.log.Error(err, "Failed to write response buffer")
		return n, err
	}
	n, err = w.ResponseWriter.Write(b)
	if err != nil {
		w.log.Error(err, "Failed to write response")
		return n, err
	}
	return n, nil
}

func (w *loggingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *loggingResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *loggingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

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
	metadataHeaders  []string
	annotations      map[string]string
	certName         string
	tlsSkipVerify    bool
}

func New(logUrl *url.URL, sourceUri *url.URL, logMode v1beta1.LoggerType,
	inferenceService string, namespace string, endpoint string, component string, next http.Handler, metadataHeaders []string,
	certName string, annotations map[string]string, tlsSkipVerify bool,
) http.Handler {
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
		annotations:      annotations,
		metadataHeaders:  metadataHeaders,
		certName:         certName,
		tlsSkipVerify:    tlsSkipVerify,
	}
}

// call svc and add send request/responses to logUrl
func (eh *LoggerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if network.IsKubeletProbe(r) {
		if eh.next != nil {
			eh.next.ServeHTTP(w, r)
		}
		return
	}
	// Read request payload
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	metadata := map[string][]string{}
	if eh.metadataHeaders != nil {
		// Loop over header names
		for name, values := range r.Header {
			// Loop over all values for the name.
			if slices.Contains(eh.metadataHeaders, name) {
				metadata[name] = values
			}
		}
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
			ReqType:          CEInferenceRequest,
			Id:               id,
			SourceUri:        eh.sourceUri,
			InferenceService: eh.inferenceService,
			Namespace:        eh.namespace,
			Endpoint:         eh.endpoint,
			Component:        eh.component,
			Annotations:      eh.annotations,
			Metadata:         metadata,
			CertName:         eh.certName,
			TlsSkipVerify:    eh.tlsSkipVerify,
		}); err != nil {
			eh.log.Error(err, "Failed to log request")
		}
	}

	// Proxy Request
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	// TODO: Set a reasonable initial buffer size
	var responseBuf bytes.Buffer
	lrw := &loggingResponseWriter{ResponseWriter: w, responseBuffer: &responseBuf, log: eh.log}
	eh.next.ServeHTTP(lrw, r)
	// Read the response body from the buffer
	reader := bufio.NewReader(lrw.responseBuffer)
	responseBody, err := io.ReadAll(reader)
	if err != nil {
		eh.log.Error(err, "Failed to read response body")
	}
	// log Response
	if lrw.statusCode == http.StatusOK {
		if eh.logMode == v1beta1.LogAll || eh.logMode == v1beta1.LogResponse {
			if err := QueueLogRequest(LogRequest{
				Url:              eh.logUrl,
				Bytes:            &responseBody,
				ContentType:      contentType,
				ReqType:          CEInferenceResponse,
				Id:               id,
				SourceUri:        eh.sourceUri,
				InferenceService: eh.inferenceService,
				Namespace:        eh.namespace,
				Endpoint:         eh.endpoint,
				Annotations:      eh.annotations,
				Component:        eh.component,
				CertName:         eh.certName,
				TlsSkipVerify:    eh.tlsSkipVerify,
			}); err != nil {
				eh.log.Error(err, "Failed to log response")
			}
		}
	} else {
		eh.log.Info("Failed to proxy request", "status code", lrw.statusCode)
	}
}

func getOrCreateID(r *http.Request) string {
	id := r.Header.Get(CloudEventsIdHeader)
	if id == "" {
		id = guuid.New().String()
	}
	return id
}
