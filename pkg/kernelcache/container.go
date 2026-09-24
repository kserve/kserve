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

package kernelcache

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

const (
	vllmCacheRootEnv     = "VLLM_CACHE_ROOT"
	defaultVLLMCachePath = "/root/.cache/vllm"
	defaultOCIPath       = "io.vllm.cache"
)

// ResolveRuntimeContainerName resolves the container that owns a cache path.
// Standard KServe runtime container names are used when no override is provided.
func ResolveRuntimeContainerName(containers []corev1.Container, requested string) (string, error) {
	if requested != "" {
		if hasContainer(containers, requested) {
			return requested, nil
		}
		return "", fmt.Errorf("cache container %q was not found", requested)
	}

	for _, candidate := range []string{
		constants.InferenceServiceContainerName,
		constants.LLMInferenceServiceContainerName,
	} {
		if hasContainer(containers, candidate) {
			return candidate, nil
		}
	}

	return "", errors.New("containerName is required when no standard KServe runtime container is present")
}

// ResolveContainerPath resolves the cache path for a runtime container.
// An explicit path takes precedence over VLLM_CACHE_ROOT and the default path.
func ResolveContainerPath(container *corev1.Container, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	if container == nil {
		return "", errors.New("container is required when containerPath is not specified")
	}

	for _, env := range container.Env {
		if env.Name != vllmCacheRootEnv {
			continue
		}
		if env.ValueFrom != nil {
			return "", fmt.Errorf("%s uses valueFrom; provide containerPath explicitly", env.Name)
		}
		if env.Value == "" {
			return "", fmt.Errorf("%s is empty; provide containerPath explicitly", env.Name)
		}
		return env.Value, nil
	}

	return defaultVLLMCachePath, nil
}

// ResolveOCIPath resolves and validates an OCI cache path supported by the MCV
// image format. An empty value selects the default path.
func ResolveOCIPath(requested string) (string, error) {
	if requested == "" {
		return defaultOCIPath, nil
	}
	if requested != defaultOCIPath && requested != "io.triton.cache" {
		return "", fmt.Errorf("unsupported OCI path %q; supported paths are %q and %q", requested, defaultOCIPath, "io.triton.cache")
	}
	return requested, nil
}

func hasContainer(containers []corev1.Container, name string) bool {
	for _, container := range containers {
		if container.Name == name {
			return true
		}
	}
	return false
}
