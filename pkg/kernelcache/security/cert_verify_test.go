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

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/kernelcache/security"
	"github.com/kserve/kserve/pkg/kernelcache/security/fixture"
)

// verifierTrusting builds a cert-mode Verifier trusting caPEM (stored as a
// Secret under key "ca.crt"), matching signer SANs against subjectRE.
func verifierTrusting(t *testing.T, caPEM []byte, subjectRE string) security.Verifier {
	t.Helper()
	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/ca": {"ca.crt": caPEM},
	}}
	return verifierWith(t, src, "", subjectRE)
}

// verifierWith builds a cert-mode Verifier over an explicit source and trust-bundle key.
func verifierWith(t *testing.T, src security.SecretSource, key, subjectRE string) security.Verifier {
	t.Helper()
	v, err := security.NewVerifier(context.Background(), security.SecurityConfig{
		Mode:          security.ModeCert,
		FailurePolicy: security.FailurePolicyReject,
		Cert:          security.CertConfig{TrustBundle: "kserve/ca", TrustBundleKey: key, SubjectRegexp: subjectRE},
	}, src)
	require.NoError(t, err)
	return v
}

func untrustedCA(t *testing.T) *fixture.TestCA {
	return fixture.NewCA(t, fixture.WithCACommonName("kserve-kernelcache-untrusted-test-ca"))
}

// trusted CA + matching SAN -> verified, digest returned.
func TestCertVerify_TrustedMatchingSAN(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/ok")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	v := verifierTrusting(t, ca.CertPEM, "^spiffe://kserve-test/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, res.Verified)
	assert.Equal(t, security.ModeCert, res.Mode)
	assert.Equal(t, digest, res.Digest)
}

// signed by an untrusted CA -> not verified, no operational error.
func TestCertVerify_UntrustedCA(t *testing.T) {
	trusted := fixture.NewCA(t)
	untrusted := untrustedCA(t)
	leafKey, leafPEM := untrusted.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/untrusted")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, untrusted.CertPEM)

	v := verifierTrusting(t, trusted.CertPEM, "^spiffe://.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.False(t, res.Verified)
}

// chain ok but SAN does not match -> not verified, reason recorded.
func TestCertVerify_SANMismatch(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/sanmismatch")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	v := verifierTrusting(t, ca.CertPEM, "^spiffe://other/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.False(t, res.Verified)
	assert.NotEmpty(t, res.Reason, "a failed check should record a reason")
}

// no signature attached -> not verified.
func TestCertVerify_NoSignature(t *testing.T) {
	ca := fixture.NewCA(t)
	host := fixture.StartRegistry(t)
	ref, _ := fixture.PushImage(t, host, "kc/nosig")

	v := verifierTrusting(t, ca.CertPEM, "^spiffe://.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.False(t, res.Verified)
}

// trust bundle with two CAs (rotation), signed by one of them -> verified.
func TestCertVerify_MultiCARotation(t *testing.T) {
	caOld := fixture.NewCA(t)
	caNew := fixture.NewCA(t)
	leafKey, leafPEM := caOld.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/rotation")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, caOld.CertPEM)

	bundle := append(append([]byte{}, caOld.CertPEM...), caNew.CertPEM...)
	v := verifierTrusting(t, bundle, "^spiffe://kserve-test/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, res.Verified)
}

// a failed check still fills the digest (so a warn policy can pin it).
func TestCertVerify_DigestFilledOnFailure(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/digestfill")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	v := verifierTrusting(t, ca.CertPEM, "^spiffe://other/.*$") // mismatch
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.False(t, res.Verified)
	assert.Equal(t, digest, res.Digest)
}

// trust bundle uses a non-default key name.
func TestCertVerify_CustomBundleKey(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/customkey")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	src := &fixture.FakeSecretSource{Secrets: map[string]map[string][]byte{
		"kserve/ca": {"cabundle.pem": ca.CertPEM},
	}}
	v := verifierWith(t, src, "cabundle.pem", "^spiffe://kserve-test/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, res.Verified)
}

// trust bundle comes from a ConfigMap (no Secret).
func TestCertVerify_ConfigMapSource(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/cmsource")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	src := &fixture.FakeSecretSource{ConfigMaps: map[string]map[string]string{
		"kserve/ca": {"ca.crt": string(ca.CertPEM)},
	}}
	v := verifierWith(t, src, "", "^spiffe://kserve-test/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, res.Verified)
}

// when both a Secret and a ConfigMap exist, the Secret wins. The image is signed
// by the CA in the Secret; the ConfigMap holds a different (untrusted) CA, so a
// pass proves the Secret was used.
func TestCertVerify_SecretPrecedence(t *testing.T) {
	secretCA := fixture.NewCA(t)
	cmCA := untrustedCA(t)
	leafKey, leafPEM := secretCA.IssueLeaf(t)
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/precedence")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, secretCA.CertPEM)

	src := &fixture.FakeSecretSource{
		Secrets:    map[string]map[string][]byte{"kserve/ca": {"ca.crt": secretCA.CertPEM}},
		ConfigMaps: map[string]map[string]string{"kserve/ca": {"ca.crt": string(cmCA.CertPEM)}},
	}
	v := verifierWith(t, src, "", "^spiffe://kserve-test/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.True(t, res.Verified)
}

// a valid signature for one image transplanted onto another must not verify
// (the payload's digest no longer matches). Proves digest binding.
func TestCertVerify_SignatureTransplant(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t)
	host := fixture.StartRegistry(t)

	refA, digA := fixture.PushImage(t, host, "kc/transplant-a")
	fixture.Sign(t, refA, digA, leafKey, leafPEM, ca.CertPEM) // A is validly signed
	refB, digB := fixture.PushImage(t, host, "kc/transplant-b")
	fixture.Transplant(t, refA, digA, refB, digB) // move A's signature onto B

	v := verifierTrusting(t, ca.CertPEM, "^spiffe://kserve-test/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: refB.Name()})
	require.NoError(t, err)
	assert.False(t, res.Verified, "a signature for another image must not verify")
}

// an expired signing cert must not verify.
func TestCertVerify_ExpiredCert(t *testing.T) {
	ca := fixture.NewCA(t)
	leafKey, leafPEM := ca.IssueLeaf(t, fixture.Expired())
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/expired")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	v := verifierTrusting(t, ca.CertPEM, "^spiffe://kserve-test/.*$")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.False(t, res.Verified, "an expired signing cert must not verify")
}

// an unanchored subjectRegexp is anchored to a full match, so a lookalike SAN
// that merely contains the pattern is rejected.
func TestCertVerify_SubjectRegexpIsAnchored(t *testing.T) {
	ca := fixture.NewCA(t)
	// SAN embeds the operator's pattern as a substring but is not equal to it.
	leafKey, leafPEM := ca.IssueLeaf(t, fixture.WithSAN("spiffe://attacker/spiffe://kserve-test/kc-signer/x"))
	host := fixture.StartRegistry(t)
	ref, digest := fixture.PushImage(t, host, "kc/anchored")
	fixture.Sign(t, ref, digest, leafKey, leafPEM, ca.CertPEM)

	// unanchored pattern: a substring match would wrongly accept the lookalike.
	v := verifierTrusting(t, ca.CertPEM, "spiffe://kserve-test/kc-signer")
	res, err := v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	require.NoError(t, err)
	assert.False(t, res.Verified, "a lookalike SAN must not match an anchored pattern")
}

// image cannot be resolved -> operational error (retryable).
func TestCertVerify_ImageNotFound(t *testing.T) {
	ca := fixture.NewCA(t)
	host := fixture.StartRegistry(t)
	ref, err := name.ParseReference(host + "/kc/nope:latest")
	require.NoError(t, err)

	v := verifierTrusting(t, ca.CertPEM, "^spiffe://.*$")
	_, err = v.Verify(context.Background(), security.VerifyRequest{ImageRef: ref.Name()})
	assert.Error(t, err)
}
