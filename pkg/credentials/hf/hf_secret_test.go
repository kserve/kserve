/*
Copyright 2023 The KServe Authors.

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

package hf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildSecretEnvs_WithToken(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			HFTokenKey: []byte("my-secret-token"),
		},
	}

	envs := BuildSecretEnvs(secret)

	assert.Len(t, envs, 2)
	assert.Equal(t, HFTokenKey, envs[0].Name)
	assert.Equal(t, "my-secret-token", envs[0].Value)
	assert.Equal(t, HFTransfer, envs[1].Name)
	assert.Equal(t, "1", envs[1].Value)
}

func TestBuildSecretEnvs_WithoutToken(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{},
	}

	envs := BuildSecretEnvs(secret)

	assert.Empty(t, envs)
}

func TestBuildSecretEnvs_NilSecret(t *testing.T) {
	var secret *corev1.Secret = &corev1.Secret{
		Data: nil,
	}

	envs := BuildSecretEnvs(secret)

	assert.Empty(t, envs)
}
