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

	"github.com/kserve/kserve/pkg/kernelcache/types"
)

// Required fields are enforced per role at construction, not by Validate, so a
// verify-only or sign-only config both pass Validate but the wrong constructor
// still rejects a config that lacks its material.
func TestCertConfig_ConstructorRequirements(t *testing.T) {
	ctx := context.Background()

	t.Run("verifier needs trust bundle", func(t *testing.T) {
		_, err := newCertVerifier(ctx, types.CertConfig{SubjectRegexp: ".*"}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trustBundle")
	})
	t.Run("verifier needs subject regexp", func(t *testing.T) {
		_, err := newCertVerifier(ctx, types.CertConfig{TrustBundle: "kserve/ca"}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subjectRegexp")
	})
	t.Run("signer needs signing secret", func(t *testing.T) {
		_, err := newCertSigner(ctx, types.CertConfig{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signingSecret")
	})
}
