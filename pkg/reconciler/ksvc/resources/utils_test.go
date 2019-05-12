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

package resources

import (
	"github.com/google/go-cmp/cmp"
	"github.com/knative/serving/pkg/apis/autoscaling"
	"testing"
)

func TestFilterUtil(t *testing.T) {
	scenarios := map[string]struct {
		input     map[string]string
		predicate func(string) bool
		expected  map[string]string
	}{
		"ConfigurationAnnotationFilter": {
			input: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "1",
				autoscaling.TargetAnnotationKey: "1", autoscaling.ClassAnnotationKey: "KPA"},
			predicate: configurationAnnotationFilter,
			expected:  map[string]string{autoscaling.TargetAnnotationKey: "1", autoscaling.ClassAnnotationKey: "KPA"},
		},
		"RouteAnnotationFilter": {
			input: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "1",
				"random annotation": "test"},
			predicate: routeAnnotationFilter,
			expected:  map[string]string{},
		},
	}
	for name, scenario := range scenarios {
		result := filter(scenario.input, scenario.predicate)

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
			input1: nil,
			input2: nil,
			expected: map[string]string{},
		},
	}
	for name, scenario := range scenarios {
		result := union(scenario.input1, scenario.input2)

		if diff := cmp.Diff(scenario.expected, result); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
