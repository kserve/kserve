//go:build integration

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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/kernelcache/security/fixture"
	"github.com/kserve/kserve/pkg/kernelcache/types"
)

const offlineKeylessFixtureDir = "testdata/keyless/"

type offlineKeylessFixtureMetadata struct {
	ImageDigest string `json:"imageDigest"`
	Subject     string `json:"subject"`
	Issuer      string `json:"issuer"`
}

// TestKeylessVerifyOfflineFixture exercises the real Cosign verification path
// with a captured public-Sigstore signature. The test uses an in-memory OCI
// registry and a checked-in Sigstore trusted-root snapshot.
func TestKeylessVerifyOfflineFixture(t *testing.T) {
	fixtureDir := offlineKeylessFixtureDir
	metadata := readOfflineKeylessFixtureMetadata(t, fixtureDir)

	image := readOCIImageIndexFixture(t, filepath.Join(fixtureDir, "image"))

	imageDigest, err := image.Digest()
	require.NoError(t, err)
	require.Equal(t, metadata.ImageDigest, imageDigest.String(), "fixture image digest must match the captured signature")

	tests := []struct {
		name            string
		subject         string
		issuer          string
		attachSignature bool
		tamperImage     bool
		wantVerified    bool
		wantReason      string
	}{
		{
			name:            "valid Fulcio certificate SCT and Rekor bundle",
			subject:         metadata.Subject,
			issuer:          metadata.Issuer,
			attachSignature: true,
			wantVerified:    true,
		},
		{
			name:            "subject mismatch",
			subject:         "attacker@example.invalid",
			issuer:          metadata.Issuer,
			attachSignature: true,
			wantVerified:    false,
			wantReason:      "no valid keyless signature with verified transparency evidence",
		},
		{
			name:            "issuer mismatch",
			subject:         metadata.Subject,
			issuer:          "https://issuer.invalid",
			attachSignature: true,
			wantVerified:    false,
			wantReason:      "no valid keyless signature with verified transparency evidence",
		},
		{
			name:            "signature absent",
			subject:         metadata.Subject,
			issuer:          metadata.Issuer,
			attachSignature: false,
			wantVerified:    false,
			wantReason:      "no signatures found",
		},
		{
			name:            "signature transplanted to a different image digest",
			subject:         metadata.Subject,
			issuer:          metadata.Issuer,
			attachSignature: true,
			tamperImage:     true,
			wantVerified:    false,
			wantReason:      "no valid keyless signature with verified transparency evidence",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := fixture.StartRegistry(t)
			repository := fmt.Sprintf("kc/keyless-case-%d", i)
			trustedMaterial, err := root.NewTrustedRootFromPath(
				filepath.Join(fixtureDir, "trusted-root.json"),
			)
			require.NoError(t, err, "load pinned Sigstore trusted root")
			imageRef := pushOfflineImageIndex(
				t, host, repository, "fixture", image,
			)

			targetDigest := imageDigest

			if tt.tamperImage {
				tamperedRef, tamperedDigest := fixture.PushImage(
					t, host, fmt.Sprintf("kc/keyless-tampered-%d", i),
				)
				imageRef = tamperedRef.Name()
				targetDigest, err = v1.NewHash(tamperedDigest)
				require.NoError(t, err)
			}

			parsedRef, err := name.ParseReference(imageRef, name.Insecure)
			require.NoError(t, err)

			signatureRef := fmt.Sprintf(
				"%s:sha256-%s.sig",
				parsedRef.Context().Name(),
				targetDigest.Hex,
			)

			signatureTag, err := name.NewTag(signatureRef, name.Insecure)
			require.NoError(t, err)

			if tt.attachSignature {
				signatureImage := readOCIImageFixture(
					t, filepath.Join(fixtureDir, "signature"),
				)
				pushOfflineImageReference(t, signatureRef, signatureImage)
			}

			_, lookupErr := remote.Get(signatureTag)
			if tt.attachSignature {
				require.NoError(t, lookupErr, "expected signature tag to exist")
			} else {
				require.Error(t, lookupErr, "signature tag must be absent")
			}

			verifier, err := newKeylessVerifierWithCheckOpts(
				types.KeylessConfig{
					Subject: tt.subject,
					Issuer:  tt.issuer,
				},
				cosign.CheckOpts{
					TrustedMaterial: trustedMaterial,
				},
			)
			require.NoError(t, err)

			result, err := verifier.Verify(context.Background(), types.VerifyRequest{
				ImageRef: imageRef,
				RegistrySettings: types.RegistrySettings{
					RegistryInsecure: true,
				},
			})
			require.NoError(t, err)

			require.Equal(t, tt.wantVerified, result.Verified)
		})
	}
}

func readOfflineKeylessFixtureMetadata(t *testing.T, dir string) offlineKeylessFixtureMetadata {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	require.NoError(t, err, "read captured fixture metadata")

	var metadata offlineKeylessFixtureMetadata
	require.NoError(t, json.Unmarshal(data, &metadata))
	require.NotEmpty(t, metadata.ImageDigest)
	require.NotEmpty(t, metadata.Subject)
	require.NotEmpty(t, metadata.Issuer)
	return metadata
}
func readOCIImageFixture(t *testing.T, dir string) v1.Image {
	t.Helper()

	path, err := layout.FromPath(dir)
	require.NoError(t, err, "open OCI layout %q", dir)

	layoutIndex, err := path.ImageIndex()
	require.NoError(t, err, "read OCI layout index %q", dir)

	manifest, err := layoutIndex.IndexManifest()
	require.NoError(t, err)

	var fixtureDigest string
	for _, descriptor := range manifest.Manifests {
		if descriptor.Annotations["org.opencontainers.image.ref.name"] == "fixture" {
			fixtureDigest = descriptor.Digest.String()
			break
		}
	}
	require.NotEmpty(t, fixtureDigest, "fixture descriptor not found in %q", dir)

	hash, err := v1.NewHash(fixtureDigest)
	require.NoError(t, err)

	imageGetter, ok := layoutIndex.(interface {
		Image(v1.Hash) (v1.Image, error)
	})
	require.True(t, ok, "OCI layout index does not support loading an image")

	image, err := imageGetter.Image(hash)
	require.NoError(t, err, "load signature image %s", fixtureDigest)

	actualDigest, err := image.Digest()
	require.NoError(t, err)
	require.Equal(t, fixtureDigest, actualDigest.String())

	return image
}

func readOCIImageIndexFixture(t *testing.T, dir string) v1.ImageIndex {
	t.Helper()

	path, err := layout.FromPath(dir)
	require.NoError(t, err, "open OCI layout %q", dir)

	layoutIndex, err := path.ImageIndex()
	require.NoError(t, err, "read OCI layout index %q", dir)

	layoutManifest, err := layoutIndex.IndexManifest()
	require.NoError(t, err)

	var fixtureDigest string
	for _, descriptor := range layoutManifest.Manifests {
		if descriptor.Annotations["org.opencontainers.image.ref.name"] == "fixture" {
			fixtureDigest = descriptor.Digest.String()
			break
		}
	}
	require.NotEmpty(t, fixtureDigest, "fixture descriptor not found in %q", dir)

	hash, err := v1.NewHash(fixtureDigest)
	require.NoError(t, err)

	imageIndex, err := layoutIndex.(interface {
		ImageIndex(v1.Hash) (v1.ImageIndex, error)
	}).ImageIndex(hash)
	require.NoError(t, err, "load image index %s", fixtureDigest)

	actualDigest, err := imageIndex.Digest()
	require.NoError(t, err)
	require.Equal(t, fixtureDigest, actualDigest.String())

	return imageIndex
}

func pushOfflineImageIndex(
	t *testing.T,
	host, repository, tag string,
	index v1.ImageIndex,
) string {
	t.Helper()

	ref := fmt.Sprintf("%s/%s:%s", host, repository, tag)
	parsed, err := name.NewTag(ref, name.Insecure)
	require.NoError(t, err)

	require.NoError(t, remote.WriteIndex(parsed, index),
		"push fixture image index %q to in-memory registry", ref)

	return ref
}

func pushOfflineImageReference(t *testing.T, ref string, image v1.Image) {
	t.Helper()
	tag, err := name.NewTag(ref, name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(tag, image), "push fixture image %q to in-memory registry", ref)
}
