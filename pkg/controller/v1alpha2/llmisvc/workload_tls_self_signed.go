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
	"net"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"knative.dev/pkg/kmeta"
	"knative.dev/pkg/network"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	// Certificate validity period and renewal settings
	certificateDuration                      = time.Hour * 24 * 365 * 10 // 10 years
	certificateExpirationRenewBufferDuration = certificateDuration / 5

	// Annotation to track certificate expiration
	certificatesExpirationAnnotation = "certificates.kserve.io/expiration"
)

// reconcileSelfSignedCertsSecret reconciles the secret containing self-signed certs used by the server to serve TLS.
// These self signed certs are used for cluster internal communication encryption by the workload and the scheduler.
// The certificates are automatically renewed before expiration to ensure continuous secure communication.
func (r *LLMISVCReconciler) reconcileSelfSignedCertsSecret(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) error {
	log.FromContext(ctx).Info("Reconciling self-signed certificates secret")

	ips, err := r.collectIPAddresses(ctx, llmSvc)
	if err != nil {
		return fmt.Errorf("failed to collect IP addresses: %w", err)
	}
	dnsNames := r.collectDNSNames(ctx, llmSvc)

	// Generating a new certificate is quite slow and expensive as it generates a new certificate, check if the current
	// self-signed certificate (if any) is expired before creating a new one.
	var certFunc createCertFunc = func() ([]byte, []byte, error) {
		return createSelfSignedTLSCertificate(dnsNames, ips)
	}
	if curr := r.getExistingSelfSignedCertificate(ctx, llmSvc); curr != nil && !ShouldRecreateCertificate(curr, dnsNames, ips) {
		certFunc = func() ([]byte, []byte, error) {
			return curr.Data["tls.key"], curr.Data["tls.crt"], nil
		}
	}

	expected, err := r.expectedSelfSignedCertsSecret(llmSvc, certFunc)
	if err != nil {
		return fmt.Errorf("failed to get expected self-signed certificate secret: %w", err)
	}

	if utils.GetForceStopRuntime(llmSvc) {
		return Delete(ctx, r, llmSvc, expected)
	}

	if err := Reconcile(ctx, r, llmSvc, &corev1.Secret{}, expected, SemanticCertificateSecretIsEqual); err != nil {
		return fmt.Errorf("failed to reconcile self-signed TLS certificate: %w", err)
	}
	return nil
}

type createCertFunc func() ([]byte, []byte, error)

func (r *LLMISVCReconciler) expectedSelfSignedCertsSecret(llmSvc *v1alpha2.LLMInferenceService, certFunc createCertFunc) (*corev1.Secret, error) {
	keyBytes, certBytes, err := certFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to create self-signed TLS certificate: %w", err)
	}

	expected := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kmeta.ChildName(llmSvc.GetName(), "-kserve-self-signed-certs"),
			Namespace: llmSvc.GetNamespace(),
			Labels: map[string]string{
				constants.KubernetesComponentLabelKey: constants.LLMComponentWorkload,
				constants.KubernetesAppNameLabelKey:   llmSvc.GetName(),
				constants.KubernetesPartOfLabelKey:    constants.LLMInferenceServicePartOfValue,
			},
			Annotations: map[string]string{
				certificatesExpirationAnnotation: time.Now().
					Add(certificateDuration - certificateExpirationRenewBufferDuration).
					Format(time.RFC3339),
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(llmSvc, v1alpha2.LLMInferenceServiceGVK),
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
func createSelfSignedTLSCertificate(dnsNames []string, ipStrings []string) ([]byte, []byte, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating serial number: %w", err)
	}
	ipAddresses := make([]net.IP, 0, len(ipStrings))
	for _, ip := range ipStrings {
		if p := net.ParseIP(ip); p != nil {
			ipAddresses = append(ipAddresses, p)
		}
	}

	now := time.Now()
	notBefore := now.UTC()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Kserve Self Signed"},
		},
		NotBefore:             notBefore,
		NotAfter:              now.Add(certificateDuration + certificateExpirationRenewBufferDuration).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
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

func (r *LLMISVCReconciler) getExistingSelfSignedCertificate(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) *corev1.Secret {
	curr := &corev1.Secret{}
	key := client.ObjectKey{Namespace: llmSvc.GetNamespace(), Name: kmeta.ChildName(llmSvc.GetName(), "-kserve-self-signed-certs")}
	err := r.Get(ctx, key, curr)
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

func ShouldRecreateCertificate(curr *corev1.Secret, expectedDNSNames []string, expectedIPs []string) bool {
	if curr == nil || isCertificateExpired(curr) || len(curr.Data["tls.key"]) == 0 || len(curr.Data["tls.crt"]) == 0 {
		return true
	}

	// Decode PEM-encoded certificate
	certBlock, _ := pem.Decode(curr.Data["tls.crt"])
	if certBlock == nil {
		return true
	}
	cert, certErr := x509.ParseCertificate(certBlock.Bytes)

	// Decode PEM-encoded private key
	keyBlock, _ := pem.Decode(curr.Data["tls.key"])
	if keyBlock == nil {
		return true
	}
	_, keyErr := x509.ParsePKCS8PrivateKey(keyBlock.Bytes) // Must match createSelfSignedTLSCertificate form.

	if certErr != nil || keyErr != nil {
		return true
	}

	expectedDnsNamesSet := sets.NewString(expectedDNSNames...)
	currDnsNames := sets.NewString(cert.DNSNames...)
	if !currDnsNames.IsSuperset(expectedDnsNamesSet) {
		return true
	}

	// Only recreate certificates when the current IPs are not covering all possible IPs to account for temporary
	// changes and avoid too frequent changes [current.IsSuperset(expected)].

	expectedIpSet := sets.NewString(expectedIPs...)
	currIps := sets.NewString()
	for _, ip := range cert.IPAddresses {
		if len(ip) > 0 {
			currIps.Insert(ip.String())
		}
	}

	if !currIps.IsSuperset(expectedIpSet) {
		return true
	}

	return time.Now().UTC().After(cert.NotAfter.UTC())
}

func (r *LLMISVCReconciler) collectDNSNames(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) []string {
	dnsNames := []string{
		"localhost", // P/D sidecar sends requests for decode over localhost
		network.GetServiceHostname(kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"), llmSvc.GetNamespace()),
		fmt.Sprintf("%s.%s.svc", kmeta.ChildName(llmSvc.GetName(), "-kserve-workload-svc"), llmSvc.GetNamespace()),
	}

	if llmSvc.Spec.Router != nil && llmSvc.Spec.Router.Scheduler != nil && llmSvc.Spec.Router.Scheduler.Pool != nil {
		infPoolSpec := llmSvc.Spec.Router.Scheduler.Pool.Spec
		if llmSvc.Spec.Router.Scheduler.Pool.HasRef() {
			infPool := &igwapi.InferencePool{
				ObjectMeta: metav1.ObjectMeta{Namespace: llmSvc.GetNamespace(), Name: llmSvc.Spec.Router.Scheduler.Pool.Ref.Name},
			}

			// If there is an error, this will be reported properly as part of the Router reconciliation.
			if err := r.Get(ctx, client.ObjectKeyFromObject(infPool), infPool); err == nil {
				infPoolSpec = &infPool.Spec
			}
		}

		if infPoolSpec != nil {
			dnsNames = append(dnsNames, network.GetServiceHostname(string(infPoolSpec.EndpointPickerRef.Name), llmSvc.GetNamespace()))
			dnsNames = append(dnsNames, fmt.Sprintf("%s.%s.svc", string(infPoolSpec.EndpointPickerRef.Name), llmSvc.GetNamespace()))
		}
	}

	sort.Strings(dnsNames)
	return dnsNames
}

func (r *LLMISVCReconciler) collectIPAddresses(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService) ([]string, error) {
	pods := &corev1.PodList{}
	listOptions := &client.ListOptions{
		Namespace: llmSvc.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			constants.KubernetesAppNameLabelKey: llmSvc.Name,
			constants.KubernetesPartOfLabelKey:  constants.LLMInferenceServicePartOfValue,
		}),
	}

	if err := r.List(ctx, pods, listOptions); err != nil {
		return nil, fmt.Errorf("failed to list pods associated with LLM inference service: %w", err)
	}

	ips := sets.NewString("127.0.0.1") // P/D sidecar sends requests for decode over local host
	for _, pod := range pods.Items {
		ips.Insert(pod.Status.PodIP)
		for _, ip := range pod.Status.PodIPs {
			ips.Insert(ip.IP)
		}
	}

	services := &corev1.ServiceList{}
	if err := r.List(ctx, services, listOptions); err != nil {
		return nil, fmt.Errorf("failed to list services associated with LLM inference service: %w", err)
	}

	for _, svc := range services.Items {
		ips.Insert(svc.Spec.ClusterIP)
		for _, ip := range svc.Spec.ClusterIPs {
			ips.Insert(ip)
		}
	}

	// List sorts IPs, so that the resulting list is always the same regardless of the order of the IPs.
	return ips.List(), nil
}

// SemanticCertificateSecretIsEqual is a semantic comparison for secrets that is specifically meant to compare TLS
// certificates secrets handling expiration and renewal.
func SemanticCertificateSecretIsEqual(expected *corev1.Secret, curr *corev1.Secret) bool {
	if isCertificateExpired(curr) {
		return false
	}

	expectedAnnotations := maps.Clone(expected.Annotations)
	delete(expectedAnnotations, certificatesExpirationAnnotation)

	return equality.Semantic.DeepDerivative(expected.Immutable, curr.Immutable) &&
		equality.Semantic.DeepDerivative(expected.Labels, curr.Labels) &&
		equality.Semantic.DeepDerivative(expectedAnnotations, curr.Annotations) &&
		equality.Semantic.DeepDerivative(expected.Type, curr.Type) &&
		equality.Semantic.DeepDerivative(expected.Data, curr.Data)
}
