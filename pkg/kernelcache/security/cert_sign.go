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
	"bytes"
	"cmp"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/oci/mutate"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"
	"github.com/sigstore/cosign/v3/pkg/oci/static"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/sigstore/sigstore/pkg/signature/payload"

	"github.com/kserve/kserve/pkg/kernelcache/types"
)

// Default Secret data keys for the signing material: the kubernetes.io/tls layout
// (tls.key/tls.crt) plus cert-manager's ca.crt chain. Overridable per CertConfig.
const (
	defaultSigningKeyKey   = "tls.key"
	defaultSigningCertKey  = "tls.crt"
	defaultSigningChainKey = "ca.crt"
)

// certSigner signs an image's digest with a cert-manager issued key and attaches
// the leaf certificate plus CA chain, producing the standard cosign signature
// format that certVerifier accepts. It uses no transparency log or SCT.
type certSigner struct {
	sv          signature.SignerVerifier
	leafCert    *x509.Certificate
	certChain   []*x509.Certificate
	leafCertPEM []byte
	caChainPEM  []byte
	now         func() time.Time
}

// newCertSigner loads the signing key, leaf certificate, and CA chain from the
// signing Secret referenced by cfg. It fails fast on missing or malformed
// material. The load is bounded by a timeout derived from the caller's context.
func newCertSigner(ctx context.Context, cfg types.CertConfig, src SecretSource) (*certSigner, error) {
	// Signing needs its secret; presence is enforced here (SecurityConfig.Validate
	// only checks syntax).
	if cfg.SigningSecret == "" {
		return nil, errors.New("cert.signingSecret must be set")
	}
	if src == nil {
		return nil, fmt.Errorf("signing secret %q: no secret source configured", cfg.SigningSecret)
	}

	loadCtx, cancel := context.WithTimeout(ctx, secretLoadTimeout)
	defer cancel()
	data, err := src.GetSecret(loadCtx, cfg.SigningSecret)
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: %w", cfg.SigningSecret, err)
	}

	keyName := cmp.Or(cfg.SigningKeyKey, defaultSigningKeyKey)
	certName := cmp.Or(cfg.SigningCertKey, defaultSigningCertKey)
	chainName := cmp.Or(cfg.SigningChainKey, defaultSigningChainKey)

	keyPEM, err := requireDataKey(data, keyName, cfg.SigningSecret)
	if err != nil {
		return nil, err
	}
	leafPEM, err := requireDataKey(data, certName, cfg.SigningSecret)
	if err != nil {
		return nil, err
	}
	caPEM, err := requireDataKey(data, chainName, cfg.SigningSecret)
	if err != nil {
		return nil, err
	}

	priv, err := cryptoutils.UnmarshalPEMToPrivateKey(keyPEM, cryptoutils.SkipPassword)
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: parse signing key: %w", cfg.SigningSecret, err)
	}
	key, ok := priv.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("signing secret %q: signing key does not support signing", cfg.SigningSecret)
	}
	leafCerts, err := cryptoutils.UnmarshalCertificatesFromPEM(leafPEM)
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: parse leaf certificate (%s): %w", cfg.SigningSecret, certName, err)
	}
	if len(leafCerts) == 0 {
		return nil, fmt.Errorf("signing secret %q: no certificate found in %s", cfg.SigningSecret, certName)
	}
	// Reject a mismatched keypair here rather than emitting a signature that only
	// fails at verify time with a confusing "does not verify" reason.
	if err := cryptoutils.EqualKeys(key.Public(), leafCerts[0].PublicKey); err != nil {
		return nil, fmt.Errorf("signing secret %q: private key (%s) does not match leaf certificate (%s): %w",
			cfg.SigningSecret, keyName, certName, err)
	}
	// Same reason: an expired/not-yet-valid leaf would sign but never verify.
	if err := cryptoutils.CheckExpiration(leafCerts[0], time.Now()); err != nil {
		return nil, fmt.Errorf("signing secret %q: leaf certificate (%s): %w", cfg.SigningSecret, certName, err)
	}
	caCerts, err := cryptoutils.UnmarshalCertificatesFromPEM(caPEM)
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: parse CA chain (%s): %w", cfg.SigningSecret, chainName, err)
	}
	if len(caCerts) == 0 {
		return nil, fmt.Errorf("signing secret %q: no CA certificate found in %s", cfg.SigningSecret, chainName)
	}

	// The leaf must actually chain to the supplied CA: intermediates in tls.crt
	// (leafCerts[1:]) bridge to the roots in ca.crt. Signing a leaf anchored to a
	// different bundle would emit a signature the verifier can never validate.
	// KeyUsages must be CodeSigning to match the leaf; the default ServerAuth
	// would reject it.
	roots := x509.NewCertPool()
	for i, c := range caCerts {
		if !c.IsCA {
			return nil, fmt.Errorf("signing secret %q: certificate %d in CA chain (%s) is not a CA certificate", cfg.SigningSecret, i, chainName)
		}
		roots.AddCert(c)
	}
	intermediates := x509.NewCertPool()
	for _, c := range leafCerts[1:] {
		intermediates.AddCert(c)
	}
	verifiedChains, err := leafCerts[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		CurrentTime:   time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: leaf certificate does not chain to the supplied CA (%s): %w", cfg.SigningSecret, chainName, err)
	}
	if len(verifiedChains) == 0 {
		return nil, fmt.Errorf("signing secret %q: no verified certificate chain found", cfg.SigningSecret)
	}

	// Cosign stores the leaf separately from its chain. cert-manager may include
	// intermediates after the leaf in tls.crt, so normalize them into the chain
	// annotation ahead of the CA bundle certificates.
	leafCertPEM, err := cryptoutils.MarshalCertificateToPEM(leafCerts[0])
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: marshal leaf certificate (%s): %w", cfg.SigningSecret, certName, err)
	}
	chainCerts := append([]*x509.Certificate{}, leafCerts[1:]...)
	chainCerts = append(chainCerts, caCerts...)
	caChainPEM, err := cryptoutils.MarshalCertificatesToPEM(chainCerts)
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: marshal CA chain (%s): %w", cfg.SigningSecret, chainName, err)
	}

	sv, err := signature.LoadSignerVerifier(key, crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("signing secret %q: load signer: %w", cfg.SigningSecret, err)
	}

	return &certSigner{
		sv:          sv,
		leafCert:    leafCerts[0],
		certChain:   verifiedChains[0],
		leafCertPEM: leafCertPEM,
		caChainPEM:  caChainPEM,
		now:         time.Now,
	}, nil
}

// Sign resolves the image digest and attaches a signature over that digest
// (never the mutable tag), so the signature is bound to the exact content. A
// failure to resolve or write is operational and returned as an error.
func (s *certSigner) Sign(ctx context.Context, req types.SignRequest) (types.SignResult, error) {
	res := types.SignResult{Mode: types.ModeCert}

	now := s.now()
	for i, cert := range s.certChain {
		if err := cryptoutils.CheckExpiration(cert, now); err != nil {
			return res, fmt.Errorf("certificate chain element %d is no longer valid: %w", i, err)
		}
	}

	ref, err := name.ParseReference(req.ImageRef)
	if err != nil {
		return res, fmt.Errorf("parse image reference %q: %w", req.ImageRef, err)
	}

	// Registry access uses the ambient keychain, matching the verifier; the
	// wiring will inject a keychain built from imagePullSecrets.
	keychain := authn.DefaultKeychain
	remoteClientOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(keychain),
	}
	remoteOpts := ociremote.WithRemoteOptions(remoteClientOpts...)

	desc, err := remote.Head(ref, remoteClientOpts...)
	if err != nil {
		return res, fmt.Errorf("resolve image %q: %w", req.ImageRef, err)
	}
	res.Digest = desc.Digest.String()
	digestRef := ref.Context().Digest(res.Digest)

	pl, err := (&payload.Cosign{Image: digestRef}).MarshalJSON()
	if err != nil {
		return res, fmt.Errorf("build signing payload: %w", err)
	}
	sig, err := s.sv.SignMessage(bytes.NewReader(pl))
	if err != nil {
		return res, fmt.Errorf("sign payload: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(sig)

	ociSig, err := static.NewSignature(pl, b64, static.WithCertChain(s.leafCertPEM, s.caChainPEM))
	if err != nil {
		return res, fmt.Errorf("assemble signature: %w", err)
	}

	se, err := ociremote.SignedEntity(digestRef, remoteOpts)
	if err != nil {
		return res, fmt.Errorf("read signed entity %q: %w", res.Digest, err)
	}
	newSE, err := mutate.AttachSignatureToEntity(se, ociSig)
	if err != nil {
		return res, fmt.Errorf("attach signature: %w", err)
	}
	if err := ociremote.WriteSignatures(digestRef.Repository, newSE, remoteOpts); err != nil {
		return res, fmt.Errorf("write signature for %q: %w", res.Digest, err)
	}

	return res, nil
}
