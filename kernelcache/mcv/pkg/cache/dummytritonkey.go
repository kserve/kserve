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

package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	logging "github.com/sirupsen/logrus"
)

const hashKey = "hash"

// getCacheInvalidatingEnvVars retrieves environment variables affecting cache using a Python Triton call.
func getCacheInvalidatingEnvVars() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", `
import json
try:
    from triton._C.libtriton import get_cache_invalidating_env_vars
    print(json.dumps(get_cache_invalidating_env_vars(), sort_keys=True))
except ImportError as e:
    print(json.dumps({}, sort_keys=True))
`)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var envVars map[string]string
	err = json.Unmarshal(output, &envVars)
	if err != nil {
		return nil, err
	}

	return envVars, nil
}

// getCurrentTarget retrieves the current target information from Triton.
func getCurrentTarget() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", `
import json
try:
    from triton.runtime.driver import driver
    target = driver.active.get_current_target()
    result = f"{target.backend}-{target.arch}-{target.warp_size}"
    print(json.dumps(result))
except Exception as e:
    print(json.dumps("unknown-backend-0-0"))
`)
	output, err := cmd.Output()
	if err != nil {
		return "unknown-backend-0-0", err
	}

	var target string
	err = json.Unmarshal(output, &target)
	if err != nil {
		return "unknown-backend-0-0", err
	}

	return target, nil
}

// getTritonInstallationKey retrieves the Triton installation fingerprint.
func getTritonInstallationKey() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", `
import json
try:
    import triton
    key = triton.compiler.compiler.triton_key()
    print(json.dumps(key, sort_keys=True))
except Exception:
    print(json.dumps("unknown-triton-key", sort_keys=True))
`)
	output, err := cmd.Output()
	if err != nil {
		return "unknown-triton-key", err
	}

	var key string
	err = json.Unmarshal(output, &key)
	if err != nil {
		return "unknown-triton-key", err
	}

	return key, nil
}

func generateSHA256(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// sortJSON ensures JSON objects have consistent key ordering before hashing.
func sortJSON(input interface{}) string {
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		logging.Fatalf("Failed to serialize JSON: %v", err)
	}
	return string(jsonBytes)
}

type Components map[string]interface{}

func generateTritonCacheKey(sourceHash string, data *TritonCacheData) (string, Components, error) {
	components := make(Components)

	tritonKey, err := getTritonInstallationKey()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get Triton installation fingerprint: %v", err)
	}
	components["triton_key"] = tritonKey

	if sourceHash == "" {
		sourceHash = "dummy_source_content"
	}
	components["source"] = map[string]string{
		"content": sourceHash,
		hashKey:   generateSHA256(sourceHash),
	}

	var backendInfo string
	if data != nil {
		// Use provided cache metadata instead of Python target call
		backendInfo = fmt.Sprintf("%s-%s-%d", data.Target.Backend, ConvertArchToString(data.Target.Arch), data.Target.WarpSize)
	} else {
		backendInfo, err = getCurrentTarget()
		if err != nil {
			return "", nil, fmt.Errorf("failed to get backend info: %v", err)
		}
	}
	components["backend"] = map[string]string{
		"info":  backendInfo,
		hashKey: generateSHA256(backendInfo),
	}

	// Build options from metadata if available
	var options map[string]interface{}
	if data != nil {
		options = map[string]interface{}{
			"num_warps":  data.NumWarps,
			"num_stages": data.NumStages,
			"debug":      data.Debug,
		}

		if data.PtxVersion != nil {
			options["ptx"] = *data.PtxVersion
		} else {
			options["ptx"] = 0
		}
	} else {
		options = map[string]interface{}{
			"num_warps":  4,
			"num_stages": 3,
			"debug":      os.Getenv("TRITON_DEBUG") == "1",
		}
	}
	sortedOptions := sortJSON(options)
	components["options"] = map[string]string{
		"values": sortedOptions,
		hashKey:  generateSHA256(sortedOptions),
	}

	envVars, err := getCacheInvalidatingEnvVars()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get environment variables: %v", err)
	}
	sortedEnvVars := sortJSON(envVars)
	components["environment"] = map[string]string{
		"variables": sortedEnvVars,
		hashKey:     generateSHA256(sortedEnvVars),
	}

	// Composite string used for final hash
	keyComponents := fmt.Sprintf("%s-%s-%s-%s-%s",
		components["triton_key"],
		components["source"].(map[string]string)[hashKey],
		components["backend"].(map[string]string)[hashKey],
		components["options"].(map[string]string)[hashKey],
		components["environment"].(map[string]string)[hashKey],
	)

	components["final_composite"] = keyComponents
	finalHash := generateSHA256(keyComponents)

	return finalHash, components, nil
}

func ComputeOneDummyTritonKey() (string, error) {
	logging.Debug("Generating Triton cache key...")
	cacheKey, components, err := generateTritonCacheKey("", nil)
	if err != nil {
		return "", fmt.Errorf("critical error during cache key generation: %v", err)
	}

	logging.Debug("\nCache Key Components Breakdown:")
	logging.Debug(strings.Repeat("-", 40))
	jsonComponents, _ := json.MarshalIndent(components, "", "  ")
	logging.Debug(string(jsonComponents))
	logging.Debugf("\nFinal Cache Key: %s", cacheKey)

	return cacheKey, nil
}

func ComputeDummyTritonKey(data *TritonCacheData) (string, error) {
	logging.Debugf("Generating Triton cache key for hash=%s...", data.Hash)

	cacheKey, components, err := generateTritonCacheKey("", data)
	if err != nil {
		return "", fmt.Errorf("critical error during cache key generation: %v", err)
	}

	logging.Debug("\nPer-Cache Dummy Key Components Breakdown:")
	logging.Debug(strings.Repeat("-", 40))
	jsonComponents, _ := json.MarshalIndent(components, "", "  ")
	logging.Debug(string(jsonComponents))
	logging.Debugf("\nFinal Per-Cache Cache Key: %s", cacheKey)

	return cacheKey, nil
}
