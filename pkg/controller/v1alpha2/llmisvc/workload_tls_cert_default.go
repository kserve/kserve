//go:build !distro

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

import "context"

// createWorkloadCertificate returns a createCertFunc that generates a self-signed TLS certificate.
// Because the certificate signs itself (issuer = subject, same key pair), it is its own CA -
// ca.crt is therefore identical to tls.crt.
func (r *LLMISVCReconciler) createWorkloadCertificate(_ context.Context, dnsNames []string, ips []string) createCertFunc {
	return func() (*certBundle, error) {
		keyBytes, certBytes, err := createSelfSignedTLSCertificate(dnsNames, ips)
		if err != nil {
			return nil, err
		}
		return &certBundle{Key: keyBytes, Cert: certBytes, CACert: certBytes}, nil
	}
}
