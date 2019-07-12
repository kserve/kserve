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

package http

import (
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type fakeResponseWriter struct{}

func (w *fakeResponseWriter) Header() http.Header         { return http.Header{"item1": []string{"value1"}} }
func (w *fakeResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *fakeResponseWriter) WriteHeader(code int)        {}
func (w *fakeResponseWriter) Flush()                      {}

var defaultHeader = http.Header{"item1": []string{"value1"}}

func TestResponseRecorder(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus int
		finalStatus   int
		hijack        bool
		writeSize     int
		wantStatus    int
		wantSize      int32
	}{{
		name:          "no hijack",
		initialStatus: http.StatusAccepted,
		finalStatus:   http.StatusBadGateway,
		hijack:        false,
		writeSize:     12,
		wantStatus:    http.StatusBadGateway,
		wantSize:      12,
	}, {
		name:          "hijack",
		initialStatus: http.StatusAccepted,
		finalStatus:   http.StatusBadGateway,
		hijack:        true,
		writeSize:     12,
		wantStatus:    http.StatusAccepted,
		wantSize:      12,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rr := NewResponseRecorder(&fakeResponseWriter{}, test.initialStatus)
			if test.hijack {
				rr.Hijack()
			}

			b := make([]byte, test.writeSize)
			rr.Write(b)
			rr.Flush()
			rr.WriteHeader(test.finalStatus)

			if got, want := rr.ResponseCode, test.wantStatus; got != want {
				t.Errorf("got %v, want %v", got, want)
			}
			if got, want := rr.ResponseSize, test.wantSize; got != want {
				t.Errorf("got %v, want %v", got, want)
			}
			if diff := cmp.Diff(rr.Header(), defaultHeader); diff != "" {
				t.Errorf("Headers are different (-want, +got) = %v", diff)
			}
		})
	}
}
