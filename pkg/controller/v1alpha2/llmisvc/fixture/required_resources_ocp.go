//go:build distro

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

package fixture

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

// additionalRequiredResources creates the CA signing key secret required by the
// distro (OpenShift) build. The reconciler's createWorkloadCertificate loads
// this secret to sign workload TLS certificates.
func additionalRequiredResources(ctx context.Context, c client.Client) {
	ns := llmisvc.ServiceCASigningSecretNamespace

	// Create the namespace that holds the CA signing secret.
	gomega.Expect(client.IgnoreAlreadyExists(c.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}))).To(gomega.Succeed())

	// Generate a self-signed CA certificate for tests.
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	caTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"KServe Test CA"},
		},
		NotBefore:             time.Now().UTC(),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour).UTC(),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})

	caKeyBytes, err := x509.MarshalPKCS8PrivateKey(caKey)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: caKeyBytes})

	gomega.Expect(client.IgnoreAlreadyExists(c.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      llmisvc.ServiceCASigningSecretName,
			Namespace: ns,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": caCertPEM,
			"tls.key": caKeyPEM,
			"ca.crt":  caCertPEM,
		},
	}))).To(gomega.Succeed())
}
