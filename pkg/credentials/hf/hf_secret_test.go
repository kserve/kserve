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
