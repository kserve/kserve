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

package tls

import (
	"context"
	"crypto/tls"
	"errors"

	"k8s.io/client-go/rest"
)

// Resolve builds TLS option functions from the provided min version and cipher
// suites strings. When both are empty, it returns hardened Intermediate defaults
// (TLS 1.2, ECDHE AEAD ciphers, ALPN h2/http1.1).
// In the default (upstream) build, ctx and cfg are unused.
func Resolve(_ context.Context, _ *rest.Config, tlsMinVersion, tlsCipherSuites string) ([]func(*tls.Config), error) {
	minVersion, err := parseMinVersion(tlsMinVersion)
	if err != nil {
		return nil, err
	}

	ciphers, err := parseCipherSuites(tlsCipherSuites)
	if err != nil {
		return nil, err
	}

	if minVersion >= tls.VersionTLS13 && len(ciphers) > 0 {
		return nil, errors.New("cipher suites cannot be configured with TLS 1.3 (Go manages TLS 1.3 ciphers internally)")
	}

	return []func(*tls.Config){
		func(c *tls.Config) {
			c.MinVersion = minVersion
			if len(ciphers) > 0 {
				c.CipherSuites = ciphers
			}
			c.NextProtos = []string{"h2", "http/1.1"}
		},
	}, nil
}
