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
	"strings"
	"testing"
)

func TestParseMinVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint16
		wantErr bool
	}{
		{"empty defaults to TLS 1.2", "", tls.VersionTLS12, false},
		{"TLS 1.0 rejected", "VersionTLS10", 0, true},
		{"TLS 1.1 rejected", "VersionTLS11", 0, true},
		{"TLS 1.2", "VersionTLS12", tls.VersionTLS12, false},
		{"TLS 1.3", "VersionTLS13", tls.VersionTLS13, false},
		{"whitespace trimmed", " VersionTLS12 ", tls.VersionTLS12, false},
		{"unknown version", "VersionTLS99", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMinVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseMinVersion(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseMinVersion(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseMinVersion(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCipherSuites(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{"empty returns nil", "", 0, false},
		{"whitespace only returns nil", "   ", 0, false},
		{"single valid cipher", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256", 1, false},
		{"multiple valid ciphers", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384", 2, false},
		{"whitespace trimmed", " TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 , TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384 ", 2, false},
		{"unknown cipher is error", "BOGUS_CIPHER", 0, true},
		{"mixed valid and invalid is error", "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,BOGUS", 0, true},
		{"only commas is error", ",,", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCipherSuites(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseCipherSuites(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseCipherSuites(%q) unexpected error: %v", tt.input, err)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("parseCipherSuites(%q) returned %d ciphers, want %d", tt.input, len(got), tt.wantCount)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name           string
		minVersion     string
		cipherSuites   string
		wantErr        bool
		errContains    string
		wantMinVersion uint16
		wantCiphers    int
	}{
		{
			name:           "defaults: TLS 1.2, no ciphers",
			wantMinVersion: tls.VersionTLS12,
		},
		{
			name:           "explicit TLS 1.2",
			minVersion:     "VersionTLS12",
			wantMinVersion: tls.VersionTLS12,
		},
		{
			name:           "TLS 1.3, no ciphers",
			minVersion:     "VersionTLS13",
			wantMinVersion: tls.VersionTLS13,
		},
		{
			name:           "TLS 1.2 with ciphers",
			minVersion:     "VersionTLS12",
			cipherSuites:   "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			wantMinVersion: tls.VersionTLS12,
			wantCiphers:    2,
		},
		{
			name:           "empty version with ciphers defaults to TLS 1.2",
			cipherSuites:   "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			wantMinVersion: tls.VersionTLS12,
			wantCiphers:    1,
		},
		{
			name:        "invalid version",
			minVersion:  "bogus",
			wantErr:     true,
			errContains: "unrecognized TLS version",
		},
		{
			name:         "unknown cipher",
			cipherSuites: "BOGUS_CIPHER",
			wantErr:      true,
			errContains:  "unknown TLS cipher suite",
		},
		{
			name:         "TLS 1.3 with ciphers is error",
			minVersion:   "VersionTLS13",
			cipherSuites: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			wantErr:      true,
			errContains:  "cannot be configured with TLS 1.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Resolve(context.Background(), nil, tt.minVersion, tt.cipherSuites)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve() expected error containing %q, got nil", tt.errContains)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Resolve() error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("Resolve() returned %d TLSOpts, want 1", len(result))
			}
			cfg := &tls.Config{} //nolint:gosec // intentionally empty to test TLS opts
			result[0](cfg)
			if cfg.MinVersion != tt.wantMinVersion {
				t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tt.wantMinVersion)
			}
			if len(cfg.CipherSuites) != tt.wantCiphers {
				t.Errorf("CipherSuites count = %d, want %d", len(cfg.CipherSuites), tt.wantCiphers)
			}
			if len(cfg.NextProtos) != 2 || cfg.NextProtos[0] != "h2" || cfg.NextProtos[1] != "http/1.1" {
				t.Errorf("NextProtos = %v, want [h2 http/1.1]", cfg.NextProtos)
			}
		})
	}
}
