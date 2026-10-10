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

package registry

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/kernelcache/security/fixture"
)

func TestNewTransportAddsConfiguredCAToRootPool(t *testing.T) {
	ca := fixture.NewCA(t)
	_, leafPEM := ca.IssueLeaf(t)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "registry-ca", Namespace: "workload"},
		Data:       map[string]string{"service-ca.crt": string(ca.CertPEM)},
	}
	reader := fake.NewClientBuilder().WithObjects(configMap).Build()

	transport, err := NewTransport(t.Context(), reader, "workload", v1beta1.KernelCacheRegistryConfig{
		CAConfigMapRef: &v1beta1.KernelCacheConfigMapKeyRef{Name: "registry-ca", Key: "service-ca.crt"},
	})
	require.NoError(t, err)

	leafBlock, _ := pem.Decode(leafPEM)
	require.NotNil(t, leafBlock)
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	require.NoError(t, err)
	tlsConfig := transport.(*http.Transport).TLSClientConfig
	require.NotNil(t, tlsConfig)
	_, err = leaf.Verify(x509.VerifyOptions{Roots: tlsConfig.RootCAs, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}})
	require.NoError(t, err)
}

func TestNewTransportRejectsInvalidConfiguredCA(t *testing.T) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "registry-ca", Namespace: "workload"},
		Data:       map[string]string{"service-ca.crt": "not a certificate"},
	}
	reader := fake.NewClientBuilder().WithObjects(configMap).Build()

	_, err := NewTransport(context.Background(), reader, "workload", v1beta1.KernelCacheRegistryConfig{
		CAConfigMapRef: &v1beta1.KernelCacheConfigMapKeyRef{Name: "registry-ca", Key: "service-ca.crt"},
	})
	require.EqualError(t, err, "registry CA ConfigMap workload/registry-ca: key \"service-ca.crt\" contains no valid CA certificate")
}

func TestNewTransportUsesExplicitInsecureMode(t *testing.T) {
	transport, err := NewTransport(context.Background(), nil, "workload", v1beta1.KernelCacheRegistryConfig{Insecure: true})
	require.NoError(t, err)
	require.True(t, transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}
