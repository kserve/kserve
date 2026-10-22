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
	"errors"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"

	"github.com/kserve/kserve/pkg/kernelcache/types"
)

type keylessVerifier struct {
	checkOpts cosign.CheckOpts
}

var _ Verifier = (*keylessVerifier)(nil)

func newKeylessVerifier(cfg types.KeylessConfig) (*keylessVerifier, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	trustedMaterial, err := cosign.TrustedRoot()
	if err != nil {
		return nil, fmt.Errorf("load public Sigstore trusted root: %w", err)
	}

	return newKeylessVerifierWithCheckOpts(cfg, cosign.CheckOpts{TrustedMaterial: trustedMaterial})
}

func newKeylessVerifierWithCheckOpts(cfg types.KeylessConfig, opts cosign.CheckOpts) (*keylessVerifier, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if opts.TrustedMaterial == nil {
		return nil, errors.New("keyless verification requires Sigstore trusted material")
	}

	opts.Identities = []cosign.Identity{{Subject: cfg.Subject, Issuer: cfg.Issuer}}
	opts.ClaimVerifier = cosign.SimpleClaimVerifier
	opts.IgnoreTlog = false
	opts.IgnoreSCT = false
	opts.NewBundleFormat = false

	return &keylessVerifier{checkOpts: opts}, nil
}

func (v *keylessVerifier) Verify(ctx context.Context, req types.VerifyRequest) (types.VerifyResult, error) {
	res := types.VerifyResult{Mode: types.ModeKeyless}

	parseOptions := []name.Option{}
	if req.RegistryInsecure {
		parseOptions = append(parseOptions, name.Insecure)
	}
	ref, err := name.ParseReference(req.ImageRef, parseOptions...)
	if err != nil {
		return res, fmt.Errorf("parse image reference %q: %w", req.ImageRef, err)
	}

	remoteClientOpts := []remote.Option{remote.WithContext(ctx)}
	if req.RegistryAuthenticator != nil {
		remoteClientOpts = append(remoteClientOpts, remote.WithAuth(req.RegistryAuthenticator))
	} else {
		remoteClientOpts = append(remoteClientOpts, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}
	if req.RegistryTransport != nil {
		remoteClientOpts = append(remoteClientOpts, remote.WithTransport(req.RegistryTransport))
	}

	desc, err := remote.Head(ref, remoteClientOpts...)
	if err != nil {
		return res, fmt.Errorf("resolve image %q: %w", req.ImageRef, err)
	}
	res.Digest = desc.Digest.String()
	digestRef := ref.Context().Digest(res.Digest)
	remoteOpts := ociremote.WithRemoteOptions(remoteClientOpts...)

	// Fetch signatures separately so registry errors remain operational errors.
	signedEntity, err := ociremote.SignedEntity(digestRef, remoteOpts)
	if err != nil {
		return res, fmt.Errorf("load signatures for %q: %w", req.ImageRef, err)
	}
	signatures, err := signedEntity.Signatures()
	if err != nil {
		return res, fmt.Errorf("load signatures for %q: %w", req.ImageRef, err)
	}
	sigs, err := signatures.Get()
	if err != nil {
		return res, fmt.Errorf("load signatures for %q: %w", req.ImageRef, err)
	}
	if len(sigs) == 0 {
		res.Reason = "no signatures found"
		return res, nil
	}

	opts := v.checkOpts
	for _, sig := range sigs {
		bundleVerified, err := cosign.VerifyImageSignature(ctx, sig, desc.Digest, &opts)
		if err == nil && bundleVerified {
			res.Verified = true
			return res, nil
		}
	}

	res.Reason = "no valid keyless signature with verified transparency evidence"
	return res, nil
}
