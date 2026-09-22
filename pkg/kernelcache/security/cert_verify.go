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
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"

	"github.com/kserve/kserve/pkg/kernelcache/types"
)

// secretLoadTimeout bounds the one-time load of key/trust material (CA bundle or
// signing secret) from the SecretSource at construction.
const secretLoadTimeout = 5 * time.Second

// certVerifier verifies a signature made with a cert-manager issued key by
// checking the X.509 chain against a CA bundle plus a SAN identity. It does not
// use a transparency log or SCT.
type certVerifier struct {
	caPool *x509.CertPool
	// subjectPattern is the identity regexp, anchored to a full match, passed to
	// cosign. Anchoring closes cosign's default unanchored (substring) matching.
	subjectPattern string
}

// newCertVerifier loads the CA bundle from src and prepares the identity match.
// It fails fast on a missing or invalid trust bundle. The load is bounded by a
// timeout derived from the caller's context.
func newCertVerifier(ctx context.Context, cfg types.CertConfig, src SecretSource) (*certVerifier, error) {
	// Verification needs both the trust anchor and the identity to match against;
	// presence is enforced here (SecurityConfig.Validate only checks syntax).
	if cfg.TrustBundle == "" {
		return nil, errors.New("cert.trustBundle must be set")
	}
	if cfg.SubjectRegexp == "" {
		return nil, errors.New("cert.subjectRegexp must be set")
	}

	// Anchor the operator's regexp so it must match the whole SAN, not just a
	// substring (cosign matches identity regexps unanchored). The pattern's
	// syntax was already validated by SecurityConfig.Validate.
	subjectPattern := "^(?:" + cfg.SubjectRegexp + ")$"

	loadCtx, cancel := context.WithTimeout(ctx, secretLoadTimeout)
	defer cancel()
	caPEM, err := loadTrustBundle(loadCtx, src, cfg.TrustBundle, cfg.TrustBundleKey)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("trust bundle %q: no valid CA certificate found", cfg.TrustBundle)
	}
	return &certVerifier{caPool: pool, subjectPattern: subjectPattern}, nil
}

// requireDataKey returns a non-empty value for key in secret/configmap data,
// or a clear error. An empty value is treated the same as a missing one so a
// blank field does not slip through to a later, murkier parse failure.
// Used by both signing and verification material loading.
func requireDataKey(data map[string][]byte, key, ref string) ([]byte, error) {
	v, ok := data[key]
	if !ok || len(v) == 0 {
		return nil, fmt.Errorf("key %q not present in %q", key, ref)
	}
	return v, nil
}

// loadTrustBundle reads the CA PEM under key from the referenced Secret or
// ConfigMap. A Secret and a ConfigMap of the same name may both exist; the
// Secret takes precedence. When neither yields the key, the underlying lookup
// errors (or "key not present") are surfaced so a real permission/network
// failure is not masked as a plain "not found".
func loadTrustBundle(ctx context.Context, src SecretSource, ref, key string) ([]byte, error) {
	if src == nil {
		return nil, fmt.Errorf("trust bundle %q: no secret source configured", ref)
	}
	if key == "" {
		key = types.DefaultTrustBundleKey
	}

	var problems []string

	// Secret takes precedence.
	if data, err := src.GetSecret(ctx, ref); err != nil {
		problems = append(problems, fmt.Sprintf("secret: %v", err))
	} else if ca, ok := data[key]; ok {
		return ca, nil
	} else {
		problems = append(problems, fmt.Sprintf("secret: key %q not present", key))
	}

	// ConfigMap fallback.
	if data, err := src.GetConfigMap(ctx, ref); err != nil {
		problems = append(problems, fmt.Sprintf("configmap: %v", err))
	} else if ca, ok := data[key]; ok {
		return []byte(ca), nil
	} else {
		problems = append(problems, fmt.Sprintf("configmap: key %q not present", key))
	}

	return nil, fmt.Errorf("trust bundle %q: key %q not found (%s)", ref, key, strings.Join(problems, "; "))
}

// Verify resolves the image digest, then checks its signature against the CA
// bundle and the signer identity. The digest is resolved first: a failure there
// is operational (retryable) and returns an error, whereas a completed check
// that fails returns VerifyResult{Verified: false} with the digest still set so
// a warn policy can pin it.
func (v *certVerifier) Verify(ctx context.Context, req types.VerifyRequest) (types.VerifyResult, error) {
	res := types.VerifyResult{Mode: types.ModeCert}

	ref, err := name.ParseReference(req.ImageRef)
	if err != nil {
		return res, fmt.Errorf("parse image reference %q: %w", req.ImageRef, err)
	}

	// Registry access uses the ambient keychain (the same options back both the
	// digest resolve and the signature fetch). Per-InferenceService
	// imagePullSecrets are not consulted here; the wiring will inject a keychain
	// built from them.
	keychain := authn.DefaultKeychain

	// Resolve the digest first. Failure here means the image is unreachable
	// (operational, retryable), not a verification failure.
	desc, err := remote.Head(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(keychain))
	if err != nil {
		return res, fmt.Errorf("resolve image %q: %w", req.ImageRef, err)
	}
	res.Digest = desc.Digest.String()
	digestRef := ref.Context().Digest(res.Digest)

	co := &cosign.CheckOpts{
		RootCerts:     v.caPool,
		IgnoreTlog:    true,
		IgnoreSCT:     true,
		Identities:    []cosign.Identity{{SubjectRegExp: v.subjectPattern}},
		ClaimVerifier: cosign.SimpleClaimVerifier,
		RegistryClientOpts: []ociremote.Option{
			ociremote.WithRemoteOptions(remote.WithContext(ctx), remote.WithAuthFromKeychain(keychain)),
		},
	}

	// The image exists (digest resolved), so a cosign error here means the
	// signature is missing or does not verify: a completed, failed check.
	sigs, _, err := cosign.VerifyImageSignatures(ctx, digestRef, co)
	if err != nil {
		// A cosign error here means the signature check itself failed (missing or
		// invalid signature), not an operational failure - so it is reported as
		// Verified=false with a nil error, per the Verifier contract.
		res.Verified = false
		res.Reason = err.Error()
		return res, nil //nolint:nilerr // failed check, not an operational error
	}
	if len(sigs) == 0 {
		res.Verified = false
		res.Reason = "no valid signatures"
		return res, nil
	}

	res.Verified = true
	return res, nil
}
