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
	"path/filepath"
	"testing"

	"github.com/kserve/kserve/kernelcache/mcv/pkg/config"
	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/kernelcache/mcv/pkg/imgbuild"
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
		snapshotFlag    bool
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
			name:         "Valid snapshot with dir",
			snapshotFlag: true,
			cacheDirName: "/tmp/cache",
			expectError:  false,
		},
		{
			name:         "Missing dir for snapshot",
			snapshotFlag: true,
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
			err := validateFlagCombinations(tt.createFlag, tt.extractFlag, tt.snapshotFlag, tt.gpuInfoFlag, tt.checkCompatFlag, tt.imageName, tt.cacheDirName, tt.stubFlag)
			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

// TestConfigureBoolFlagsNoGPU verifies that configureBoolFlags disables GPU
// detection when noGPUFlag is true. This guards the flag-ordering regression
// where --no-gpu was silently ignored on --create because configureBoolFlags
// was called after the create branch returned.
func TestConfigureBoolFlagsNoGPU(t *testing.T) {
	if _, err := config.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	origGPU := config.IsGPUEnabled()
	t.Cleanup(func() { config.SetEnabledGPU(origGPU) })

	configureBoolFlags(false, true, false)
	if config.IsGPUEnabled() {
		t.Error("expected GPU disabled after configureBoolFlags(noGPU=true), got enabled")
	}

	configureBoolFlags(false, false, false)
	if !config.IsGPUEnabled() {
		t.Error("expected GPU enabled after configureBoolFlags(noGPU=false), got disabled")
	}
}

func TestValidateDeltaFlag(t *testing.T) {
	tests := []struct {
		name        string
		delta       bool
		create      bool
		builder     string
		expectError bool
	}{
		{name: "Delta disabled", builder: imgbuild.Buildah},
		{name: "Delta requires create", delta: true, builder: imgbuild.OCI, expectError: true},
		{name: "Delta requires OCI builder", delta: true, create: true, builder: imgbuild.Buildah, expectError: true},
		{name: "Valid delta create", delta: true, create: true, builder: imgbuild.OCI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeltaFlag(tt.delta, tt.create, tt.builder)
			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestValidateResultFlag(t *testing.T) {
	tests := []struct {
		name        string
		resultPath  string
		create      bool
		builder     string
		expectError bool
	}{
		{name: "Result disabled"},
		{name: "Result requires create", resultPath: "/tmp/result.json", builder: imgbuild.OCI, expectError: true},
		{name: "Result requires OCI builder", resultPath: "/tmp/result.json", create: true, builder: imgbuild.Buildah, expectError: true},
		{name: "Valid OCI result", resultPath: "/tmp/result.json", create: true, builder: imgbuild.OCI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResultFlag(tt.resultPath, tt.create, tt.builder)
			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestResolveCacheDirectory(t *testing.T) {
	got, err := resolveCacheDirectory(filepath.Join(".", "cache"))
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(got))
	require.Equal(t, filepath.Clean(got), got)
}

func TestSnapshotRootOptionsIncludeDefaultExclusions(t *testing.T) {
	roots := snapshotRootOptions("/tmp/cache", []string{"dummy_cache", "temporary"})
	require.Len(t, roots, 1)
	require.Equal(t, "/tmp/cache", roots[0].Source)
	require.Equal(t, []string{"dummy_cache", "temporary"}, roots[0].ExcludedDirectories)
}

func TestValidateExcludedDirectories(t *testing.T) {
	require.NoError(t, validateExcludedDirectories(nil, false))
	require.NoError(t, validateExcludedDirectories([]string{"dummy_cache"}, true))
	require.Error(t, validateExcludedDirectories([]string{"dummy_cache"}, false))
}
