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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMountImagePullSecretsAsDockerConfig(t *testing.T) {
	t.Run("zero secrets is a no-op", func(t *testing.T) {
		container := &corev1.Container{Name: "storage-initializer"}
		var volumes []corev1.Volume
		MountImagePullSecretsAsDockerConfig(nil, container, &volumes)
		assert.Empty(t, volumes)
		assert.Empty(t, container.VolumeMounts)
		assert.Empty(t, container.Env)
	})

	t.Run("single secret adds volume, mount and config-path env", func(t *testing.T) {
		container := &corev1.Container{Name: "storage-initializer"}
		var volumes []corev1.Volume
		secrets := []corev1.LocalObjectReference{{Name: "reg-cred"}}
		MountImagePullSecretsAsDockerConfig(secrets, container, &volumes)

		require.Len(t, volumes, 1)
		require.NotNil(t, volumes[0].Secret)
		assert.Equal(t, "reg-cred", volumes[0].Secret.SecretName)
		require.NotNil(t, volumes[0].Secret.DefaultMode)
		assert.Equal(t, int32(0o400), *volumes[0].Secret.DefaultMode)
		require.Len(t, container.VolumeMounts, 1)
		assert.Equal(t, OciFetchDockerConfigDir, container.VolumeMounts[0].MountPath)
		require.Len(t, container.Env, 1)
		assert.Equal(t, OciFetchDockerConfigPathEnvVar, container.Env[0].Name)
		assert.Equal(t, OciFetchDockerConfigDir+"/config.json", container.Env[0].Value)
	})

	t.Run("multiple secrets use the first named secret", func(t *testing.T) {
		container := &corev1.Container{Name: "storage-initializer"}
		var volumes []corev1.Volume
		secrets := []corev1.LocalObjectReference{{Name: ""}, {Name: "first"}, {Name: "second"}}
		MountImagePullSecretsAsDockerConfig(secrets, container, &volumes)
		require.Len(t, volumes, 1)
		require.NotNil(t, volumes[0].Secret)
		assert.Equal(t, "first", volumes[0].Secret.SecretName)
	})
}

func TestFetchAndValidateDockerConfigJSONSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	t.Run("missing secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		err := FetchAndValidateDockerConfigJSONSecret(context.Background(), cl, "ns", "reg-cred")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("wrong type", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "reg-cred", Namespace: "ns"},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{}`)},
		}).Build()
		err := FetchAndValidateDockerConfigJSONSecret(context.Background(), cl, "ns", "reg-cred")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "has type")
	})

	t.Run("missing key", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "reg-cred", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
		}).Build()
		err := FetchAndValidateDockerConfigJSONSecret(context.Background(), cl, "ns", "reg-cred")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing key")
	})

	t.Run("valid secret", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "reg-cred", Namespace: "ns"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
		}).Build()
		require.NoError(t, FetchAndValidateDockerConfigJSONSecret(context.Background(), cl, "ns", "reg-cred"))
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
