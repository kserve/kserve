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

package llmisvc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/constants"
)

var (
	ServiceCASigningSecretName      = constants.GetEnvOrDefault("SERVICE_CA_SIGNING_SECRET_NAME", "signing-key")
	ServiceCASigningSecretNamespace = constants.GetEnvOrDefault("SERVICE_CA_SIGNING_SECRET_NAMESPACE", "openshift-service-ca")
)

// createWorkloadCertificate returns a createCertFunc that generates a CA-signed TLS certificate.
// It loads the CA certificate from the OpenShift service-ca secret and signs the certificate with it.
func (r *LLMISVCReconciler) createWorkloadCertificate(ctx context.Context, dnsNames []string, ips []string) createCertFunc {
	return func() (*certBundle, error) {
		caCert, caSigner, caCertPEM, err := loadCAFromSecret(ctx, r.Client, ServiceCASigningSecretName, ServiceCASigningSecretNamespace)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate (required for signing): %w", err)
		}

		serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
		serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
		if err != nil {
			return nil, fmt.Errorf("error creating serial number: %w", err)
		}
		ipAddresses := make([]net.IP, 0, len(ips))
		for _, ip := range ips {
			if p := net.ParseIP(ip); p != nil {
				ipAddresses = append(ipAddresses, p)
			}
		}

		log.FromContext(ctx).Info("Creating CA-signed certificate", "ips", ips, "dnsNames", dnsNames)

		now := time.Now()
		template := x509.Certificate{
			SerialNumber: serialNumber,
			Subject: pkix.Name{
				Organization: []string{"Kserve CA Signed"},
			},
			NotBefore:             now.UTC(),
			NotAfter:              now.Add(certificateDuration + certificateExpirationRenewBufferDuration).UTC(),
			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
			DNSNames:              dnsNames,
			IPAddresses:           ipAddresses,
			SignatureAlgorithm:    x509.SHA256WithRSA,
		}

		priv, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, fmt.Errorf("error generating key: %w", err)
		}

		// Sign the certificate with the CA.
		derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caSigner)
		if err != nil {
			return nil, fmt.Errorf("failed to create CA-signed TLS certificate: %w", err)
		}
		certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

		privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			return nil, fmt.Errorf("failed to marshall TLS private key: %w", err)
		}
		keyBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

		return &certBundle{Key: keyBytes, Cert: certBytes, CACert: caCertPEM}, nil
	}
}

// loadCAFromSecret loads a CA certificate and private key from a Kubernetes secret.
// It returns a crypto.Signer to support both RSA and ECDSA CA keys.
func loadCAFromSecret(ctx context.Context, c client.Client, secretName, secretNamespace string) (*x509.Certificate, crypto.Signer, []byte, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: secretNamespace, Name: secretName}, secret); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get CA secret %s/%s: %w", secretNamespace, secretName, err)
	}

	certPEM, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, nil, nil, fmt.Errorf("CA secret %s/%s does not contain tls.crt", secretNamespace, secretName)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, nil, errors.New("failed to decode certificate PEM from CA secret")
	}
	caCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	keyPEM, ok := secret.Data["tls.key"]
	if !ok {
		return nil, nil, nil, fmt.Errorf("CA secret %s/%s does not contain tls.key", secretNamespace, secretName)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, nil, errors.New("failed to decode private key PEM from CA secret")
	}

	// Try PKCS8 first (handles RSA, ECDSA, Ed25519), then fall back to PKCS1 (RSA only).
	var signer crypto.Signer
	if key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		var ok bool
		signer, ok = key.(crypto.Signer)
		if !ok {
			return nil, nil, nil, errors.New("CA private key does not implement crypto.Signer")
		}
	} else if key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		signer = key
	} else {
		return nil, nil, nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	caCertPEM := certPEM
	if v, ok := secret.Data["ca.crt"]; ok && len(v) > 0 {
		caCertPEM = v
	}

	return caCert, signer, caCertPEM, nil
}
