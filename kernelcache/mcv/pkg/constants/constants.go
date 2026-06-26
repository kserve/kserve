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

package constants

import (
	"os"
	"path/filepath"

	logging "github.com/sirupsen/logrus"
)

// Core default paths and environment keys
const (
	VLLM             = "vllm"
	Triton           = "triton"
	MCVBuildDir      = "/tmp/.mcv"
	CacheDir         = "cache"
	ManifestDir      = "manifest"
	ManifestFileName = "manifest.json"
	VLLMHOME         = "/home/vllm"
	VLLMCache        = ".cache/vllm"

	MCVTritonCacheDir    = "io.triton.cache/"
	MCVTritonManifestDir = "io.triton.manifest"
	MCVVLLMCacheDir      = "io.vllm.cache"
	MCVVLLMManifestDir   = "io.vllm.manifest"

	EnvTritonCacheDir    = "TRITON_CACHE_DIR"
	DefaultCacheFilePath = "/tmp/device_cache.json"
	StubbedCacheFile     = "/tmp/device_cache_stub.json"
)

// Configurable runtime paths
var (
	TritonCacheDir     string
	ExtractCacheDir    string
	ExtractManifestDir string
	VLLMCacheDir       string
	HasTritonCache     bool
	HasVLLMCache       bool
	LogLevels          = []string{"debug", "info", "warning", "error"} // accepted log levels
)

func init() {
	HasTritonCache = false
	HasVLLMCache = false
	ExtractCacheDir = ""
	// Derive user's home directory as the Triton/vLLM caches are stored somewhere here.
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		logging.Warnf("Failed to determine user home dir, falling back to /tmp: %v", err)
		home = "/tmp"
	}

	// Determine Triton cache directory
	if val := os.Getenv(EnvTritonCacheDir); val != "" {
		TritonCacheDir = val
	} else {
		TritonCacheDir = filepath.Join(home, ".triton", "cache")
	}
	if _, err := os.Stat(TritonCacheDir); err == nil {
		HasTritonCache = true
	}

	VLLMCacheDir = filepath.Join(home, VLLMCache)
	if _, err := os.Stat(VLLMCacheDir); err == nil {
		HasVLLMCache = true
	}
}
