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

package apis

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGVK2GVR(t *testing.T) {

	tests := []struct {
		name  string
		input schema.GroupVersionKind
		want  schema.GroupVersionResource
	}{{
		name: "simple",
		input: schema.GroupVersionKind{
			Group:   "test.knative.dev",
			Version: "v1",
			Kind:    "Resource",
		},
		want: schema.GroupVersionResource{
			Group:    "test.knative.dev",
			Version:  "v1",
			Resource: "resources",
		},
	}, {
		name: "non-trivial",
		input: schema.GroupVersionKind{
			Group:   "test.knative.dev",
			Version: "v1",
			Kind:    "Mess",
		},
		want: schema.GroupVersionResource{
			Group:    "test.knative.dev",
			Version:  "v1",
			Resource: "messes",
		},
	}, {
		name: "another non-trivial (not ies)",
		input: schema.GroupVersionKind{
			Group:   "test.knative.dev",
			Version: "v1",
			Kind:    "Gateway",
		},
		want: schema.GroupVersionResource{
			Group:    "test.knative.dev",
			Version:  "v1",
			Resource: "gateways",
		},
	}}

	// TODO(mattmoor): Errata: pony, cactus, ?

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := KindToResource(test.input)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("KindToResource (-want, +got) = %v", diff)
			}
		})
	}
}
