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

package security_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/kernelcache/security"
	"github.com/kserve/kserve/pkg/kernelcache/security/fixture"
	"github.com/kserve/kserve/pkg/kernelcache/types"
)

const (
	signerSAN     = "spiffe://kserve-test/kc-signer"
	signerSubject = `^spiffe://kserve-test/.*$`
)

// signerOver builds a cert-mode Signer that loads its material from src under the
// given signing-secret ref.
func signerOver(t *testing.T, src security.SecretSource, secretRef string) security.Signer {
	t.Helper()
	s, err := security.NewSigner(context.Background(), types.SecurityConfig{
		Mode: types.ModeCert,
		Cert: types.CertConfig{SigningSecret: secretRef},
	}, src)
	require.NoError(t, err)
	return s
}

// TestCertSign_RoundTrip is the core contract: what the signer produces, the
// verifier accepts. Run for both key encodings cert-manager can emit.
func TestCertSign_RoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pkcs8 bool
	}{
		{"pkcs8 key", true},
		{"sec1 key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ca := fixture.NewCA(t)
			leafKey, leafPEM := ca.IssueLeaf(t, fixture.WithSAN(signerSAN))
			host := fixture.StartRegistry(t)
			ref, wantDigest := fixture.PushImage(t, host, "kc/sign")

			src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, leafKey, tc.pkcs8), leafPEM, ca.CertPEM),
				"kserve/ca":     {"ca.crt": ca.CertPEM},
			}}

			sres, err := signerOver(t, src, "kserve/signer").Sign(ctx, types.SignRequest{ImageRef: ref.Name()})
			require.NoError(t, err)
			assert.Equal(t, types.ModeCert, sres.Mode)
			assert.Equal(t, wantDigest, sres.Digest)

			v, err := security.NewVerifier(ctx, types.SecurityConfig{
				Mode: types.ModeCert,
				Cert: types.CertConfig{TrustBundle: "kserve/ca", SubjectRegexp: signerSubject},
			}, src)
			require.NoError(t, err)
			vres, err := v.Verify(ctx, types.VerifyRequest{ImageRef: ref.Name()})
			require.NoError(t, err)
			assert.True(t, vres.Verified, "reason: %s", vres.Reason)
			assert.Equal(t, sres.Digest, vres.Digest)
		})
	}
}

func TestCertSign_IntermediateRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := fixture.NewCA(t)
	intermediate := root.IssueIntermediate(t)
	leafKey, leafPEM := intermediate.IssueLeaf(t, fixture.WithSAN(signerSAN))
	tlsCertPEM := append(append([]byte{}, leafPEM...), intermediate.CertPEM...)
	host := fixture.StartRegistry(t)
	ref, wantDigest := fixture.PushImage(t, host, "kc/intermediate")

	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, leafKey, true), tlsCertPEM, root.CertPEM),
		"kserve/ca":     {"ca.crt": root.CertPEM},
	}}

	sres, err := signerOver(t, src, "kserve/signer").Sign(ctx, types.SignRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.Equal(t, wantDigest, sres.Digest)

	v, err := security.NewVerifier(ctx, types.SecurityConfig{
		Mode: types.ModeCert,
		Cert: types.CertConfig{TrustBundle: "kserve/ca", SubjectRegexp: signerSubject},
	}, src)
	require.NoError(t, err)
	vres, err := v.Verify(ctx, types.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, vres.Verified, "reason: %s", vres.Reason)
	assert.Equal(t, sres.Digest, vres.Digest)
}

// A user-provided Secret with non-cert-manager key names works when the config
// overrides them (mirrors verify's TrustBundleKey).
func TestCertSign_CustomSecretKeys(t *testing.T) {
	ctx := context.Background()
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t, fixture.WithSAN(signerSAN))
	host := fixture.StartRegistry(t)
	ref, _ := fixture.PushImage(t, host, "kc/customkeys")

	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/signer": {
			"signer.key": fixture.LeafKeyPEM(t, leafKey, true),
			"signer.crt": leafPEM,
			"chain.pem":  ca.CertPEM,
		},
		"kserve/ca": {"ca.crt": ca.CertPEM},
	}}
	cfg := types.SecurityConfig{Mode: types.ModeCert, Cert: types.CertConfig{
		TrustBundle: "kserve/ca", SubjectRegexp: signerSubject, SigningSecret: "kserve/signer",
		SigningKeyKey: "signer.key", SigningCertKey: "signer.crt", SigningChainKey: "chain.pem",
	}}

	s, err := security.NewSigner(ctx, cfg, src)
	require.NoError(t, err)
	_, err = s.Sign(ctx, types.SignRequest{ImageRef: ref.Name()})
	require.NoError(t, err)

	v, err := security.NewVerifier(ctx, cfg, src)
	require.NoError(t, err)
	vres, err := v.Verify(ctx, types.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, vres.Verified, "reason: %s", vres.Reason)
}

// A signer needs its signing secret; a cert config without one is a construction
// error, not a silent no-op.
func TestNewSigner_RequiresSigningSecret(t *testing.T) {
	_, err := security.NewSigner(context.Background(), types.SecurityConfig{Mode: types.ModeCert}, &fixture.FakeSecretSource{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signingSecret")
}

// A mismatched key/cert pair must fail at construction, not at verify time.
func TestCertSign_KeyCertMismatch(t *testing.T) {
	ca := fixture.NewCA(t)
	keyA, _ := ca.IssueLeaf(t, fixture.WithSAN(signerSAN))
	_, certB := ca.IssueLeaf(t, fixture.WithSAN(signerSAN))

	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, keyA, true), certB, ca.CertPEM),
	}}

	s, err := security.NewSigner(context.Background(), types.SecurityConfig{
		Mode: types.ModeCert,
		Cert: types.CertConfig{SigningSecret: "kserve/signer"},
	}, src)
	assert.Nil(t, s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestCertSign_RejectsNonCAInCAChain(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t, fixture.WithSAN(signerSAN))
	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, leafKey, true), leafPEM, leafPEM),
	}}

	s, err := security.NewSigner(context.Background(), types.SecurityConfig{
		Mode: types.ModeCert,
		Cert: types.CertConfig{SigningSecret: "kserve/signer"},
	}, src)
	assert.Nil(t, s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a CA certificate")
}

// A leaf whose key/cert come from one CA but whose ca.crt chain is an unrelated
// CA must fail construction: signing it would emit a signature the verifier
// (anchored to a different trust bundle) can never validate.
func TestCertSign_UnrelatedCAChain(t *testing.T) {
	caA := fixture.NewCA(t)
	caB := fixture.NewCA(t, fixture.WithCACommonName("kserve-kernelcache-unrelated-test-ca"))
	leafKey, leafPEM := caA.IssueLeaf(t, fixture.WithSAN(signerSAN))
	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, leafKey, true), leafPEM, caB.CertPEM),
	}}
	s, err := security.NewSigner(context.Background(), types.SecurityConfig{
		Mode: types.ModeCert,
		Cert: types.CertConfig{SigningSecret: "kserve/signer"},
	}, src)
	assert.Nil(t, s)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not chain")
}

// The signer must accept an RSA leaf key encoded as PKCS#1, since cert-manager
// can emit that layout. Full sign -> verify round trip.
func TestCertSign_RoundTripRSA_PKCS1(t *testing.T) {
	ctx := context.Background()
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeafRSA(t, fixture.WithSAN(signerSAN))
	host := fixture.StartRegistry(t)
	ref, wantDigest := fixture.PushImage(t, host, "kc/sign-rsa")

	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/signer": fixture.SigningSecret(fixture.RSAKeyPEMPKCS1(t, leafKey), leafPEM, ca.CertPEM),
		"kserve/ca":     {"ca.crt": ca.CertPEM},
	}}

	sres, err := signerOver(t, src, "kserve/signer").Sign(ctx, types.SignRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.Equal(t, wantDigest, sres.Digest)

	v, err := security.NewVerifier(ctx, types.SecurityConfig{
		Mode: types.ModeCert,
		Cert: types.CertConfig{TrustBundle: "kserve/ca", SubjectRegexp: signerSubject},
	}, src)
	require.NoError(t, err)
	vres, err := v.Verify(ctx, types.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, vres.Verified, "reason: %s", vres.Reason)
}

func TestCertSign_ConstructionErrors(t *testing.T) {
	ctx := context.Background()
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t, fixture.WithSAN(signerSAN))
	expiredKey, expiredPEM := ca.IssueLeaf(t, fixture.WithSAN(signerSAN), fixture.Expired())
	notYetKey, notYetPEM := ca.IssueLeaf(t, fixture.WithSAN(signerSAN), fixture.NotYetValid())

	tests := []struct {
		name string
		src  *fixture.FakeSecretSource
	}{
		{
			name: "secret ref absent",
			src:  &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{}},
		},
		{
			name: "tls.key missing",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": {"tls.crt": leafPEM, "ca.crt": ca.CertPEM},
			}},
		},
		{
			name: "malformed key",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": fixture.SigningSecret([]byte("not a pem key"), leafPEM, ca.CertPEM),
			}},
		},
		{
			name: "malformed leaf cert",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, leafKey, true), []byte("not a certificate"), ca.CertPEM),
			}},
		},
		{
			name: "malformed ca chain",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, leafKey, true), leafPEM, []byte("not a certificate")),
			}},
		},
		{
			name: "leaf cert blank (no PEM block)",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, leafKey, true), []byte("   \n  "), ca.CertPEM),
			}},
		},
		{
			name: "ca chain missing",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": {"tls.key": fixture.LeafKeyPEM(t, leafKey, true), "tls.crt": leafPEM},
			}},
		},
		{
			name: "expired leaf cert",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, expiredKey, true), expiredPEM, ca.CertPEM),
			}},
		},
		{
			name: "not-yet-valid leaf cert",
			src: &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
				"kserve/signer": fixture.SigningSecret(fixture.LeafKeyPEM(t, notYetKey, true), notYetPEM, ca.CertPEM),
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := security.NewSigner(ctx, types.SecurityConfig{
				Mode: types.ModeCert,
				Cert: types.CertConfig{SigningSecret: "kserve/signer"},
			}, tt.src)
			assert.Nil(t, s)
			assert.Error(t, err)
		})
	}
}
