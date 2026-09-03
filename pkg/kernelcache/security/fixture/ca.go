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

// Package fixture provides reusable test support for the kernel cache security
// package and its consumers: an in-memory OCI registry, obviously test-only CA
// and leaf certificates, cosign signing helpers, and an in-memory SecretSource.
package fixture

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCA is a self-signed CA for tests, with its PEM encoding.
type TestCA struct {
	Cert    *x509.Certificate
	Key     *ecdsa.PrivateKey
	CertPEM []byte
}

type caOptions struct {
	commonName string
}

// CAOption customises a test CA.
type CAOption func(*caOptions)

// WithCACommonName overrides the CA subject common name (e.g. to mint a second,
// distinct "untrusted" CA whose cert dump is unambiguous).
func WithCACommonName(cn string) CAOption {
	return func(o *caOptions) { o.commonName = cn }
}

// NewCA mints a self-signed CA whose subject makes clear it is test-only.
func NewCA(tb testing.TB, opts ...CAOption) *TestCA {
	tb.Helper()
	o := caOptions{commonName: "kserve-kernelcache-test-ca"}
	for _, f := range opts {
		f(&o)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(tb, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: o.commonName, Organization: []string{"KServe TEST (DO NOT TRUST)"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(tb, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(tb, err)
	return &TestCA{Cert: cert, Key: key, CertPEM: pemBlock("CERTIFICATE", der)}
}

type leafOptions struct {
	san       string
	notBefore time.Time
	notAfter  time.Time
}

// LeafOption customises an issued leaf certificate.
type LeafOption func(*leafOptions)

// WithSAN sets the leaf certificate URI SAN (the signer identity).
func WithSAN(uri string) LeafOption {
	return func(o *leafOptions) { o.san = uri }
}

// WithValidity sets an explicit validity window.
func WithValidity(notBefore, notAfter time.Time) LeafOption {
	return func(o *leafOptions) { o.notBefore, o.notAfter = notBefore, notAfter }
}

// Expired makes the leaf certificate already expired.
func Expired() LeafOption {
	return func(o *leafOptions) {
		o.notBefore = time.Now().Add(-48 * time.Hour)
		o.notAfter = time.Now().Add(-24 * time.Hour)
	}
}

// IssueLeaf issues a signing (leaf) cert from the CA. Defaults: a spiffe URI SAN
// and a validity window of an hour ago for 24 hours.
func (ca *TestCA) IssueLeaf(tb testing.TB, opts ...LeafOption) (leafKey *ecdsa.PrivateKey, leafCertPEM []byte) {
	tb.Helper()
	o := leafOptions{
		san:       "spiffe://kserve-test/kc-signer",
		notBefore: time.Now().Add(-time.Hour),
		notAfter:  time.Now().Add(24 * time.Hour),
	}
	for _, f := range opts {
		f(&o)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(tb, err)
	u, err := url.Parse(o.san)
	require.NoError(tb, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "kc-signer"},
		NotBefore:    o.notBefore,
		NotAfter:     o.notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		URIs:         []*url.URL{u},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.Key)
	require.NoError(tb, err)
	return key, pemBlock("CERTIFICATE", der)
}

func pemBlock(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}
