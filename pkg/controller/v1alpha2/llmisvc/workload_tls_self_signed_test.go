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

package llmisvc_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

const certificatesExpirationAnnotation = "certificates.kserve.io/expiration"

func generateTestCert(t *testing.T, dnsNames []string, ips []net.IP, notAfter time.Time) ([]byte, []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return keyPEM, certPEM
}

func TestShouldRecreateCertificate(t *testing.T) {
	validKey, validCert := generateTestCert(t,
		[]string{"localhost", "svc.cluster.local"},
		[]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")},
		time.Now().Add(24*time.Hour),
	)

	tests := []struct {
		name             string
		secret           *corev1.Secret
		expectedDNSNames []string
		expectedIPs      []string
		want             bool
	}{
		{
			name:   "nil secret",
			secret: nil,
			want:   true,
		},
		{
			name: "expired annotation",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(-time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": validKey,
					"tls.crt": validCert,
				},
			},
			want: true,
		},
		{
			name: "missing tls.key",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.crt": validCert,
				},
			},
			want: true,
		},
		{
			name: "missing tls.crt",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": validKey,
				},
			},
			want: true,
		},
		{
			name: "invalid cert PEM",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": validKey,
					"tls.crt": []byte("not-a-pem"),
				},
			},
			want: true,
		},
		{
			name: "invalid key PEM",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": []byte("not-a-pem"),
					"tls.crt": validCert,
				},
			},
			want: true,
		},
		{
			name: "DNS names not covered",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": validKey,
					"tls.crt": validCert,
				},
			},
			expectedDNSNames: []string{"localhost", "svc.cluster.local", "new-dns-name.example.com"},
			expectedIPs:      []string{"127.0.0.1"},
			want:             true,
		},
		{
			name: "IPs not covered",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": validKey,
					"tls.crt": validCert,
				},
			},
			expectedDNSNames: []string{"localhost"},
			expectedIPs:      []string{"127.0.0.1", "10.0.0.1", "192.168.1.1"},
			want:             true,
		},
		{
			name: "all covered - no recreation needed",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": validKey,
					"tls.crt": validCert,
				},
			},
			expectedDNSNames: []string{"localhost"},
			expectedIPs:      []string{"127.0.0.1"},
			want:             false,
		},
		{
			name: "superset of expected - no recreation needed",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
				Data: map[string][]byte{
					"tls.key": validKey,
					"tls.crt": validCert,
				},
			},
			expectedDNSNames: []string{"localhost"},
			expectedIPs:      []string{"127.0.0.1"},
			want:             false,
		},
		{
			name: "x509 expired cert",
			secret: func() *corev1.Secret {
				key, cert := generateTestCert(t,
					[]string{"localhost"},
					[]net.IP{net.ParseIP("127.0.0.1")},
					time.Now().Add(-time.Minute), // x509 already expired
				)
				return &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							// annotation says not expired yet
							certificatesExpirationAnnotation: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
						},
					},
					Data: map[string][]byte{
						"tls.key": key,
						"tls.crt": cert,
					},
				}
			}(),
			expectedDNSNames: []string{"localhost"},
			expectedIPs:      []string{"127.0.0.1"},
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmisvc.ShouldRecreateCertificate(tt.secret, tt.expectedDNSNames, tt.expectedIPs)
			if got != tt.want {
				t.Errorf("ShouldRecreateCertificate() = %v, want %v", got, tt.want)
			}
		})
	}
}
