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

package captureconfig

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestCaptureConfigRoundTrip(t *testing.T) {
	want := CaptureConfig{
		Version:     CurrentVersion,
		CacheDir:    "/workspace/cache/0",
		TargetImage: "registry.example/team/cache:session",
		Capture: CaptureIdentity{
			Name:      "capture",
			Namespace: "team",
			SessionID: "session-id",
		},
		CachePaths: []v1alpha1.KernelCachePath{{
			ContainerName: "kserve-container",
			ContainerPath: "/tmp/vllm",
			OCIPath:       "io.vllm.cache",
		}},
	}

	value, err := MarshalCaptureConfig(want)
	require.NoError(t, err)
	got, err := ParseCaptureConfig(value)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestParseCaptureConfigRejectsUnsupportedVersion(t *testing.T) {
	_, err := ParseCaptureConfig(`{"version":2}`)
	require.EqualError(t, err, "unsupported capture config version 2")
}

func TestMarshalCaptureConfigRejectsMissingRequiredFields(t *testing.T) {
	valid := CaptureConfig{
		Version:     CurrentVersion,
		CacheDir:    "/workspace/cache/0",
		TargetImage: "registry.example/team/cache:session",
		Capture: CaptureIdentity{
			Name:      "capture",
			Namespace: "team",
			SessionID: "session-id",
		},
		CachePaths: []v1alpha1.KernelCachePath{{
			ContainerName: "kserve-container",
			ContainerPath: "/tmp/vllm",
			OCIPath:       "io.vllm.cache",
		}},
	}
	tests := []struct {
		name   string
		mutate func(*CaptureConfig)
		err    string
	}{
		{name: "cache dir", mutate: func(config *CaptureConfig) { config.CacheDir = "" }, err: "cacheDir is required"},
		{name: "target image", mutate: func(config *CaptureConfig) { config.TargetImage = "" }, err: "targetImage is required"},
		{name: "capture name", mutate: func(config *CaptureConfig) { config.Capture.Name = "" }, err: "capture.name is required"},
		{name: "capture namespace", mutate: func(config *CaptureConfig) { config.Capture.Namespace = "" }, err: "capture.namespace is required"},
		{name: "capture session", mutate: func(config *CaptureConfig) { config.Capture.SessionID = "" }, err: "capture.sessionID is required"},
		{name: "cache paths", mutate: func(config *CaptureConfig) { config.CachePaths = nil }, err: "cachePaths is required"},
		{name: "container name", mutate: func(config *CaptureConfig) { config.CachePaths[0].ContainerName = "" }, err: "cachePaths[0].containerName is required"},
		{name: "container path", mutate: func(config *CaptureConfig) { config.CachePaths[0].ContainerPath = "" }, err: "cachePaths[0].containerPath is required"},
		{name: "oci path", mutate: func(config *CaptureConfig) { config.CachePaths[0].OCIPath = "" }, err: "cachePaths[0].ociPath is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			config.CachePaths = append([]v1alpha1.KernelCachePath(nil), valid.CachePaths...)
			test.mutate(&config)
			_, err := MarshalCaptureConfig(config)
			require.EqualError(t, err, "marshal capture config: "+test.err)
		})
	}
}

func TestParseCaptureConfigRejectsMissingRequiredFields(t *testing.T) {
	_, err := ParseCaptureConfig(`{"version":1,"cacheDir":"/workspace/cache/0","targetImage":"registry.example/team/cache:session","capture":{"name":"capture","namespace":"team","sessionID":"session-id"}}`)
	require.EqualError(t, err, "parse capture config: cachePaths is required")
}

func TestParseCaptureConfigRejectsInvalidJSON(t *testing.T) {
	_, err := ParseCaptureConfig(`{"version":1,`)
	require.Error(t, err)
}

func TestMarshalReadinessConfig(t *testing.T) {
	value, err := MarshalReadinessConfig(ReadinessConfig{
		URL:                               "http://127.0.0.1:8080/health",
		MCVCaptureReadinessTimeoutSeconds: 600,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"url":"http://127.0.0.1:8080/health","mcvCaptureReadinessTimeoutSeconds":600}`, value)
}

func TestMarshalReadinessConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		config ReadinessConfig
		err    string
	}{
		{name: "missing URL", config: ReadinessConfig{MCVCaptureReadinessTimeoutSeconds: 600}, err: "url is required"},
		{name: "whitespace URL", config: ReadinessConfig{URL: "  ", MCVCaptureReadinessTimeoutSeconds: 600}, err: "url is required"},
		{name: "zero timeout", config: ReadinessConfig{URL: "http://127.0.0.1:8080/health"}, err: "mcvCaptureReadinessTimeoutSeconds must be greater than zero"},
		{name: "negative timeout", config: ReadinessConfig{URL: "http://127.0.0.1:8080/health", MCVCaptureReadinessTimeoutSeconds: -1}, err: "mcvCaptureReadinessTimeoutSeconds must be greater than zero"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := MarshalReadinessConfig(test.config)
			require.EqualError(t, err, "marshal readiness config: "+test.err)
		})
	}
}

func TestMarshalRuntimeInfo(t *testing.T) {
	value, err := MarshalRuntimeInfo(RuntimeInfo{
		CommandHash:  "command-hash",
		ArgsHash:     "args-hash",
		ModelURIHash: "model-uri-hash",
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"commandHash":"command-hash","argsHash":"args-hash","modelURIHash":"model-uri-hash"}`, value)
}
