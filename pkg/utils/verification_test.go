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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
)

func TestApplyVerificationContainerConfig(t *testing.T) {
	t.Parallel()

	digest := "sha256:abc123"

	tests := []struct {
		name         string
		digest       *string
		wantEnvName  string
		wantEnvValue string
		wantInjected bool
	}{
		{
			name:         "injects VERIFICATION_DIGEST when digest is set",
			digest:       &digest,
			wantEnvName:  constants.VerificationDigestEnvVar,
			wantEnvValue: digest,
			wantInjected: true,
		},
		{
			name:         "does not inject env var when digest is nil",
			digest:       nil,
			wantInjected: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			container := &corev1.Container{
				Name:  "storage-initializer",
				Image: "kserve/storage-initializer:latest",
			}

			ApplyVerificationContainerConfig(container, tt.digest)

			if tt.wantInjected {
				require.NotEmpty(t, container.Env, "expected env vars to be injected")
				var found bool
				for _, env := range container.Env {
					if env.Name == tt.wantEnvName {
						assert.Equal(t, tt.wantEnvValue, env.Value)
						found = true
						break
					}
				}
				assert.True(t, found, "expected env var %q to be present", tt.wantEnvName)
			} else {
				for _, env := range container.Env {
					assert.NotEqual(t, constants.VerificationDigestEnvVar, env.Name,
						"expected %q not to be injected when digest is nil", constants.VerificationDigestEnvVar)
				}
			}
		})
	}
}

func TestApplyVerificationContainerConfig_UpdatesExistingEnvVar(t *testing.T) {
	t.Parallel()

	oldDigest := "sha256:old"
	newDigest := "sha256:new"

	container := &corev1.Container{
		Name: "storage-initializer",
		Env: []corev1.EnvVar{
			{Name: constants.VerificationDigestEnvVar, Value: oldDigest},
		},
	}

	ApplyVerificationContainerConfig(container, &newDigest)

	var count int
	for _, env := range container.Env {
		if env.Name == constants.VerificationDigestEnvVar {
			assert.Equal(t, newDigest, env.Value)
			count++
		}
	}
	assert.Equal(t, 1, count, "expected exactly one VERIFICATION_DIGEST env var after update")
}

func TestApplyVerificationContainerConfig_DoesNotModifyOtherEnvVars(t *testing.T) {
	t.Parallel()

	digest := "sha256:abc"
	container := &corev1.Container{
		Name: "storage-initializer",
		Env: []corev1.EnvVar{
			{Name: "STORAGE_URI", Value: "s3://bucket/model"},
			{Name: "UNRELATED_VAR", Value: "unchanged"},
		},
	}

	ApplyVerificationContainerConfig(container, &digest)

	// Pre-existing env vars must be untouched.
	envMap := make(map[string]string)
	for _, e := range container.Env {
		envMap[e.Name] = e.Value
	}
	assert.Equal(t, "s3://bucket/model", envMap["STORAGE_URI"])
	assert.Equal(t, "unchanged", envMap["UNRELATED_VAR"])
	assert.Equal(t, digest, envMap[constants.VerificationDigestEnvVar])
}
