/*
Copyright 2018 The Knative Authors
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
package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/serving/pkg/activator"
)

func TestRequestEventHandler(t *testing.T) {
	namespace := "testspace"
	revision := "testrevision"

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	reqChan := make(chan ReqEvent, 2)
	handler := NewRequestEventHandler(reqChan, baseHandler)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example.com", bytes.NewBufferString(""))
	req.Header.Add(activator.RevisionHeaderNamespace, namespace)
	req.Header.Add(activator.RevisionHeaderName, revision)

	handler.ServeHTTP(resp, req)

	in := <-handler.ReqChan
	wantIn := ReqEvent{
		Key:       fmt.Sprintf("%s/%s", namespace, revision),
		EventType: ReqIn,
	}

	if diff := cmp.Diff(wantIn, in); diff != "" {
		t.Errorf("Unexpected event (-want +got): %v", diff)
	}

	out := <-handler.ReqChan
	wantOut := ReqEvent{
		Key:       wantIn.Key,
		EventType: ReqOut,
	}

	if diff := cmp.Diff(wantOut, out); diff != "" {
		t.Errorf("Unexpected event (-want +got): %v", diff)
	}
}
