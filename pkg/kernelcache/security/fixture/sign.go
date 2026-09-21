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
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"encoding/base64"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/cosign/v3/pkg/oci/mutate"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"
	"github.com/sigstore/cosign/v3/pkg/oci/static"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/sigstore/sigstore/pkg/signature/payload"
	"github.com/stretchr/testify/require"
)

// Sign attaches a cosign signature over the image's digest, signed by leafKey
// and carrying the leaf cert + CA chain. This is the standard cosign signature
// format, so certVerifier can verify it.
//
// Mirrors production certSigner.Sign but takes key material directly, so
// verify-side tests need not depend on the signer. Keep the two in sync.
func Sign(tb testing.TB, ref name.Reference, digest string, leafKey *ecdsa.PrivateKey, leafCertPEM, caChainPEM []byte) {
	tb.Helper()
	digestRef := ref.Context().Digest(digest)

	pl, err := (&payload.Cosign{Image: digestRef}).MarshalJSON()
	require.NoError(tb, err)

	sv, err := signature.LoadSignerVerifier(leafKey, crypto.SHA256)
	require.NoError(tb, err)
	sig, err := sv.SignMessage(bytes.NewReader(pl))
	require.NoError(tb, err)
	b64 := base64.StdEncoding.EncodeToString(sig)

	ociSig, err := static.NewSignature(pl, b64, static.WithCertChain(leafCertPEM, caChainPEM))
	require.NoError(tb, err)

	se, err := ociremote.SignedEntity(digestRef)
	require.NoError(tb, err)
	newSE, err := mutate.AttachSignatureToEntity(se, ociSig)
	require.NoError(tb, err)
	require.NoError(tb, ociremote.WriteSignatures(digestRef.Repository, newSE))
}

// ReplaySignaturesOntoImage copies the signatures attached to (fromRef@fromDigest)
// onto (toRef@toDigest). Used to prove a signature for one image cannot be
// replayed onto another.
func ReplaySignaturesOntoImage(tb testing.TB, fromRef name.Reference, fromDigest string, toRef name.Reference, toDigest string) {
	tb.Helper()
	seFrom, err := ociremote.SignedEntity(fromRef.Context().Digest(fromDigest))
	require.NoError(tb, err)
	sigsFrom, err := seFrom.Signatures()
	require.NoError(tb, err)
	got, err := sigsFrom.Get()
	require.NoError(tb, err)
	require.NotEmpty(tb, got)

	toDR := toRef.Context().Digest(toDigest)
	seTo, err := ociremote.SignedEntity(toDR)
	require.NoError(tb, err)
	for _, s := range got {
		seTo, err = mutate.AttachSignatureToEntity(seTo, s)
		require.NoError(tb, err)
	}
	require.NoError(tb, ociremote.WriteSignatures(toDR.Repository, seTo))
}
