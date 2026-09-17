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

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const testImg = "quay.io/gkm/cache-examples:vector-add-cache-cuda"

// check that extracting cache detects the lack of a GPU and continues without error
func TestExtractCacheWithGPUEnabled(t *testing.T) {
	gpu := true
	opts := Options{
		ImageName: testImg,
		EnableGPU: &gpu,
	}

	matchedIDs, unmatchedIDs, err := ExtractCache(opts)
	assert.NoError(t, err, "ExtractCache should not return an error")
	assert.Nil(t, matchedIDs, "Matched IDs should be nil")         // as we are on a no GPU system
	assert.Nil(t, unmatchedIDs, "Unmatched IDs should not be nil") // as we are on a no GPU system
}

// TODO test needs debug
// func TestExtractCacheWithBaremetalEnabled(t *testing.T) {
// 	baremetal := true
// 	opts := Options{
// 		ImageName:       "quay.io/gkm/cache-examples:vector-add-cache-cuda",
// 		EnableBaremetal: &baremetal,
// 	}

// 	matchedIDs, unmatchedIDs, err := ExtractCache(opts)
// 	assert.Error(t, err, "ExtractCache should return an error")
// 	assert.Nil(t, matchedIDs, "Matched IDs should be nil")
// 	assert.Nil(t, unmatchedIDs, "Unmatched IDs should not be nil")
// }

func TestExtractCacheWithSkipPrecheck(t *testing.T) {
	skipPrecheck := true
	opts := Options{
		ImageName:    testImg,
		SkipPrecheck: &skipPrecheck,
	}

	matchedIDs, unmatchedIDs, err := ExtractCache(opts)
	assert.NoError(t, err, "ExtractCache should not return an error")
	assert.Nil(t, matchedIDs, "Matched IDs should be nil")
	assert.Nil(t, unmatchedIDs, "Unmatched IDs should not be nil")
}

func TestGetSystemGPUInfoWithTimeoutDisabled(t *testing.T) {
	stub := true
	opts := HwOptions{
		EnableStub: &stub,
		Timeout:    0, // Disable timeout
	}

	summary, err := GetSystemGPUInfo(opts)
	assert.NoError(t, err, "GetSystemGPUInfo should not return an error")
	if summary != nil {
		assert.Greater(t, len(summary.GPUs), 0, "There should be at least one GPU detected")
	} else {
		t.Log("No GPUs detected, which is acceptable in some environments")
	}
}

func TestGetSystemGPUInfoWithTimeoutEnabled(t *testing.T) {
	stub := true
	opts := HwOptions{
		EnableStub: &stub,
		Timeout:    5, // Set a timeout of 5 seconds
	}

	summary, err := GetSystemGPUInfo(opts)
	assert.NoError(t, err, "GetSystemGPUInfo should not return an error")
	if summary != nil {
		assert.Greater(t, len(summary.GPUs), 0, "There should be at least one GPU detected")
	} else {
		t.Log("No GPUs detected, which is acceptable in some environments")
	}
}

func TestPreflightCheck(t *testing.T) {
	imageName := testImg

	matchedIDs, unmatchedIDs, err := PreflightCheck(imageName)
	assert.Error(t, err, "PreflightCheck should return an error") // in a non-GPU environment
	assert.Nil(t, matchedIDs, "Matched IDs should not be nil")
	assert.Nil(t, unmatchedIDs, "Unmatched IDs should not be nil")
}

func TestGetSystemGPUInfo(t *testing.T) {
	stub := true
	opts := HwOptions{
		EnableStub: &stub,
		Timeout:    10, // Set a timeout of 10 seconds
	}

	summary, err := GetSystemGPUInfo(opts)
	assert.NoError(t, err, "GetSystemGPUInfo should not return an error")
	if summary != nil {
		assert.Greater(t, len(summary.GPUs), 0, "There should be at least one GPU detected")
	} else {
		t.Log("No GPUs detected, which is acceptable in some environments")
	}
}

func TestInspectCacheImage(t *testing.T) {
	imageName := testImg

	labels, err := InspectCacheImage(imageName)
	assert.NoError(t, err, "InspectCacheImage should not return an error")
	assert.NotNil(t, labels, "Labels should not be nil")
	assert.Greater(t, len(labels), 0, "Labels should contain at least one entry")
}

func TestExtractCacheWithInvalidImageName(t *testing.T) {
	opts := Options{
		ImageName: "invalid-image-name",
	}

	matchedIDs, unmatchedIDs, err := ExtractCache(opts)
	assert.Error(t, err, "ExtractCache should return an error for an invalid image name")
	assert.Nil(t, matchedIDs, "Matched IDs should be nil for an invalid image name")
	assert.Nil(t, unmatchedIDs, "Unmatched IDs should be nil for an invalid image name")
}

func TestInspectCacheImageWithInvalidImageName(t *testing.T) {
	imageName := "invalid-image-name"

	labels, err := InspectCacheImage(imageName)
	assert.Error(t, err, "InspectCacheImage should return an error for an invalid image name")
	assert.Nil(t, labels, "Labels should be nil for an invalid image name")
}

func TestExtractCacheWithCacheDir(t *testing.T) {
	cacheDir := "/tmp/test-cache-dir"
	opts := Options{
		ImageName: testImg,
		CacheDir:  cacheDir,
	}

	matchedIDs, unmatchedIDs, err := ExtractCache(opts)
	assert.NoError(t, err, "ExtractCache should not return an error")
	assert.Nil(t, matchedIDs, "Matched IDs should be nil")
	assert.Nil(t, unmatchedIDs, "Unmatched IDs should not be nil")
}

func TestExtractCacheWithGPUDisabled(t *testing.T) {
	gpu := false
	opts := Options{
		ImageName: testImg,
		EnableGPU: &gpu,
	}

	matchedIDs, unmatchedIDs, err := ExtractCache(opts)
	assert.NoError(t, err, "ExtractCache should not return an error")
	assert.Nil(t, matchedIDs, "Matched IDs should be nil")
	assert.Nil(t, unmatchedIDs, "Unmatched IDs should not be nil")
}

func TestGetSystemGPUInfoWithStubDisabled(t *testing.T) {
	stub := false
	opts := HwOptions{
		EnableStub: &stub,
		Timeout:    10,
	}

	summary, err := GetSystemGPUInfo(opts)
	assert.NoError(t, err, "GetSystemGPUInfo should not return an error")
	if summary != nil {
		assert.Greater(t, len(summary.GPUs), 0, "There should be at least one GPU detected")
	} else {
		t.Log("No GPUs detected, which is acceptable in some environments")
	}
}

func TestInspectCacheImageWithValidImageName(t *testing.T) {
	imageName := testImg

	labels, err := InspectCacheImage(imageName)
	assert.NoError(t, err, "InspectCacheImage should not return an error for a valid image name")
	assert.NotNil(t, labels, "Labels should not be nil for a valid image name")
	assert.Greater(t, len(labels), 0, "Labels should contain at least one entry for a valid image name")
}
