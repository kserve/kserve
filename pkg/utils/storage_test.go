/*
Copyright 2021 The KServe Authors.

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

	"github.com/onsi/gomega"
)

func TestFindCommonParentPath(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		paths    []string
		expected string
	}{
		"EmptyPaths": {
			paths:    []string{},
			expected: "",
		},
		"SinglePath": {
			paths:    []string{"/mnt/models"},
			expected: "/mnt/models",
		},
		"CommonParent": {
			paths: []string{
				"/mnt/models/model1",
				"/mnt/models/model2",
			},
			expected: "/mnt/models",
		},
		"DifferentRoots": {
			paths: []string{
				"/mnt/models",
				"/opt/models",
			},
			expected: "/",
		},
		"NestedCommonParent": {
			paths: []string{
				"/mnt/models/pytorch/model1",
				"/mnt/models/pytorch/model2",
				"/mnt/models/pytorch/",
			},
			expected: "/mnt/models/pytorch",
		},
		"NoCommonParent": {
			paths: []string{
				"/mnt/models",
				"/opt/data",
				"/var/cache",
			},
			expected: "/",
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result := FindCommonParentPath(scenario.paths)
			g.Expect(result).To(gomega.Equal(scenario.expected))
		})
	}
}

func TestGetVolumeNameFromPath(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	scenarios := map[string]struct {
		path     string
		expected string
	}{
		"RootPath": {
			path:     "/",
			expected: "",
		},
		"SimplePath": {
			path:     "/mnt/models",
			expected: "mnt-models",
		},
		"NestedPath": {
			path:     "/mnt/models/subdir",
			expected: "mnt-models-subdir",
		},
		"PathWithTrailingSlash": {
			path:     "/mnt/models/",
			expected: "mnt-models",
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			result := GetVolumeNameFromPath(scenario.path)
			g.Expect(result).To(gomega.Equal(scenario.expected))
		})
	}
}
