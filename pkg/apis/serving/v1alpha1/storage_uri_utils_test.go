/*
Copyright 2023 The KServe Authors.

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

package v1alpha1

import "testing"

func TestMatchStorageURI(t *testing.T) {
	tests := []struct {
		name       string
		cachedUri  string
		storageUri string
		expected   bool
	}{
		{"exact match", "gs://bucket/model", "gs://bucket/model", true},
		{"exact match with trailing slash", "gs://bucket/model/", "gs://bucket/model", true},
		{"subdirectory match", "gs://bucket/model", "gs://bucket/model/subdir", true},
		{"no match different path", "gs://bucket/model", "gs://bucket/other", false},
		{"no match prefix attack", "gs://bucket/model", "gs://bucket/model-evil", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchStorageURI(tt.cachedUri, tt.storageUri); got != tt.expected {
				t.Errorf("MatchStorageURI(%q, %q) = %v, want %v", tt.cachedUri, tt.storageUri, got, tt.expected)
			}
		})
	}
}
