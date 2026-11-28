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

package jobs

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

func TestInjectHTTPStorageURIAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		sourceURI string
		allowlist []string
		container corev1.Container
		wantEnv   *corev1.EnvVar
	}{
		{
			name:      "adds policy for HTTP source",
			sourceURI: "https://models.example/model",
			allowlist: []string{"models.example"},
			wantEnv:   &corev1.EnvVar{Name: constants.HTTPStorageURIAllowlistEnvVar, Value: `["models.example"]`},
		},
		{
			name:      "clears inherited policy for HTTP source",
			sourceURI: "http://models.example/model",
			container: corev1.Container{Env: []corev1.EnvVar{{Name: constants.HTTPStorageURIAllowlistEnvVar, Value: "old"}}},
			wantEnv:   &corev1.EnvVar{Name: constants.HTTPStorageURIAllowlistEnvVar, Value: `[]`},
		},
		{
			name:      "leaves non-HTTP source unchanged",
			sourceURI: "s3://bucket/model",
			container: corev1.Container{Env: []corev1.EnvVar{{Name: "CUSTOM", Value: "kept"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := tt.container.DeepCopy()
			if err := InjectHTTPStorageURIAllowlist(container, tt.sourceURI, tt.allowlist); err != nil {
				t.Fatalf("InjectHTTPStorageURIAllowlist() error = %v", err)
			}
			if tt.wantEnv == nil {
				if len(container.Env) != len(tt.container.Env) {
					t.Fatalf("env length = %d, want unchanged length %d", len(container.Env), len(tt.container.Env))
				}
				return
			}
			found := false
			for _, env := range container.Env {
				if env.Name == tt.wantEnv.Name && env.Value == tt.wantEnv.Value {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("env = %v, want %v", container.Env, *tt.wantEnv)
			}
		})
	}
}
