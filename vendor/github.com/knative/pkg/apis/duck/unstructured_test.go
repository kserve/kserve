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

package duck

import (
	"reflect"
	"testing"

	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFromUnstructuredFooable(t *testing.T) {
	tcs := []struct {
		name      string
		in        Marshalable
		want      FooStatus
		wantError error
	}{{
		name: "Works with valid status",
		in: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test",
				"kind":       "test_kind",
				"name":       "test_name",
				"status": map[string]interface{}{
					"extra": "fields",
					"fooable": map[string]interface{}{
						"field1": "foo",
						"field2": "bar",
					},
				},
			}},
		want: FooStatus{&Fooable{
			Field1: "foo",
			Field2: "bar",
		}},
		wantError: nil,
	}, {
		name: "does not work with missing fooable status",
		in: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "test",
				"kind":       "test_kind",
				"name":       "test_name",
				"status": map[string]interface{}{
					"extra": "fields",
				},
			}},
		want:      FooStatus{},
		wantError: nil,
	}, {
		name:      "empty unstructured",
		in:        &unstructured.Unstructured{},
		want:      FooStatus{},
		wantError: nil,
	}}
	for _, tc := range tcs {
		raw, err := json.Marshal(tc.in)
		if err != nil {
			panic("failed to marshal")
		}

		t.Logf("Marshalled : %s", string(raw))

		got := Foo{}
		err = FromUnstructured(tc.in, &got)
		if err != tc.wantError {
			t.Errorf("Unexpected error for %q: %v", string(tc.name), err)
			continue
		}

		if !reflect.DeepEqual(tc.want, got.Status) {
			t.Errorf("Decode(%q) want: %+v\ngot: %+v", string(tc.name), tc.want, got)
		}
	}
}
