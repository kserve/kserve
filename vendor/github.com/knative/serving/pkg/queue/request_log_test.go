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
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var defaultRevInfo = &RequestLogRevInfo{
	Name:          "rev",
	Namespace:     "ns",
	Service:       "svc",
	Configuration: "cfg",
	PodName:       "pn",
	PodIP:         "ip",
}

func TestRequestLogHandler(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name     string
		url      string
		body     string
		template string
		want     string
		wantErr  bool
	}{{
		name:     "empty template",
		url:      "http://example.com/testpage",
		body:     "test",
		template: "",
		want:     "\n",
		wantErr:  false,
	}, {
		name:     "template with new line",
		url:      "http://example.com/testpage",
		body:     "test",
		template: "{{.Request.URL}}\n",
		want:     "http://example.com/testpage\n",
		wantErr:  false,
	}, {
		name:     "template without new line",
		url:      "http://example.com",
		body:     "test",
		template: "{{.Request.ContentLength}}",
		want:     "4\n",
		wantErr:  false,
	}, {
		name:     "invalid template",
		url:      "http://example.com",
		body:     "test",
		template: "{{}}",
		want:     "",
		wantErr:  true,
	}, {
		name:     "revision info",
		url:      "http://example.com",
		body:     "test",
		template: "{{.Revision.Name}}, {{.Revision.Namespace}}, {{.Revision.Service}}, {{.Revision.Configuration}}, {{.Revision.PodName}}, {{.Revision.PodIP}}",
		want:     "rev, ns, svc, cfg, pn, ip\n",
		wantErr:  false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := bytes.NewBufferString("")
			handler, err := NewRequestLogHandler(baseHandler, buf, test.template, defaultRevInfo)
			if test.wantErr != (err != nil) {
				t.Errorf("got %v, want error %v", err, test.wantErr)
			}

			if !test.wantErr {
				resp := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, test.url, bytes.NewBufferString(test.body))
				handler.ServeHTTP(resp, req)

				got := string(buf.Bytes())
				if got != test.want {
					t.Errorf("got '%v', want '%v'", got, test.want)
				}
			}
		})
	}
}

func TestPanickingHandler(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("no!")
	})
	buf := bytes.NewBufferString("")
	handler, err := NewRequestLogHandler(baseHandler, buf, "{{.Request.URL}}", defaultRevInfo)
	if err != nil {
		t.Errorf("got %v, want error: %v", err, false)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewBufferString("test"))
	defer func() {
		err := recover()
		if err == nil {
			t.Error("want ServeHTTP to panic, got nothing.")
		}

		got := string(buf.Bytes())
		if want := "http://example.com\n"; got != want {
			t.Errorf("got '%v', want '%v'", got, want)
		}
	}()
	handler.ServeHTTP(resp, req)
}

func TestFailedTemplateExecution(t *testing.T) {
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	buf := bytes.NewBufferString("")
	handler, err := NewRequestLogHandler(baseHandler, buf, "{{.Request.Something}}", defaultRevInfo)
	if err != nil {
		t.Errorf("got %v, wantErr %v, ", err, false)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com", bytes.NewBufferString("test"))
	handler.ServeHTTP(resp, req)

	got := string(buf.Bytes())
	if want := "Invalid request log template: "; !strings.HasPrefix(got, want) {
		t.Errorf("got: '%v', want: '%v'", got, want)
	}
}
