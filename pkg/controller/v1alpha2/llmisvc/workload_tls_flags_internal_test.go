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

package llmisvc

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestMigrateRoutingSidecarTLSFlags(t *testing.T) {
	deprecatedFlags := []string{
		"/app/pd-sidecar",
		"--secure-proxy=true",
		"--cert-path=/var/run/kserve/tls",
		"--decoder-use-tls=true",
		"--prefiller-use-tls=true",
	}
	enableTLSFlags := []string{
		"/app/pd-sidecar",
		"--secure-proxy=true",
		"--cert-path=/var/run/kserve/tls",
		"--enable-tls=decoder",
		"--enable-tls=prefiller",
	}

	tests := []struct {
		name        string
		annotations map[string]string
		command     []string
		want        []string
		wantErr     bool
	}{
		{
			name:        "version is 0.10 migrate to --enable-tls flag",
			annotations: map[string]string{"app.kubernetes.io/version": "0.10.0"},
			command:     append([]string(nil), deprecatedFlags...),
			want:        enableTLSFlags,
		},
		{
			name:        "version > 0.10 migrate to --enable-tls flag",
			annotations: map[string]string{"app.kubernetes.io/version": "0.11.2"},
			command:     append([]string(nil), deprecatedFlags...),
			want:        enableTLSFlags,
		},
		{
			name:        "version 0.9 keeps deprecated flags",
			annotations: map[string]string{"app.kubernetes.io/version": "0.9.0"},
			command:     append([]string(nil), deprecatedFlags...),
			want:        deprecatedFlags,
		},
		{
			name:        "missing annotation defaults to 0.0.0 and keeps deprecated flags",
			annotations: nil,
			command:     append([]string(nil), deprecatedFlags...),
			want:        deprecatedFlags,
		},
		{
			name:        "unparseable version (not semver) throw error",
			annotations: map[string]string{"app.kubernetes.io/version": "not-a-version"},
			command:     append([]string(nil), deprecatedFlags...),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &corev1.Container{Command: tt.command}
			err := migrateRoutingSidecarTLSFlags(tt.annotations, c)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(c.Command, tt.want) {
				t.Errorf("command = %q, want %q", c.Command, tt.want)
			}
		})
	}
}

func TestMigrateRoutingSidecarTLSFlags_NilContainer(t *testing.T) {
	// Must not panic and must not error.
	if err := migrateRoutingSidecarTLSFlags(map[string]string{"app.kubernetes.io/version": "0.10.0"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
