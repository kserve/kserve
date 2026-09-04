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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kserve/kserve/pkg/kernelcache/types"
)

// A reusable signer whose leaf expires after construction must reject further
// signing: the re-check at the top of Sign uses the signer's clock, so we can
// advance it past NotAfter synchronously instead of sleeping. The expiry check
// runs before any registry access, so an unreachable ref never gets touched.
func TestCertSigner_RejectsExpiredLeafAtSignTime(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	notAfter := time.Now().Add(time.Hour)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	// Advance the clock past the leaf's NotAfter.
	s := &certSigner{
		leafCert:  leaf,
		certChain: []*x509.Certificate{leaf},
		now:       func() time.Time { return notAfter.Add(time.Minute) },
	}

	_, err = s.Sign(context.Background(), types.SignRequest{ImageRef: "example.com/kc/whatever:latest"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate chain element 0 is no longer valid")
}

func TestCertSigner_RejectsExpiredIssuerAtSignTime(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(time.Hour),
	}
	issuer := &x509.Certificate{
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(-time.Minute),
	}
	s := &certSigner{
		certChain: []*x509.Certificate{leaf, issuer},
		now:       func() time.Time { return now },
	}

	_, err := s.Sign(context.Background(), types.SignRequest{ImageRef: "example.com/kc/whatever:latest"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate chain element 1 is no longer valid")
}
