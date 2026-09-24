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
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

func TestResolveRuntimeContainerName(t *testing.T) {
	tests := []struct {
		name        string
		containers  []corev1.Container
		requested   string
		want        string
		wantFailure bool
	}{
		{
			name:       "uses explicit container override",
			containers: []corev1.Container{{Name: "custom-runtime"}},
			requested:  "custom-runtime",
			want:       "custom-runtime",
		},
		{
			name:       "defaults to inference service container",
			containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
			want:       constants.InferenceServiceContainerName,
		},
		{
			name:       "falls back to llm inference service container",
			containers: []corev1.Container{{Name: constants.LLMInferenceServiceContainerName}},
			want:       constants.LLMInferenceServiceContainerName,
		},
		{
			name:        "rejects missing explicit container",
			containers:  []corev1.Container{{Name: constants.InferenceServiceContainerName}},
			requested:   "custom-runtime",
			wantFailure: true,
		},
		{
			name:        "rejects when no standard container is present",
			containers:  []corev1.Container{{Name: "custom-runtime"}},
			wantFailure: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveRuntimeContainerName(test.containers, test.requested)
			if test.wantFailure {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveContainerPath(t *testing.T) {
	tests := []struct {
		name        string
		container   corev1.Container
		requested   string
		want        string
		wantFailure bool
	}{
		{
			name:      "uses explicit path",
			container: corev1.Container{Env: []corev1.EnvVar{{Name: vllmCacheRootEnv, Value: "/env/cache"}}},
			requested: "/explicit/cache",
			want:      "/explicit/cache",
		},
		{
			name:      "uses runtime environment",
			container: corev1.Container{Env: []corev1.EnvVar{{Name: vllmCacheRootEnv, Value: "/env/cache"}}},
			want:      "/env/cache",
		},
		{
			name: "uses fallback",
			want: defaultVLLMCachePath,
		},
		{
			name: "rejects valueFrom",
			container: corev1.Container{Env: []corev1.EnvVar{{
				Name:      vllmCacheRootEnv,
				ValueFrom: &corev1.EnvVarSource{},
			}}},
			wantFailure: true,
		},
		{
			name:        "rejects empty environment value",
			container:   corev1.Container{Env: []corev1.EnvVar{{Name: vllmCacheRootEnv}}},
			wantFailure: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveContainerPath(&test.container, test.requested)
			if test.wantFailure {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveContainerPathRejectsNilContainer(t *testing.T) {
	_, err := ResolveContainerPath(nil, "")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestResolveOCIPath(t *testing.T) {
	tests := []struct {
		name        string
		requested   string
		want        string
		wantFailure bool
	}{
		{name: "uses vllm default", want: "io.vllm.cache"},
		{name: "accepts vllm path", requested: "io.vllm.cache", want: "io.vllm.cache"},
		{name: "accepts triton path", requested: tritonOCIPath, want: tritonOCIPath},
		{name: "rejects unsupported path", requested: "custom/cache", wantFailure: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveOCIPath(test.requested)
			if test.wantFailure {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
