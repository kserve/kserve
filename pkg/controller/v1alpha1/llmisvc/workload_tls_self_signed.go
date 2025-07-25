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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"maps"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

const (
	certificateDuration                      = time.Hour * 24 * 365 * 10 // 10 years
	certificateExpirationRenewBufferDuration = certificateDuration / 5

	certificatesExpirationAnnotation = "certificates.kserve.io/expiration"
)

func (r *LLMInferenceServiceReconciler) reconcileSelfSignedCertsSecret(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) error {
	log.FromContext(ctx).Info("Reconciling self-signed certificates secret")

	// Generating a new certificate is quite slow and expensive as it generates a new certificate, check if the current
	// self-signed certificate (if any) is expired before creating a new one.
	var certFunc createCertFunc = createSelfSignedTLSCertificate
	if curr := r.getExistingSelfSignedCertificate(ctx, llmSvc); curr != nil && (isCertificateExpired(curr) || len(curr.Data["tls.key"]) == 0 || len(curr.Data["tls.crt"]) == 0) {
		certFunc = func() ([]byte, []byte, error) {
			return curr.Data["tls.key"], curr.Data["tls.crt"], nil
		}
	}

	expected, err := r.expectedSelfSignedCertsSecret(llmSvc, certFunc)
	if err != nil {
		return fmt.Errorf("failed to get expected self-signed certificate secret: %w", err)
	}
	if err := Reconcile(ctx, r, llmSvc, &corev1.Secret{}, expected, semanticCertificateSecretIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile self-signed TLS certificate: %w", err)
	}
	return nil
}

type createCertFunc func() ([]byte, []byte, error)

func (r *LLMInferenceServiceReconciler) expectedSelfSignedCertsSecret(llmSvc *v1alpha1.LLMInferenceService, certFunc createCertFunc) (*corev1.Secret, error) {
	keyBytes, certBytes, err := certFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to create self-signed TLS certificate: %w", err)
	}

	expected := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-self-signed-certs"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				"app.kubernetes.io/component": "llminferenceservice-workload",
				"app.kubernetes.io/name":      llmSvc.GetName(),
				"app.kubernetes.io/part-of":   "llminferenceservice",
			},
			Annotations: map[string]string{
				certificatesExpirationAnnotation: time.Now().
					Add(certificateDuration - certificateExpirationRenewBufferDuration).
					Format(time.RFC3339),
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha1.LLMInferenceServiceGVK),
			},
		},
		Data: map[string][]byte{
			"tls.crt": certBytes,
			"tls.key": keyBytes,
		},
		Type: corev1.SecretTypeTLS,
	}
	return expected, nil
}

// createSelfSignedTLSCertificate creates a self-signed cert the server can use to serve TLS.
func createSelfSignedTLSCertificate() ([]byte, []byte, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating serial number: %w", err)
	}
	now := time.Now()
	notBefore := now.UTC()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Kserve Self Signed"},
		},
		NotBefore:             notBefore,
		NotAfter:              now.Add(certificateDuration).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating key: %w", err)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create TLS certificate: %w", err)
	}
	certBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshall TLS private key: %w", err)
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return keyBytes, certBytes, nil
}

func (r *LLMInferenceServiceReconciler) getExistingSelfSignedCertificate(ctx context.Context, llmSvc *v1alpha1.LLMInferenceService) *corev1.Secret {
	curr := &corev1.Secret{}
	key := client.ObjectKey{Namespace: llmSvc.GetNamespace(), Name: kmeta.ChildName(llmSvc.GetName(), "-kserve-self-signed-certs")}
	err := r.Client.Get(ctx, key, curr)
	if err != nil {
		return nil
	}
	return curr
}

func isCertificateExpired(curr *corev1.Secret) bool {
	expires, ok := curr.Annotations[certificatesExpirationAnnotation]
	if ok {
		t, err := time.Parse(time.RFC3339, expires)
		return err == nil && time.Now().UTC().After(t.UTC())
	}
	return false
}

// semanticCertificateSecretIsEqual is a semantic comparison for secrets that is specifically meant to compare TLS
// certificates secrets handling expiration and renewal.
func semanticCertificateSecretIsEqual(expected *corev1.Secret, curr *corev1.Secret) bool {
	if isCertificateExpired(curr) {
		return true
	}

	expectedAnnotations := maps.Clone(expected.Annotations)
	delete(expectedAnnotations, certificatesExpirationAnnotation)

	return equality.Semantic.DeepDerivative(expected.Immutable, curr.Immutable) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expectedAnnotations, curr.Annotations) &&
		equality.Semantic.DeepDerivative(expected.Type, curr.Type)
}
