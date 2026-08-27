/*
Copyright 2026 The KServe Authors.

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

package main

import (
	"testing"
)

const (
	testImageName    = "quay.io/gkm/cache-examples:vector-add-cache-cuda"
	testCacheDirName = "../example/vector-add-cache"
)

func TestValidateFlagCombinations(t *testing.T) {
	tests := []struct {
		name            string
		createFlag      bool
		extractFlag     bool
		gpuInfoFlag     bool
		checkCompatFlag bool
		imageName       string
		cacheDirName    string
		stubFlag        bool
		expectError     bool
	}{
		{
			name:         "Valid create flag with image and dir",
			createFlag:   true,
			imageName:    testImageName,
			cacheDirName: testCacheDirName,
			expectError:  false,
		},
		{
			name:         "Missing image name for create",
			createFlag:   true,
			cacheDirName: testCacheDirName,
			expectError:  true,
		},
		{
			name:        "Multiple action flags",
			createFlag:  true,
			extractFlag: true,
			imageName:   "quay.io/gkm/cache-examples:vector-add-cache-cuda",
			expectError: true,
		},
		{
			name:         "Invalid image name format",
			createFlag:   true,
			imageName:    "invalid:image_name",
			cacheDirName: testCacheDirName,
			expectError:  true,
		},
		{
			name:        "Stub flag without gpu-info",
			stubFlag:    true,
			expectError: true,
		},
		{
			name:            "Valid check-compat flag with image",
			checkCompatFlag: true,
			imageName:       "quay.io/gkm/cache-examples:vector-add-cache-cuda",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFlagCombinations(tt.createFlag, tt.extractFlag, tt.gpuInfoFlag, tt.checkCompatFlag, tt.imageName, tt.cacheDirName, tt.stubFlag)
			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}
