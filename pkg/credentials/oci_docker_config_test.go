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

package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestMountImagePullSecretsAsDockerConfig(t *testing.T) {
	t.Run("zero secrets is a no-op", func(t *testing.T) {
		container := &corev1.Container{Name: "storage-initializer"}
		var volumes []corev1.Volume
		require.NoError(t, MountImagePullSecretsAsDockerConfig(nil, container, &volumes))
		assert.Empty(t, volumes)
		assert.Empty(t, container.VolumeMounts)
		assert.Empty(t, container.Env)
	})

	t.Run("single secret adds volume, mount and config-path env", func(t *testing.T) {
		container := &corev1.Container{Name: "storage-initializer"}
		var volumes []corev1.Volume
		secrets := []corev1.LocalObjectReference{{Name: "reg-cred"}}
		require.NoError(t, MountImagePullSecretsAsDockerConfig(secrets, container, &volumes))

		require.Len(t, volumes, 1)
		require.NotNil(t, volumes[0].Secret)
		assert.Equal(t, "reg-cred", volumes[0].Secret.SecretName)
		require.Len(t, container.VolumeMounts, 1)
		assert.Equal(t, OciFetchDockerConfigDir, container.VolumeMounts[0].MountPath)
		require.Len(t, container.Env, 1)
		assert.Equal(t, OciFetchDockerConfigPathEnvVar, container.Env[0].Name)
		assert.Equal(t, OciFetchDockerConfigDir+"/config.json", container.Env[0].Value)
		assert.Nil(t, volumes[0].Secret.DefaultMode, "0400 is unreadable by UID 1000 on root-owned secret files")
	})

	t.Run("multiple secrets use the first", func(t *testing.T) {
		container := &corev1.Container{Name: "storage-initializer"}
		var volumes []corev1.Volume
		secrets := []corev1.LocalObjectReference{{Name: "first"}, {Name: "second"}}
		require.NoError(t, MountImagePullSecretsAsDockerConfig(secrets, container, &volumes))
		require.Len(t, volumes, 1)
		require.NotNil(t, volumes[0].Secret)
		assert.Equal(t, "first", volumes[0].Secret.SecretName)
	})
}

func TestSetOciInsecureRegistryEnv(t *testing.T) {
	t.Run("sets env once", func(t *testing.T) {
		container := &corev1.Container{Name: "storage-initializer"}
		SetOciInsecureRegistryEnv(container)
		SetOciInsecureRegistryEnv(container)
		require.Len(t, container.Env, 1)
		assert.Equal(t, OciInsecureRegistryEnvVar, container.Env[0].Name)
		assert.Equal(t, "true", container.Env[0].Value)
	})
}
