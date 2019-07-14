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

package http

import (
	"net/http"
	"testing"
)

func TestLastHeaderValue(t *testing.T) {
	headerKey := "Header-Key"

	tests := []struct {
		name     string
		expected string
		headers  http.Header
	}{{
		name:     "nil headers",
		expected: "",
		headers:  nil,
	}, {
		name:     "empty header value ",
		expected: "",
		headers: http.Header{
			headerKey: nil,
		},
	}, {
		name:     "single header value ",
		expected: "first",
		headers: http.Header{
			headerKey: {"first"},
		},
	}, {
		name:     "multi header value ",
		expected: "second",
		headers: http.Header{
			headerKey: {"first", "second"},
		},
	}}

	for _, test := range tests {
		got := LastHeaderValue(test.headers, headerKey)
		if got != test.expected {
			t.Errorf("Unexpected header value got - %q want - %q", got, test.expected)
		}
	}
}
