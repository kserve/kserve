/*
Copyright 2025 The KServe Authors.

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

// Cosign verification wrapper
package cosign

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	gcrname "github.com/google/go-containerregistry/pkg/name"
	gcrremote "github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"
	rekorclient "github.com/sigstore/rekor/pkg/generated/client"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// DefaultVerificationTimeout is the default timeout for image verification operations.
	// Set to 30 seconds to accommodate v3 bundle verification which can take 15-20 seconds.
	DefaultVerificationTimeout = 30 * time.Second
)

var log logr.Logger

func init() {
	log = logf.Log.WithName("cosign")
}

// VerifyImageSignature verifies an image signature using Cosign v3.
// It tries multiple verification methods in order:
// 1. New bundle format (cosign v3 with --new-bundle-format)
// 2. Legacy .sig tag format (cosign v2)
// Returns the verified digest and nil error on success.
func VerifyImageSignature(ctx context.Context, imageRef string) (string, error) {
	ref, err := gcrname.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("parse image reference: %w", err)
	}

	regOpts := []ociremote.Option{
		ociremote.WithRemoteOptions(
			gcrremote.WithAuthFromKeychain(authn.DefaultKeychain),
		),
	}

	rc := rekorclient.NewHTTPClientWithConfig(nil,
		rekorclient.DefaultTransportConfig().
			WithHost("rekor.sigstore.dev").
			WithBasePath("/").
			WithSchemes([]string{"https"}),
	)

	trusted, err := cosign.TrustedRoot()
	if err != nil {
		return "", fmt.Errorf("load Sigstore trust roots: %w", err)
	}

	log.V(1).Info("Attempting new bundle format verification", "image", imageRef)
	digest, err := verifyNewBundleFormat(ctx, ref, regOpts, rc, trusted)
	if err == nil {
		log.Info("Successfully verified using new bundle format", "image", ref.Name(), "digest", digest)
		return digest, nil
	}
	log.Info("New bundle format verification failed, trying legacy format", "error", err)

	log.V(1).Info("Attempting legacy signature verification", "image", imageRef)
	digest, err = verifyLegacySignature(ctx, ref, regOpts, rc, trusted)
	if err == nil {
		log.Info("Successfully verified using legacy format", "image", ref.Name(), "digest", digest)
		return digest, nil
	}

	return "", fmt.Errorf("signature verification failed for all formats: %w", err)
}

// verifyNewBundleFormat verifies images signed with --new-bundle-format
func verifyNewBundleFormat(ctx context.Context, ref gcrname.Reference, regOpts []ociremote.Option, rc *rekorclient.Rekor, trusted root.TrustedMaterial) (string, error) {
	bundles, hash, err := cosign.GetBundles(ctx, ref, regOpts)
	if err != nil {
		return "", fmt.Errorf("failed to get bundles: %w", err)
	}

	if len(bundles) == 0 {
		return "", errors.New("no bundles found")
	}

	// Create artifact policy for the image digest
	digestBytes, err := hex.DecodeString(hash.Hex)
	if err != nil {
		return "", fmt.Errorf("failed to decode digest hex: %w", err)
	}
	artifactDigestPolicyOption := verify.WithArtifactDigest("sha256", digestBytes)

	verifier, err := verify.NewVerifier(trusted,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithIntegratedTimestamps(1),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create verifier: %w", err)
	}

	// Verify each bundle until we find a valid one
	var lastErr error
	for i, bundle := range bundles {
		log.V(1).Info("Verifying bundle", "index", i, "totalBundles", len(bundles))

		policy := verify.NewPolicy(artifactDigestPolicyOption, verify.WithoutIdentitiesUnsafe())
		_, err := verifier.Verify(bundle, policy)
		if err != nil {
			log.Info("Bundle verification failed", "index", i, "error", err)
			lastErr = err
			continue
		}

		log.Info("Successfully verified bundle", "index", i, "digest", hash.String())
		return hash.String(), nil
	}

	return "", fmt.Errorf("all %d bundles failed verification, last error: %w", len(bundles), lastErr)
}

// verifyLegacySignature verifies images with legacy .sig tags (cosign v2)
func verifyLegacySignature(ctx context.Context, ref gcrname.Reference, regOpts []ociremote.Option, rc *rekorclient.Rekor, trusted root.TrustedMaterial) (string, error) {
	co := &cosign.CheckOpts{
		RegistryClientOpts: regOpts,
		RekorClient:        rc,
		TrustedMaterial:    trusted,
		ClaimVerifier:      cosign.SimpleClaimVerifier,
	}

	checkedSignatures, _, err := cosign.VerifyImageSignatures(ctx, ref, co)
	if err != nil {
		return "", fmt.Errorf("legacy signature verification failed: %w", err)
	}

	if len(checkedSignatures) == 0 {
		return "", errors.New("no valid legacy signatures found")
	}

	se, err := ociremote.SignedEntity(ref, regOpts...)
	if err != nil {
		return "", fmt.Errorf("fetch signed entity for digest: %w", err)
	}
	h, err := se.Digest()
	if err != nil {
		return "", fmt.Errorf("compute digest: %w", err)
	}

	return h.String(), nil
}
