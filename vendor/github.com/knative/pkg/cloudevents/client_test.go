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

package cloudevents_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/knative/pkg/cloudevents"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// Headers that are added to the response, but we don't want to check in our assertions.
	unimportantHeaders = sets.NewString(
		"accept-encoding",
		"content-length",
		"user-agent",
	)
)

func TestNewClient(t *testing.T) {
	m := "NewClient"

	for _, test := range []struct {
		name      string
		eventType string
		source    string
		target    string
		want      *cloudevents.Client
		opt       cmp.Option
	}{
		{
			name: "Simple",
			want: &cloudevents.Client{},
		}, {
			name:      "Full",
			eventType: "event.type",
			source:    "source",
			target:    "target",
			want: &cloudevents.Client{
				Target: "target",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := cloudevents.NewClient(test.target, cloudevents.Builder{Source: test.source, EventType: test.eventType})

			if diff := cmp.Diff(test.want, got, test.opt, cmpopts.IgnoreUnexported(cloudevents.Client{})); diff != "" {
				t.Errorf("%s (-want, +got) = %v", m, diff)
			}
		})
	}
}

func TestClientSend(t *testing.T) {
	now := time.Now()
	eventID := "AABBCCDDEE"
	doc := FirestoreDocument{
		Name: "projects/demo/databases/default/documents/users/inlined",
		Fields: map[string]interface{}{
			"project": "eventing",
			"handle":  "@inlined",
		},
		CreateTime: time.Date(1985, 6, 5, 12, 0, 0, 0, time.UTC),
		UpdateTime: now.UTC(),
	}

	service := "firestore.googleapis.com"

	for _, test := range []struct {
		name            string
		client          cloudevents.Client
		override        *cloudevents.V01EventContext
		fakeResponse    *http.Response
		expectedRequest *requestValidation
		errText         string
	}{
		{
			name: "binary simple v0.1",
			client: func() cloudevents.Client {
				client := cloudevents.NewClient(
					"server url",
					cloudevents.Builder{
						Source:    fmt.Sprintf("//%s/%s", service, doc.Name),
						EventType: "google.firestore.document.create",
						Encoding:  cloudevents.BinaryV01,
					},
				)
				return *client
			}(),
			override: &cloudevents.V01EventContext{
				EventTime: now,
				EventID:   eventID,
			},
			fakeResponse: &http.Response{
				StatusCode: http.StatusAccepted,
			},
			expectedRequest: &requestValidation{
				Headers: map[string][]string{
					"ce-cloudeventsversion": {"0.1"},
					"ce-eventid":            {"AABBCCDDEE"},
					"ce-eventtime":          {now.Format(time.RFC3339Nano)},
					"ce-eventtype":          {"google.firestore.document.create"},
					"ce-source":             {"//firestore.googleapis.com/projects/demo/databases/default/documents/users/inlined"},
					"content-type":          {"application/json"},
				},
				Body: fmt.Sprintf(
					`{"name":"projects/demo/databases/default/documents/users/inlined","fields":{"handle":"@inlined","project":"eventing"},"createTime":"1985-06-05T12:00:00Z","updateTime":%q}`,
					now.UTC().Format(time.RFC3339Nano),
				),
			},
		}, {
			name: "binary full v0.1",
			client: func() cloudevents.Client {
				client := cloudevents.NewClient(
					"source url",
					cloudevents.Builder{
						Source:           fmt.Sprintf("//%s/%s", service, doc.Name),
						EventType:        "google.firestore.document.create",
						EventTypeVersion: "v1beta2",
						SchemaURL:        "http://type.googleapis.com/google.firestore.v1beta1.Document",
						Encoding:         cloudevents.BinaryV01,
						Extensions: map[string]interface{}{
							"purpose": "tbd",
						},
					},
				)
				return *client
			}(),
			override: &cloudevents.V01EventContext{
				EventTime: now,
				EventID:   eventID,
			},
			fakeResponse: &http.Response{
				StatusCode: http.StatusAccepted,
			},
			expectedRequest: &requestValidation{
				Headers: map[string][]string{
					"ce-cloudeventsversion": {"0.1"},
					"ce-eventtypeversion":   {"v1beta2"},
					"ce-eventid":            {"AABBCCDDEE"},
					"ce-eventtime":          {now.Format(time.RFC3339Nano)},
					"ce-eventtype":          {"google.firestore.document.create"},
					"ce-source":             {"//firestore.googleapis.com/projects/demo/databases/default/documents/users/inlined"},
					"ce-schemaurl":          {"http://type.googleapis.com/google.firestore.v1beta1.Document"},
					"ce-x-purpose":          {`"tbd"`},
					"content-type":          {"application/json"},
				},
				Body: fmt.Sprintf(
					`{"name":"projects/demo/databases/default/documents/users/inlined","fields":{"handle":"@inlined","project":"eventing"},"createTime":"1985-06-05T12:00:00Z","updateTime":%q}`,
					now.UTC().Format(time.RFC3339Nano),
				),
			},
		}, {
			name: "structured simple v0.1",
			client: func() cloudevents.Client {
				client := cloudevents.NewClient(
					"server url",
					cloudevents.Builder{
						Source:    fmt.Sprintf("//%s/%s", service, doc.Name),
						EventType: "google.firestore.document.create",
						Encoding:  cloudevents.StructuredV01,
					},
				)
				return *client
			}(),
			override: &cloudevents.V01EventContext{
				EventTime: now,
				EventID:   eventID,
			},
			fakeResponse: &http.Response{
				StatusCode: http.StatusAccepted,
			},
			expectedRequest: &requestValidation{
				Headers: map[string][]string{
					"content-type": {"application/cloudevents+json"},
				},
				Body: fmt.Sprintf(
					`{"cloudEventsVersion":"0.1","contentType":"application/json","data":{"name":"projects/demo/databases/default/documents/users/inlined","fields":{"handle":"@inlined","project":"eventing"},"createTime":"1985-06-05T12:00:00Z","updateTime":%q},"eventID":"AABBCCDDEE","eventTime":%q,"eventType":"google.firestore.document.create","extensions":null,"source":"//firestore.googleapis.com/projects/demo/databases/default/documents/users/inlined"}`,
					now.UTC().Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
				),
			},
		}, {
			name: "structured full v0.1",
			client: func() cloudevents.Client {
				client := cloudevents.NewClient(
					"source url",
					cloudevents.Builder{
						Source:           fmt.Sprintf("//%s/%s", service, doc.Name),
						EventType:        "google.firestore.document.create",
						EventTypeVersion: "v1beta2",
						SchemaURL:        "http://type.googleapis.com/google.firestore.v1beta1.Document",
						Encoding:         cloudevents.StructuredV01,
						Extensions: map[string]interface{}{
							"purpose": "tbd",
						},
					},
				)
				return *client
			}(),
			override: &cloudevents.V01EventContext{
				EventTime: now,
				EventID:   eventID,
			},
			fakeResponse: &http.Response{
				StatusCode: http.StatusAccepted,
			},
			expectedRequest: &requestValidation{
				Headers: map[string][]string{
					"content-type": {"application/cloudevents+json"},
				},
				Body: fmt.Sprintf(
					`{"cloudEventsVersion":"0.1","contentType":"application/json","data":{"name":"projects/demo/databases/default/documents/users/inlined","fields":{"handle":"@inlined","project":"eventing"},"createTime":"1985-06-05T12:00:00Z","updateTime":%q},"eventID":"AABBCCDDEE","eventTime":%q,"eventType":"google.firestore.document.create","eventTypeVersion":"v1beta2","extensions":{"purpose":"tbd"},"schemaURL":"http://type.googleapis.com/google.firestore.v1beta1.Document","source":"//firestore.googleapis.com/projects/demo/databases/default/documents/users/inlined"}`,
					now.UTC().Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
				),
			},
		}, {
			name: "builder fails, no event type",
			client: func() cloudevents.Client {
				client := cloudevents.NewClient(
					"server url",
					cloudevents.Builder{
						Source: fmt.Sprintf("//%s/%s", service, doc.Name),
					},
				)
				return *client
			}(),
			override: &cloudevents.V01EventContext{},
			errText:  "EventType resolved empty",
		}, {
			name: "request not accepted",
			client: func() cloudevents.Client {
				client := cloudevents.NewClient(
					"server url",
					cloudevents.Builder{
						Source:    fmt.Sprintf("//%s/%s", service, doc.Name),
						EventType: "google.firestore.document.create",
					},
				)
				return *client
			}(),
			override: &cloudevents.V01EventContext{},
			fakeResponse: &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       ioutil.NopCloser(bytes.NewBufferString(`{"error":"nope"}`)),
			},
			errText: "Status[400 Bad Request]",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := &fakeHandler{
				t:        t,
				response: test.fakeResponse,
				requests: make([]requestValidation, 0),
			}
			server := httptest.NewServer(handler)
			defer server.Close()

			// Need to update dynamic target.
			test.client.Target = server.URL

			err := test.client.Send(doc, *test.override)
			if test.errText != "" {
				if err == nil {
					t.Fatalf("failed to return expected error, got nil")
				}
				want := test.errText
				got := err.Error()
				if !strings.Contains(got, want) {
					t.Fatalf("failed to return expected error, got %q, want %q", err, want)
				}
				return
			} else {
				if err != nil {
					t.Fatalf("Failed to send event %s", err)
				}
			}

			rv := handler.popRequest(t)

			assertEquality(t, server.URL, *test.expectedRequest, rv)
		})
	}
}

type requestValidation struct {
	Host    string
	Headers http.Header
	Body    string
}

type fakeHandler struct {
	t        *testing.T
	response *http.Response
	requests []requestValidation
}

func (f *fakeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// Make a copy of the request.
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		f.t.Error("Failed to read the request body")
	}
	f.requests = append(f.requests, requestValidation{
		Host:    r.Host,
		Headers: r.Header,
		Body:    string(body),
	})

	// Write the response.
	if f.response != nil {
		for h, vs := range f.response.Header {
			for _, v := range vs {
				w.Header().Add(h, v)
			}
		}
		w.WriteHeader(f.response.StatusCode)
		var buf bytes.Buffer
		if f.response.ContentLength > 0 {
			buf.ReadFrom(f.response.Body)
			w.Write(buf.Bytes())
		}
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
	}
}

func (f *fakeHandler) popRequest(t *testing.T) requestValidation {
	if len(f.requests) == 0 {
		t.Error("Unable to pop request")
	}
	rv := f.requests[0]
	f.requests = f.requests[1:]
	return rv
}

func assertEquality(t *testing.T, replacementURL string, expected, actual requestValidation) {
	server, err := url.Parse(replacementURL)
	if err != nil {
		t.Errorf("Bad replacement URL: %q", replacementURL)
	}
	expected.Host = server.Host
	canonicalizeHeaders(expected, actual)
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Errorf("Unexpected difference (-want, +got): %v", diff)
	}
}

func canonicalizeHeaders(rvs ...requestValidation) {
	// HTTP header names are case-insensitive, so normalize them to lower case for comparison.
	for _, rv := range rvs {
		headers := rv.Headers
		for n, v := range headers {
			delete(headers, n)
			ln := strings.ToLower(n)
			if !unimportantHeaders.Has(ln) {
				headers[ln] = v
			}
		}
	}
}
