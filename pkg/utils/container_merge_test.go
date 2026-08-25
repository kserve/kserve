/*
Copyright 2025 The KServe Authors.

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
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMergeContainerWithPatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		base    corev1.Container
		overlay corev1.Container
		check   func(t *testing.T, merged corev1.Container)
	}{
		{
			name: "overlay adds env vars",
			base: corev1.Container{
				Name:  "storage-initializer",
				Image: "kserve/storage-initializer:latest",
				Args:  []string{"hf://org/model", "/mnt/models"},
			},
			overlay: corev1.Container{
				Name: "storage-initializer",
				Env: []corev1.EnvVar{
					{Name: "HF_XET_HIGH_PERFORMANCE", Value: "0"},
				},
			},
			check: func(t *testing.T, merged corev1.Container) {
				assert.Equal(t, "storage-initializer", merged.Name)
				require.Len(t, merged.Env, 1)
				assert.Equal(t, "HF_XET_HIGH_PERFORMANCE", merged.Env[0].Name)
				assert.Equal(t, "0", merged.Env[0].Value)
			},
		},
		{
			name: "overlay overrides image",
			base: corev1.Container{
				Name:  "storage-initializer",
				Image: "kserve/storage-initializer:latest",
			},
			overlay: corev1.Container{
				Name:  "storage-initializer",
				Image: "my-registry/custom-init:v2",
			},
			check: func(t *testing.T, merged corev1.Container) {
				assert.Equal(t, "my-registry/custom-init:v2", merged.Image)
			},
		},
		{
			name: "overlay overrides resources",
			base: corev1.Container{
				Name:  "init",
				Image: "img:v1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			overlay: corev1.Container{
				Name: "init",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
			check: func(t *testing.T, merged corev1.Container) {
				assert.Equal(t, resource.MustParse("1Gi"), merged.Resources.Limits[corev1.ResourceMemory])
			},
		},
		{
			name: "name restored from base when overlay has empty name",
			base: corev1.Container{
				Name:  "storage-initializer",
				Image: "img:v1",
			},
			overlay: corev1.Container{
				Image: "img:v2",
			},
			check: func(t *testing.T, merged corev1.Container) {
				assert.Equal(t, "storage-initializer", merged.Name)
				assert.Equal(t, "img:v2", merged.Image)
			},
		},
		{
			name: "valueFrom_overrides_value_on_same_name",
			base: corev1.Container{
				Name: "init",
				Env: []corev1.EnvVar{
					{Name: "AWS_KEY", Value: "literal-secret"},
				},
			},
			overlay: corev1.Container{
				Name: "init",
				Env: []corev1.EnvVar{
					{Name: "AWS_KEY", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "aws-creds"},
							Key:                  "access-key",
						},
					}},
				},
			},
			check: func(t *testing.T, merged corev1.Container) {
				require.Len(t, merged.Env, 1)
				assert.Equal(t, "AWS_KEY", merged.Env[0].Name)
				assert.Empty(t, merged.Env[0].Value, "Value should be cleared when overlay uses ValueFrom")
				assert.NotNil(t, merged.Env[0].ValueFrom)
			},
		},
		{
			name: "value_overrides_valueFrom_on_same_name",
			base: corev1.Container{
				Name: "init",
				Env: []corev1.EnvVar{
					{Name: "TOKEN", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "secret"},
							Key:                  "token",
						},
					}},
				},
			},
			overlay: corev1.Container{
				Name: "init",
				Env: []corev1.EnvVar{
					{Name: "TOKEN", Value: "hardcoded-value"},
				},
			},
			check: func(t *testing.T, merged corev1.Container) {
				require.Len(t, merged.Env, 1)
				assert.Equal(t, "TOKEN", merged.Env[0].Name)
				assert.Equal(t, "hardcoded-value", merged.Env[0].Value)
				assert.Nil(t, merged.Env[0].ValueFrom, "ValueFrom should be cleared when overlay uses Value")
			},
		},
		{
			name: "disjoint envs with different value sources",
			base: corev1.Container{
				Name: "init",
				Env: []corev1.EnvVar{
					{Name: "EXISTING", Value: "keep-me"},
				},
			},
			overlay: corev1.Container{
				Name: "init",
				Env: []corev1.EnvVar{
					{Name: "NEW_SECRET", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "s"},
							Key:                  "k",
						},
					}},
				},
			},
			check: func(t *testing.T, merged corev1.Container) {
				require.Len(t, merged.Env, 2)
				for _, e := range merged.Env {
					assert.False(t, e.Value != "" && e.ValueFrom != nil,
						"env %q must not have both Value and ValueFrom", e.Name)
				}
			},
		},
		{
			name: "args come from strategic merge (not forced)",
			base: corev1.Container{
				Name: "init",
				Args: []string{"--base-flag"},
			},
			overlay: corev1.Container{
				Name: "init",
				Args: []string{"--overlay-flag"},
			},
			check: func(t *testing.T, merged corev1.Container) {
				// SMP replaces args list; callers decide post-merge policy
				assert.Equal(t, []string{"--overlay-flag"}, merged.Args)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			merged, err := MergeContainerWithPatch(tc.base, tc.overlay)
			require.NoError(t, err)
			tc.check(t, merged)
		})
	}
}
