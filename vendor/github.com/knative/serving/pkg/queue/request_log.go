/*
Copyright 2019 The Knative Authors

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

package queue

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	pkghttp "github.com/knative/serving/pkg/http"
)

// RequestLogRevInfo provides revision related static information
// for the template execution.
type RequestLogRevInfo struct {
	Name          string
	Namespace     string
	Service       string
	Configuration string
	PodName       string
	PodIP         string
}

// respInfo provided response related information for the template execution.
type respInfo struct {
	Code    int
	Size    int
	Latency float64
}

// templateInput is the wrapper struct that provides all necessary information
// for the template execution.
type templateInput struct {
	Request  *http.Request
	Response *respInfo
	Revision *RequestLogRevInfo
}

type requestLogHandler struct {
	handler  http.Handler
	writer   io.Writer
	template *template.Template
	revision *RequestLogRevInfo
}

// NewRequestLogHandler creates an http.Handler that logs request logs to an io.Writer.
func NewRequestLogHandler(h http.Handler, w io.Writer, templateStr string,
	revInfo *RequestLogRevInfo) (http.Handler, error) {
	// Make sure that the template ends with a newline. Otherwise,
	// logging backends will not be able to parse entries separately.
	if !strings.HasSuffix(templateStr, "\n") {
		templateStr = templateStr + "\n"
	}

	// Expose a function to give access to cached environment variables within the template
	template, err := template.New("requestLog").Parse(templateStr)
	if err != nil {
		return nil, err
	}

	return &requestLogHandler{
		handler:  h,
		writer:   w,
		template: template,
		revision: revInfo,
	}, nil
}

func (h *requestLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rr := pkghttp.NewResponseRecorder(w, http.StatusOK)
	startTime := time.Now()
	defer func() {
		// If ServeHTTP panics, recover, record the failure and panic again.
		err := recover()
		latency := time.Since(startTime).Seconds()
		if err != nil {
			t := &templateInput{r, &respInfo{http.StatusInternalServerError, 0, latency}, h.revision}
			h.writeRequestLog(t)
			panic(err)
		} else {
			t := &templateInput{r, &respInfo{rr.ResponseCode, (int)(rr.ResponseSize), latency}, h.revision}
			h.writeRequestLog(t)
		}
	}()
	h.handler.ServeHTTP(rr, r)
}

func (h *requestLogHandler) writeRequestLog(t *templateInput) {
	if err := h.template.Execute(h.writer, t); err != nil {
		// Template execution failed. Write an error message with some basic information about the request.
		fmt.Fprintf(h.writer, "Invalid request log template: method: %v, response code: %v, latency: %v, url: %v\n",
			t.Request.Method, t.Response.Code, t.Response.Latency, t.Request.URL)
	}
}
