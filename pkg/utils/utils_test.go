/*
Copyright 2019 kubeflow.org.

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

package utils

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFilterUtil(t *testing.T) {
	scenarios := map[string]struct {
		input    map[string]string
		filter   map[string]struct{}
		expected map[string]string
	}{
		"TruthyFilter": {
			input:    map[string]string{"key1": "val1", "key2": "val2"},
			filter:   map[string]struct{}{},
			expected: map[string]string{"key1": "val1", "key2": "val2"},
		},
		"FalsyFilter": {
			input:    map[string]string{"key1": "val1", "key2": "val2"},
			filter:   map[string]struct{}{"key1": {}, "key2": {}},
			expected: map[string]string{},
		},
	}
	for name, scenario := range scenarios {
		result := Filter(scenario.input, scenario.filter)

		if diff := cmp.Diff(scenario.expected, result); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestUnionUtil(t *testing.T) {
	scenarios := map[string]struct {
		input1   map[string]string
		input2   map[string]string
		expected map[string]string
	}{
		"UnionTwoMaps": {
			input1: map[string]string{"serving.kubeflow.org/service": "mnist",
				"label1": "value1"},
			input2: map[string]string{"service.knative.dev/configuration": "mnist",
				"label2": "value2"},
			expected: map[string]string{"serving.kubeflow.org/service": "mnist",
				"label1": "value1", "service.knative.dev/configuration": "mnist", "label2": "value2"},
		},
		"UnionWithEmptyMap": {
			input1: map[string]string{},
			input2: map[string]string{"service.knative.dev/configuration": "mnist",
				"label2": "value2"},
			expected: map[string]string{"service.knative.dev/configuration": "mnist", "label2": "value2"},
		},
		"UnionWithNilMap": {
			input1: nil,
			input2: map[string]string{"service.knative.dev/configuration": "mnist",
				"label2": "value2"},
			expected: map[string]string{"service.knative.dev/configuration": "mnist", "label2": "value2"},
		},
		"UnionNilMaps": {
			input1:   nil,
			input2:   nil,
			expected: map[string]string{},
		},
	}
	for name, scenario := range scenarios {
		result := Union(scenario.input1, scenario.input2)

		if diff := cmp.Diff(scenario.expected, result); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestContainsUtil(t *testing.T) {
	scenarios := map[string]struct {
		input1   []string
		input2   string
		expected bool
	}{
		"SliceContainsString": {
			input1:   []string{"hey", "hello"},
			input2:   "hey",
			expected: true,
		},
		"SliceDoesNotContainString": {
			input1:   []string{"hey", "hello"},
			input2:   "he",
			expected: false,
		},
		"SliceIsEmpty": {
			input1:   []string{},
			input2:   "hey",
			expected: false,
		},
	}
	for name, scenario := range scenarios {
		result := Includes(scenario.input1, scenario.input2)
		if diff := cmp.Diff(scenario.expected, result); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
