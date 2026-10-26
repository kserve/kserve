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
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"testing"

	"github.com/stretchr/testify/require"
)

// LeafKeyPEM encodes an EC private key as PEM. With pkcs8=true it uses PKCS#8
// ("PRIVATE KEY"); otherwise SEC1 ("EC PRIVATE KEY"). cert-manager emits SEC1
// unless privateKey.encoding: PKCS8 is set, so a signer must accept both.
func LeafKeyPEM(tb testing.TB, key *ecdsa.PrivateKey, pkcs8 bool) []byte {
	tb.Helper()
	if pkcs8 {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		require.NoError(tb, err)
		return pemBlock("PRIVATE KEY", der)
	}
	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(tb, err)
	return pemBlock("EC PRIVATE KEY", der)
}

// RSAKeyPEMPKCS1 encodes an RSA private key as PKCS#1 PEM ("RSA PRIVATE KEY").
// cert-manager can emit this layout, so a signer must accept it.
func RSAKeyPEMPKCS1(tb testing.TB, key *rsa.PrivateKey) []byte {
	tb.Helper()
	der := x509.MarshalPKCS1PrivateKey(key)
	return pemBlock("RSA PRIVATE KEY", der)
}

// SigningSecret assembles a cert-manager style Secret map (tls.key / tls.crt /
// ca.crt) so a signer can load its material from a FakeSecretSource.
func SigningSecret(keyPEM, leafCertPEM, caChainPEM []byte) map[string][]byte {
	return map[string][]byte{
		"tls.key": keyPEM,
		"tls.crt": leafCertPEM,
		"ca.crt":  caChainPEM,
	}
}
