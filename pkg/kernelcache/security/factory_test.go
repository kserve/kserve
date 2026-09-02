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

package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVerifier(t *testing.T) {
	t.Run("disabled returns a working no-op", func(t *testing.T) {
		v, err := NewVerifier(SecurityConfig{Mode: ModeDisabled})
		require.NoError(t, err)
		require.NotNil(t, v)

		res, err := v.Verify(context.Background(), VerifyRequest{ImageRef: "registry/img:tag"})
		require.NoError(t, err)
		assert.False(t, res.Verified)
		assert.Equal(t, ModeDisabled, res.Mode)
	})

	t.Run("empty mode defaults to disabled", func(t *testing.T) {
		v, err := NewVerifier(SecurityConfig{})
		require.NoError(t, err)
		assert.IsType(t, noopVerifier{}, v)
	})

	t.Run("unknown mode errors", func(t *testing.T) {
		v, err := NewVerifier(SecurityConfig{Mode: "bogus"})
		assert.Nil(t, v)
		assert.ErrorIs(t, err, ErrUnknownMode)
	})
}
