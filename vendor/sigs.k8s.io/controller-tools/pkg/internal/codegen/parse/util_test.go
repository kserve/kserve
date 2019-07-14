/*
Copyright 2018 The Kubernetes Authors.

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

package parse

import (
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/gengo/types"
)

func TestParseScaleParams(t *testing.T) {
	testCases := []struct {
		name     string
		tag      string
		expected map[string]string
		parseErr error
	}{
		{
			name: "test ok",
			tag:  "+kubebuilder:subresource:scale:specpath=.spec.replica,statuspath=.status.replica,selectorpath=.spec.Label",
			expected: map[string]string{
				specReplicasPath:   ".spec.replica",
				statusReplicasPath: ".status.replica",
				labelSelectorPath:  ".spec.Label",
			},
			parseErr: nil,
		},
		{
			name: "test ok without selectorpath",
			tag:  "+kubebuilder:subresource:scale:specpath=.spec.replica,statuspath=.status.replica",
			expected: map[string]string{
				specReplicasPath:   ".spec.replica",
				statusReplicasPath: ".status.replica",
			},
			parseErr: nil,
		},
		{
			name: "test ok selectorpath has empty value",
			tag:  "+kubebuilder:subresource:scale:specpath=.spec.replica,statuspath=.status.replica,selectorpath=",
			expected: map[string]string{
				specReplicasPath:   ".spec.replica",
				statusReplicasPath: ".status.replica",
				labelSelectorPath:  "",
			},
			parseErr: nil,
		},
		{
			name:     "test no jsonpath",
			tag:      "+kubebuilder:subresource:scale",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
		{
			name:     "test no specpath",
			tag:      "+kubebuilder:subresource:scale:statuspath=.status.replica,selectorpath=.jsonpath",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
		{
			name:     "test no statuspath",
			tag:      "+kubebuilder:subresource:scale:specpath=.spec.replica,selectorpath=.jsonpath",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
		{
			name:     "test statuspath is empty string",
			tag:      "+kubebuilder:subresource:scale:statuspath=,selectorpath=.jsonpath",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
		{
			name:     "test scale jsonpath has incorrect separator",
			tag:      "+kubebuilder:subresource:scale,specpath=.spec.replica,statuspath=.jsonpath,selectorpath=.jsonpath",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
		{
			name:     "test scale jsonpath has extra separator",
			tag:      "+kubebuilder:subresource:scale:specpath=.spec.replica,statuspath=.status.replicas,selectorpath=.jsonpath,",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
		{
			name:     "test scale jsonpath has incorrect separator in-between key value pairs",
			tag:      "+kubebuilder:subresource:scale:specpath=.spec.replica;statuspath=.jsonpath;selectorpath=.jsonpath",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
		{
			name:     "test unsupported key value pairs",
			tag:      "+kubebuilder:subresource:scale:name=test,specpath=.spec.replica,statuspath=.status.replicas,selectorpath=.jsonpath",
			expected: nil,
			parseErr: fmt.Errorf(jsonPathError),
		},
	}

	for _, tc := range testCases {
		t.Logf("test case: %s", tc.name)
		r := &types.Type{}
		r.CommentLines = []string{tc.tag}
		res, err := parseScaleParams(r)
		if !reflect.DeepEqual(err, tc.parseErr) {
			t.Errorf("test [%s] failed. error is (%v),\n but expected (%v)", tc.name, err, tc.parseErr)
		}
		if !reflect.DeepEqual(res, tc.expected) {
			t.Errorf("test [%s] failed. result is (%v),\n but expected (%v)", tc.name, res, tc.expected)
		}
	}
}
func TestParsePrintColumnParams(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected []v1beta1.CustomResourceColumnDefinition
		parseErr error
	}{
		{
			name: "test ok",
			tag:  "+kubebuilder:printcolumn:name=toy,type=string,JSONPath=.status.conditions[?(@.type==\"Ready\")].status,description=descr1,priority=3,format=date",
			expected: []v1beta1.CustomResourceColumnDefinition{
				{
					Name:        "toy",
					Type:        "string",
					Format:      "date",
					Description: "descr1",
					Priority:    3,
					JSONPath:    ".status.conditions[?(@.type==\"Ready\")].status",
				},
			},
			parseErr: nil,
		},
		{
			name: "age as date",
			tag:  "+kubebuilder:printcolumn:name=age,type=date,JSONPath=.metadata.creationTimestamp",
			expected: []v1beta1.CustomResourceColumnDefinition{
				{
					Name:     "age",
					Type:     "date",
					JSONPath: ".metadata.creationTimestamp",
				},
			},
			parseErr: nil,
		},
		{
			name: "Minimum Three parameters",
			tag:  "+kubebuilder:printcolumn:name=toy,type=string,JSONPath=.status.conditions[?(@.type==\"Ready\")].status",
			expected: []v1beta1.CustomResourceColumnDefinition{
				{
					Name:     "toy",
					Type:     "string",
					JSONPath: ".status.conditions[?(@.type==\"Ready\")].status",
				},
			},
			parseErr: nil,
		},
		{
			name:     "two parameters",
			tag:      "+kubebuilder:printcolumn:name=toy,type=string",
			expected: []v1beta1.CustomResourceColumnDefinition{},
			parseErr: fmt.Errorf(printColumnError),
		},
		{
			name:     "one requied parameter is missing",
			tag:      "+kubebuilder:printcolumn:name=toy,type=string,description=.status.conditions[?(@.type==\"Ready\")].status",
			expected: []v1beta1.CustomResourceColumnDefinition{},
			parseErr: fmt.Errorf(printColumnError),
		},
		{
			name:     "Invalid value for priority",
			tag:      "+kubebuilder:printcolumn:name=toy,type=string,description=.status.conditions[?(@.type==\"Ready\")].status,priority=1.23",
			expected: []v1beta1.CustomResourceColumnDefinition{},
			parseErr: fmt.Errorf("invalid value for %s printcolumn", printColumnPri),
		},
		{
			name:     "Invalid value for format",
			tag:      "+kubebuilder:printcolumn:name=toy,type=string,description=.status.conditions[?(@.type==\"Ready\")].status,format=float",
			expected: []v1beta1.CustomResourceColumnDefinition{},
			parseErr: fmt.Errorf("invalid value for %s printcolumn", printColumnFormat),
		},
		{
			name:     "Invalid value for type",
			tag:      "+kubebuilder:printcolumn:name=toy,type=int32,description=.status.conditions[?(@.type==\"Ready\")].status",
			expected: []v1beta1.CustomResourceColumnDefinition{},
			parseErr: fmt.Errorf("invalid value for %s printcolumn", printColumnType),
		},
	}
	for _, tc := range tests {
		t.Logf("test case: %s", tc.name)
		r := &types.Type{}
		r.CommentLines = []string{tc.tag}
		res, err := parsePrintColumnParams(r)
		if !reflect.DeepEqual(err, tc.parseErr) {
			t.Errorf("test [%s] failed. error is (%v),\n but expected (%v)", tc.name, err, tc.parseErr)
		}
		if !reflect.DeepEqual(res, tc.expected) {
			t.Errorf("test [%s] failed. result is (%v),\n but expected (%v)", tc.name, res, tc.expected)
		}
	}
}
