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

package metrics

import (
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewServerOptions(t *testing.T) {
	tlsOpt := func(config *tls.Config) {
		config.MinVersion = tls.VersionTLS13
	}

	tests := []struct {
		name              string
		secureServing     bool
		withCertPath      bool
		wantErr           bool
		wantNotExistError bool
		wantFilter        bool
		certFiles         []string
	}{
		{
			name: "insecure server",
		},
		{
			name:          "secure server with generated certificates",
			secureServing: true,
			wantFilter:    true,
		},
		{
			name:         "certificate path without secure serving",
			withCertPath: true,
			wantErr:      true,
		},
		{
			name:              "secure server with missing certificate files",
			secureServing:     true,
			withCertPath:      true,
			wantErr:           true,
			wantNotExistError: true,
		},
		{
			name:              "secure server with missing key file",
			secureServing:     true,
			withCertPath:      true,
			wantErr:           true,
			wantNotExistError: true,
			certFiles:         []string{certFileName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPath := ""
			if tt.withCertPath {
				certPath = t.TempDir()
				for _, fileName := range tt.certFiles {
					if err := os.WriteFile(filepath.Join(certPath, fileName), []byte("test"), 0o600); err != nil {
						t.Fatalf("write %s: %v", fileName, err)
					}
				}
			}

			options, err := NewServerOptions(":8443", tt.secureServing, certPath, []func(*tls.Config){tlsOpt})
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewServerOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantNotExistError && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("NewServerOptions() error = %v, want os.ErrNotExist", err)
			}
			if tt.wantErr {
				return
			}

			if options.BindAddress != ":8443" {
				t.Errorf("BindAddress = %q, want %q", options.BindAddress, ":8443")
			}
			if options.SecureServing != tt.secureServing {
				t.Errorf("SecureServing = %v, want %v", options.SecureServing, tt.secureServing)
			}
			if len(options.TLSOpts) != 1 {
				t.Errorf("len(TLSOpts) = %d, want 1", len(options.TLSOpts))
			}
			if (options.FilterProvider != nil) != tt.wantFilter {
				t.Errorf("FilterProvider present = %v, want %v", options.FilterProvider != nil, tt.wantFilter)
			}
		})
	}
}

func TestNewServerOptionsWithCertificateFiles(t *testing.T) {
	certPath := t.TempDir()
	for _, fileName := range []string{certFileName, keyFileName} {
		if err := os.WriteFile(filepath.Join(certPath, fileName), []byte("test"), 0o600); err != nil {
			t.Fatalf("write %s: %v", fileName, err)
		}
	}

	options, err := NewServerOptions(":8443", true, certPath, nil)
	if err != nil {
		t.Fatalf("NewServerOptions() error = %v", err)
	}
	if options.CertDir != certPath {
		t.Errorf("CertDir = %q, want %q", options.CertDir, certPath)
	}
	if options.FilterProvider == nil {
		t.Error("FilterProvider is nil")
	}
}

func TestNewServerOptionsRejectsCertificateDirectory(t *testing.T) {
	certPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(certPath, certFileName), 0o700); err != nil {
		t.Fatalf("create certificate directory: %v", err)
	}

	_, err := NewServerOptions(":8443", true, certPath, nil)
	if err == nil {
		t.Fatal("NewServerOptions() error = nil, want certificate file error")
	}
}
