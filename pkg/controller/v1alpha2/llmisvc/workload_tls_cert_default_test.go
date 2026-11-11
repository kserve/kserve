//go:build !distro

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

package llmisvc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCreateWorkloadCertificateKey(t *testing.T) {
	injected, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	tests := []struct {
		name     string
		generate func() (*rsa.PrivateKey, error)
		verify   func(g Gomega, key *rsa.PrivateKey)
	}{
		{
			name: "defaults to a fresh RSA 4096 key",
			verify: func(g Gomega, key *rsa.PrivateKey) {
				g.Expect(key.N.BitLen()).To(Equal(4096))
			},
		},
		{
			name:     "uses the injected generator",
			generate: func() (*rsa.PrivateKey, error) { return injected, nil },
			verify: func(g Gomega, key *rsa.PrivateKey) {
				g.Expect(key.Equal(injected)).To(BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			r := &LLMISVCReconciler{GenerateCertificateKey: tt.generate}

			bundle, err := r.createWorkloadCertificate(t.Context(), []string{"svc.ns.svc.cluster.local"}, []string{"10.0.0.1"})()
			g.Expect(err).ToNot(HaveOccurred())

			keyBlock, _ := pem.Decode(bundle.Key)
			g.Expect(keyBlock).ToNot(BeNil())
			parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
			g.Expect(err).ToNot(HaveOccurred())
			key, ok := parsed.(*rsa.PrivateKey)
			g.Expect(ok).To(BeTrue())
			tt.verify(g, key)

			certBlock, _ := pem.Decode(bundle.Cert)
			g.Expect(certBlock).ToNot(BeNil())
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(key.PublicKey.Equal(cert.PublicKey)).To(BeTrue(), "certificate must carry the generated key")
		})
	}
}
