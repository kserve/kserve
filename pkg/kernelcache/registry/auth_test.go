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

package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func TestNewControllerServiceAccountAuthenticatorUsesProjectedToken(t *testing.T) {
	originalReadServiceAccountToken := readServiceAccountToken
	t.Cleanup(func() { readServiceAccountToken = originalReadServiceAccountToken })
	readServiceAccountToken = func(string) ([]byte, error) { return []byte("projected-token"), nil }

	authenticator, err := NewControllerServiceAccountAuthenticator(serviceAccountTokenRegistryConfig(), "registry.example:5000/team/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	require.NotNil(t, authenticator)
	config, err := authenticator.Authorization()
	require.NoError(t, err)
	require.Equal(t, "unused", config.Username)
	require.Equal(t, "projected-token", config.Password)
}

func TestNewControllerServiceAccountAuthenticatorRejectsRegistryOutsideConfiguredEndpoint(t *testing.T) {
	authenticator, err := NewControllerServiceAccountAuthenticator(serviceAccountTokenRegistryConfig(), "other.example:5000/team/cache:latest")
	require.Nil(t, authenticator)
	require.ErrorContains(t, err, "outside configured endpoint")
}

func TestNewControllerServiceAccountAuthenticatorReturnsNoAuthForNone(t *testing.T) {
	config := serviceAccountTokenRegistryConfig()
	config.Auth.Type = v1beta1.KernelCacheRegistryAuthTypeNone
	authenticator, err := NewControllerServiceAccountAuthenticator(config, "registry.example:5000/team/cache:latest")
	require.NoError(t, err)
	require.Nil(t, authenticator)
}

func TestNewControllerServiceAccountAuthenticatorReportsTokenReadFailure(t *testing.T) {
	originalReadServiceAccountToken := readServiceAccountToken
	t.Cleanup(func() { readServiceAccountToken = originalReadServiceAccountToken })
	readServiceAccountToken = func(string) ([]byte, error) { return nil, errors.New("not mounted") }

	authenticator, err := NewControllerServiceAccountAuthenticator(serviceAccountTokenRegistryConfig(), "registry.example:5000/team/cache:latest")
	require.Nil(t, authenticator)
	require.ErrorContains(t, err, "read controller ServiceAccount token")
}

func TestNewControllerRegistryAccessBuildsScopedAuthenticator(t *testing.T) {
	originalReadServiceAccountToken := readServiceAccountToken
	t.Cleanup(func() { readServiceAccountToken = originalReadServiceAccountToken })
	readServiceAccountToken = func(string) ([]byte, error) { return []byte("projected-token"), nil }

	registryAccess, err := NewControllerRegistryAccess(
		context.Background(),
		fake.NewClientBuilder().Build(),
		"workload",
		serviceAccountTokenRegistryConfig(),
		"registry.example:5000/team/cache@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)
	require.NoError(t, err)
	require.NotNil(t, registryAccess.Transport)
	require.False(t, registryAccess.Insecure)
	auth, err := registryAccess.Authenticator.Authorization()
	require.NoError(t, err)
	require.Equal(t, "projected-token", auth.Password)
}

func serviceAccountTokenRegistryConfig() v1beta1.KernelCacheRegistryConfig {
	return v1beta1.KernelCacheRegistryConfig{
		Endpoint: "registry.example:5000",
		Auth:     v1beta1.KernelCacheRegistryAuth{Type: v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken},
	}
}
