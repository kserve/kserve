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
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
)

var log = ctrl.Log.WithName("tls")

var tlsVersionMap = map[string]uint16{
	"VersionTLS12": tls.VersionTLS12,
	"VersionTLS13": tls.VersionTLS13,
}

// LegacyHTTP2TLSOpts returns TLS options that restrict NextProtos to http/1.1 only.
// This preserves the legacy --enable-http2=false behavior for backward compatibility.
func LegacyHTTP2TLSOpts() []func(*tls.Config) {
	return []func(*tls.Config){
		func(c *tls.Config) {
			c.NextProtos = []string{"http/1.1"}
		},
	}
}

func parseMinVersion(version string) (uint16, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return tls.VersionTLS12, nil
	}
	v, ok := tlsVersionMap[version]
	if !ok {
		return 0, fmt.Errorf("unrecognized TLS version %q, valid values: %s",
			version, strings.Join(validVersionNames(), ", "))
	}
	return v, nil
}

func parseCipherSuites(commaSeparated string) ([]uint16, error) {
	commaSeparated = strings.TrimSpace(commaSeparated)
	if commaSeparated == "" {
		return nil, nil
	}

	allCiphers := make(map[string]uint16)
	for _, cs := range tls.CipherSuites() {
		// Go does not permit applications to configure TLS 1.3 cipher suites.
		// Only expose suites that can actually be applied to tls.Config.CipherSuites.
		for _, version := range cs.SupportedVersions {
			if version == tls.VersionTLS12 {
				allCiphers[cs.Name] = cs.ID
				break
			}
		}
	}

	var ids []uint16
	var unknown []string
	for _, name := range strings.Split(commaSeparated, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("cipher suites list contains an empty entry")
		}
		if id, ok := allCiphers[name]; ok {
			ids = append(ids, id)
		} else {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown TLS cipher suite(s): %s", strings.Join(unknown, ", "))
	}
	if len(ids) == 0 {
		return nil, errors.New("cipher suites flag was set but no valid suites were specified")
	}
	return ids, nil
}

// Validate checks that a minimum TLS version and cipher-suite list can be
// applied by Go's crypto/tls implementation.
func Validate(tlsMinVersion, tlsCipherSuites string) error {
	minVersion, err := parseMinVersion(tlsMinVersion)
	if err != nil {
		return err
	}
	ciphers, err := parseCipherSuites(tlsCipherSuites)
	if err != nil {
		return err
	}
	if minVersion >= tls.VersionTLS13 && len(ciphers) > 0 {
		return errors.New("cipher suites cannot be configured with TLS 1.3 (Go manages TLS 1.3 ciphers internally)")
	}
	return nil
}

func validVersionNames() []string {
	names := make([]string, 0, len(tlsVersionMap))
	for name := range tlsVersionMap {
		names = append(names, name)
	}
	return names
}
