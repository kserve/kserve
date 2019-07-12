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
	"strings"
	"testing"

	"github.com/knative/pkg/cloudevents"
)

func TestBuilderBuildValidation(t *testing.T) {

	for _, test := range []struct {
		name      string
		b         cloudevents.Builder
		target    string
		data      interface{}
		overrides []cloudevents.SendContext
		errText   string
	}{
		{
			name:    "Source is empty",
			b:       cloudevents.Builder{},
			errText: "Source resolved empty",
		}, {
			name: "EventType is empty",
			b: cloudevents.Builder{
				Source: "source",
			},
			errText: "EventType resolved empty",
		}, {
			name: "source from overrides, EventType is empty",
			b:    cloudevents.Builder{},
			overrides: []cloudevents.SendContext{cloudevents.V01EventContext{
				Source: "source",
			}},
			errText: "EventType resolved empty",
		}, {
			name: "valid, source and event type from overrides",
			b:    cloudevents.Builder{},
			overrides: []cloudevents.SendContext{cloudevents.V01EventContext{
				Source:    "source",
				EventType: "event.type",
			}},
		}, {
			name: "too many overrides",
			b: cloudevents.Builder{
				Source:    "source",
				EventType: "event.type",
			},
			overrides: []cloudevents.SendContext{cloudevents.V01EventContext{}, cloudevents.V01EventContext{}},
			errText:   "Build was called with more than one override",
		}, {
			name: "override is wrong type",
			b: cloudevents.Builder{
				Source:    "source",
				EventType: "event.type",
			},
			overrides: []cloudevents.SendContext{nil},
			errText:   "Build was called with unknown override type",
		}, {
			name: "valid default",
			b: cloudevents.Builder{
				Source:    "source",
				EventType: "event.type",
			},
		}, {
			name: "valid binary v0.1",
			b: cloudevents.Builder{
				Source:    "source",
				EventType: "event.type",
				Encoding:  cloudevents.BinaryV01,
			},
		}, {
			name: "valid structured v0.1",
			b: cloudevents.Builder{
				Source:    "source",
				EventType: "event.type",
				Encoding:  cloudevents.StructuredV01,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := test.b.Build(test.target, test.data, test.overrides...)
			if test.errText != "" {
				if err == nil {
					t.Fatalf("failed to return expected error, got nil")
				}
				want := test.errText
				got := err.Error()
				if !strings.Contains(got, want) {
					t.Fatalf("failed to return expected error, got %v, want %v", err, want)
				}
				return
			} else if err != nil {
				t.Fatalf("wanted no error, got %v", err)
			}

			_ = req
		})
	}
}
