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

// Package registry builds OCI registry clients from KernelCache configuration.
package registry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// NewTransport returns an HTTP transport that trusts the system certificate
// pool and, when configured, the registry CA bundle in the workload namespace.
// The returned transport is a clone and never mutates http.DefaultTransport.
func NewTransport(ctx context.Context, reader client.Reader, namespace string, config v1beta1.KernelCacheRegistryConfig) (http.RoundTripper, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.Insecure {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} // #nosec G402 -- explicit registry.insecure opt-in
		return transport, nil
	}

	ref := config.CAConfigMapRef
	if ref == nil {
		return transport, nil
	}
	if reader == nil {
		return nil, fmt.Errorf("registry CA ConfigMap %q: no reader configured", ref.Name)
	}

	configMap := &corev1.ConfigMap{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, configMap); err != nil {
		return nil, fmt.Errorf("get registry CA ConfigMap %s/%s: %w", namespace, ref.Name, err)
	}
	caPEM, ok := configMap.Data[ref.Key]
	if !ok || caPEM == "" {
		return nil, fmt.Errorf("registry CA ConfigMap %s/%s: key %q not present", namespace, ref.Name, ref.Key)
	}

	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, fmt.Errorf("registry CA ConfigMap %s/%s: key %q contains no valid CA certificate", namespace, ref.Name, ref.Key)
	}

	tlsConfig := transport.TLSClientConfig.Clone()
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	tlsConfig.RootCAs = roots
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}
