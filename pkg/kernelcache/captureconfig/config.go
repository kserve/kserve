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

// Package captureconfig defines the JSON contracts exchanged between the
// KernelCache webhook/controller and the MCV capture sidecar.
package captureconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

const (
	// CurrentVersion is the capture configuration wire format version.
	CurrentVersion = 1

	// CaptureModeEnv enables capture orchestration in the MCV sidecar.
	CaptureModeEnv = "MCV_CAPTURE_MODE"
	// CaptureConfigEnv contains the JSON capture configuration.
	CaptureConfigEnv = "MCV_CAPTURE_CONFIG"
	// ReadinessConfigEnv contains the JSON workload readiness configuration.
	ReadinessConfigEnv = "MCV_READINESS_CONFIG"
	// RuntimeInfoEnv contains the JSON runtime identity information.
	RuntimeInfoEnv = "MCV_RUNTIME_INFO"
)

// CaptureConfig contains the values shared by the webhook, controller, and
// capture entrypoint for one capture session.
type CaptureConfig struct {
	// Version identifies the JSON wire format used by the capture sidecar.
	Version int `json:"version"`
	// CacheDir is the mounted cache directory that MCV snapshots and packages.
	CacheDir string `json:"cacheDir,omitempty"`
	// TargetImage is the OCI image destination for the captured cache.
	TargetImage string `json:"targetImage,omitempty"`
	// Capture identifies the KernelCacheCapture resource and capture session.
	Capture CaptureIdentity `json:"capture"`
	// CachePaths describes the cache directories included in the captured image.
	CachePaths []v1alpha1.KernelCachePath `json:"cachePaths,omitempty"`
}

// CaptureIdentity identifies a capture session.
type CaptureIdentity struct {
	// Name is the name of the KernelCacheCapture resource.
	Name string `json:"name"`
	// Namespace is the namespace of the KernelCacheCapture resource.
	Namespace string `json:"namespace"`
	// SessionID identifies the active capture attempt for the resource.
	SessionID string `json:"sessionID"`
}

// ReadinessConfig configures the workload readiness check.
type ReadinessConfig struct {
	// URL is the HTTP readiness endpoint polled by the capture sidecar.
	URL string `json:"url"`
	// MCVCaptureReadinessTimeoutSeconds is the maximum readiness wait time.
	MCVCaptureReadinessTimeoutSeconds int64 `json:"mcvCaptureReadinessTimeoutSeconds"`
}

// RuntimeInfo contains runtime identity values passed to the capture sidecar
// and copied into the capture result.
type RuntimeInfo struct {
	// CommandHash identifies the runtime container command.
	CommandHash string `json:"commandHash,omitempty"`
	// ArgsHash identifies the runtime container arguments.
	ArgsHash string `json:"argsHash,omitempty"`
	// ModelURIHash identifies the model URI used by the workload.
	ModelURIHash string `json:"modelURIHash,omitempty"`
}

// MarshalCaptureConfig serializes and validates a capture configuration.
func MarshalCaptureConfig(config CaptureConfig) (string, error) {
	if config.Version == 0 {
		config.Version = CurrentVersion
	}
	if err := validateCaptureConfig(config); err != nil {
		return "", fmt.Errorf("marshal capture config: %w", err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal capture config: %w", err)
	}
	return string(data), nil
}

// ParseCaptureConfig parses and validates a capture configuration.
func ParseCaptureConfig(value string) (CaptureConfig, error) {
	var config CaptureConfig
	if err := decode(value, &config); err != nil {
		return config, fmt.Errorf("parse capture config: %w", err)
	}
	if config.Version != CurrentVersion {
		return config, fmt.Errorf("unsupported capture config version %d", config.Version)
	}
	if err := validateCaptureConfig(config); err != nil {
		return config, fmt.Errorf("parse capture config: %w", err)
	}
	return config, nil
}

// MarshalReadinessConfig serializes readiness configuration.
func MarshalReadinessConfig(config ReadinessConfig) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal readiness config: %w", err)
	}
	return string(data), nil
}

// MarshalRuntimeInfo serializes runtime information.
func MarshalRuntimeInfo(info RuntimeInfo) (string, error) {
	data, err := json.Marshal(info)
	if err != nil {
		return "", fmt.Errorf("marshal runtime info: %w", err)
	}
	return string(data), nil
}

func validateCaptureConfig(config CaptureConfig) error {
	if config.CacheDir == "" {
		return errors.New("cacheDir is required")
	}
	if config.TargetImage == "" {
		return errors.New("targetImage is required")
	}
	if config.Capture.Name == "" {
		return errors.New("capture.name is required")
	}
	if config.Capture.Namespace == "" {
		return errors.New("capture.namespace is required")
	}
	if config.Capture.SessionID == "" {
		return errors.New("capture.sessionID is required")
	}
	if len(config.CachePaths) == 0 {
		return errors.New("cachePaths is required")
	}
	for index, cachePath := range config.CachePaths {
		if cachePath.ContainerName == "" {
			return fmt.Errorf("cachePaths[%d].containerName is required", index)
		}
		if cachePath.ContainerPath == "" {
			return fmt.Errorf("cachePaths[%d].containerPath is required", index)
		}
		if cachePath.OCIPath == "" {
			return fmt.Errorf("cachePaths[%d].ociPath is required", index)
		}
	}
	return nil
}

func decode(value string, target any) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("value is empty")
	}
	if err := json.Unmarshal([]byte(value), target); err != nil {
		return err
	}
	return nil
}
