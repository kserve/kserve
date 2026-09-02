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

package fixture

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeSecretSource(t *testing.T) {
	src := &FakeSecretSource{
		Secrets:    map[string]map[string][]byte{"kserve/ca": {"ca.crt": []byte("PEM")}},
		ConfigMaps: map[string]map[string]string{"kserve/root": {"trusted_root.json": "{}"}},
	}
	ctx := context.Background()

	t.Run("get secret and configmap", func(t *testing.T) {
		s, err := src.GetSecret(ctx, "kserve/ca")
		require.NoError(t, err)
		assert.Equal(t, []byte("PEM"), s["ca.crt"])
		cm, err := src.GetConfigMap(ctx, "kserve/root")
		require.NoError(t, err)
		assert.Equal(t, "{}", cm["trusted_root.json"])
	})

	t.Run("missing ref errors", func(t *testing.T) {
		_, err := src.GetSecret(ctx, "kserve/nope")
		assert.Error(t, err)
		_, err = src.GetConfigMap(ctx, "kserve/nope")
		assert.Error(t, err)
	})

	t.Run("returned map and bytes are copies", func(t *testing.T) {
		s, err := src.GetSecret(ctx, "kserve/ca")
		require.NoError(t, err)
		delete(s, "ca.crt")
		s["injected"] = []byte("x")
		again, err := src.GetSecret(ctx, "kserve/ca")
		require.NoError(t, err)
		assert.Equal(t, []byte("PEM"), again["ca.crt"])
		assert.NotContains(t, again, "injected")

		again["ca.crt"][0] = 'X'
		fresh, err := src.GetSecret(ctx, "kserve/ca")
		require.NoError(t, err)
		assert.Equal(t, []byte("PEM"), fresh["ca.crt"])
	})
}
