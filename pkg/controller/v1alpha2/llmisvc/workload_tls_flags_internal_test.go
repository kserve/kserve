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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

var (
	deprecatedTLSFlags = []string{
		"/app/pd-sidecar",
		"--secure-proxy=true",
		"--cert-path=/var/run/kserve/tls",
		"--decoder-use-tls=true",
		"--prefiller-use-tls=true",
	}
	enableTLSFlags = []string{
		"/app/pd-sidecar",
		"--secure-proxy=true",
		"--cert-path=/var/run/kserve/tls",
		"--enable-tls=decoder",
		"--enable-tls=prefiller",
	}
)

// sidecarPod returns a PodSpec whose init containers include a routing sidecar
// carrying the given command.
func sidecarPod(cmd []string) *corev1.PodSpec {
	return &corev1.PodSpec{
		InitContainers: []corev1.Container{
			{Name: constants.LLMISVCRoutingSidecarContainerName, Command: append([]string(nil), cmd...)},
		},
	}
}

func specWithVersion(version string, template *corev1.PodSpec) *v1alpha2.LLMInferenceServiceSpec {
	spec := &v1alpha2.LLMInferenceServiceSpec{}
	spec.Template = template
	if version != "" {
		spec.Annotations = map[string]string{constants.LLMDRouterDisaggSidecarVersionAnnotationKey: version}
	}
	return spec
}

func TestMigrateRoutingSidecars(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    []string
		wantErr bool
	}{
		{
			name:    "version is 0.10 migrate to --enable-tls flag",
			version: "0.10.0",
			want:    enableTLSFlags,
		},
		{
			name:    "version > 0.10 migrate to --enable-tls flag",
			version: "0.11.2",
			want:    enableTLSFlags,
		},
		{
			name:    "version 0.9 keeps deprecated flags",
			version: "0.9.0",
			want:    deprecatedTLSFlags,
		},
		{
			name:    "missing annotation defaults to 0.0.0 and keeps deprecated flags",
			version: "",
			want:    deprecatedTLSFlags,
		},
		{
			name:    "unparseable version (not semver) throw error",
			version: "not-a-version",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := specWithVersion(tt.version, sidecarPod(deprecatedTLSFlags))
			err := migrateRoutingSidecars(spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := spec.Template.InitContainers[0].Command
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMigrateRoutingSidecars_FlagForms(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		want    []string
	}{
		{
			name:    "pure flag and flag with space forms",
			command: []string{"/app/pd-sidecar", "--decoder-use-tls", "--prefiller-use-tls", "true"},
			want:    []string{"/app/pd-sidecar", "--enable-tls=decoder", "--enable-tls=prefiller"},
		},
		{
			name:    "flag set to 'false' should be dropped without replacement",
			command: []string{"/app/pd-sidecar", "--decoder-use-tls=false"},
			want:    []string{"/app/pd-sidecar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := specWithVersion("0.10.0", sidecarPod(tt.command))
			if err := migrateRoutingSidecars(spec); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := spec.Template.InitContainers[0].Command; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMigrateRoutingSidecars_NoSidecar verifies a pod without a routing sidecar
// is a no-op even at a version that would otherwise migrate.
func TestMigrateRoutingSidecars_NoSidecar(t *testing.T) {
	spec := specWithVersion("0.10.0", &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	})
	if err := migrateRoutingSidecars(spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
