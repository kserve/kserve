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

package cloudevents

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/google/go-cmp/cmp"
)

func TestCloudEventsContextV01(t *testing.T) {
	m := "cloudEventsContextV01"
	now := time.Now()
	for _, test := range []struct {
		name     string
		b        Builder
		override *V01EventContext
		want     V01EventContext
		opt      cmp.Option
	}{
		{
			name: "basic",
			b: Builder{
				Source:    "source",
				EventType: "event.type",
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventType:          "event.type",
				ContentType:        "application/json",
				Source:             "source",
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID", "EventTime"),
		}, {
			name: "full",
			b: Builder{
				Source:           "source",
				EventType:        "event.type",
				EventTypeVersion: "beta",
				SchemaURL:        "http://test/",
				Extensions: map[string]interface{}{
					"a": "outer",
					"b": map[string]interface{}{
						"c": "inner",
					},
				},
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventType:          "event.type",
				ContentType:        "application/json",
				Source:             "source",
				EventTypeVersion:   "beta",
				SchemaURL:          "http://test/",
				Extensions: map[string]interface{}{
					"a": "outer",
					"b": map[string]interface{}{
						"c": "inner",
					},
				},
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID", "EventTime"),
		}, {
			name: "override time",
			b: Builder{
				Source:    "source",
				EventType: "event.type",
			},
			override: &V01EventContext{
				EventTime: now,
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventTime:          now,
				EventType:          "event.type",
				ContentType:        "application/json",
				Source:             "source",
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID"),
		}, {
			name: "override event id",
			b: Builder{
				Source:    "source",
				EventType: "event.type",
			},
			override: &V01EventContext{
				EventID: "ABC",
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventID:            "ABC",
				EventType:          "event.type",
				ContentType:        "application/json",
				Source:             "source",
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventTime"),
		}, {
			name: "override source",
			b: Builder{
				Source:    "source",
				EventType: "event.type",
			},
			override: &V01EventContext{
				Source: "Override-Source",
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventType:          "event.type",
				ContentType:        "application/json",
				Source:             "Override-Source",
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID", "EventTime"),
		}, {
			name: "override content type",
			b: Builder{
				Source:    "source",
				EventType: "event.type",
			},
			override: &V01EventContext{
				ContentType: "Override-ContentType",
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventType:          "event.type",
				ContentType:        "Override-ContentType",
				Source:             "source",
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID", "EventTime"),
		}, {
			name: "override event type",
			b: Builder{
				Source:    "source",
				EventType: "event.type",
			},
			override: &V01EventContext{
				EventType: "override.event.type",
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventType:          "override.event.type",
				ContentType:        "application/json",
				Source:             "source",
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID", "EventTime"),
		}, {
			name: "override extensions",
			b: Builder{
				Source:           "source",
				EventType:        "event.type",
				EventTypeVersion: "beta",
				SchemaURL:        "http://test/",
				Extensions: map[string]interface{}{
					"a": "outer",
					"b": map[string]interface{}{
						"c": "inner",
					},
				},
			},
			override: &V01EventContext{
				Extensions: map[string]interface{}{
					"a": "override-outer",
					"d": map[string]interface{}{
						"e": "add-inner",
					},
				},
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventType:          "event.type",
				ContentType:        "application/json",
				Source:             "source",
				EventTypeVersion:   "beta",
				SchemaURL:          "http://test/",
				Extensions: map[string]interface{}{
					"a": "override-outer",
					"b": map[string]interface{}{
						"c": "inner",
					},
					"d": map[string]interface{}{
						"e": "add-inner",
					},
				},
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID", "EventTime"),
		}, {
			name: "override empty extensions",
			b: Builder{
				Source:           "source",
				EventType:        "event.type",
				EventTypeVersion: "beta",
				SchemaURL:        "http://test/",
			},
			override: &V01EventContext{
				Extensions: map[string]interface{}{
					"a": "override-outer",
					"d": map[string]interface{}{
						"e": "add-inner",
					},
				},
			},
			want: V01EventContext{
				CloudEventsVersion: "0.1",
				EventType:          "event.type",
				ContentType:        "application/json",
				Source:             "source",
				EventTypeVersion:   "beta",
				SchemaURL:          "http://test/",
				Extensions: map[string]interface{}{
					"a": "override-outer",
					"d": map[string]interface{}{
						"e": "add-inner",
					},
				},
			},
			opt: cmpopts.IgnoreFields(V01EventContext{}, "EventID", "EventTime"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := test.b.cloudEventsContextV01(test.override)
			if got.EventID == "" {
				t.Errorf("%s produced empty EventID", m)
			}
			if got.EventTime.IsZero() {
				t.Errorf("%s produced zeroed EventTime", m)
			}
			if diff := cmp.Diff(test.want, got, test.opt); diff != "" {
				t.Errorf("%s (-want, +got) = %v", m, diff)
			}
		})
	}
}
