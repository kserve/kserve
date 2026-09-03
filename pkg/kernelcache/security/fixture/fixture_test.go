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
	"testing"

	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"
	"github.com/stretchr/testify/require"
)

// TestFixtures smoke-tests the builders end to end: mint CA, issue leaf, push
// an image, sign it, and confirm a signature is attached.
func TestFixtures(t *testing.T) {
	ca := NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t)

	host := StartRegistry(t)
	ref, digest := PushImage(t, host, "kc/foo")
	Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	se, err := ociremote.SignedEntity(ref.Context().Digest(digest))
	require.NoError(t, err)
	sigs, err := se.Signatures()
	require.NoError(t, err)
	got, err := sigs.Get()
	require.NoError(t, err)
	require.NotEmpty(t, got, "expected at least one attached signature")
}
