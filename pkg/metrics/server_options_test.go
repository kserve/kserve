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
	"net/http"
	"os"
	"path/filepath"
	"testing"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestConfigureServerOptionsPreservesExistingOptions(t *testing.T) {
	extraHandler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	input := metricsserver.Options{
		BindAddress: ":8443",
		ExtraHandlers: map[string]http.Handler{
			"/debug": extraHandler,
		},
	}

	options, err := ConfigureServerOptions(input)
	if err != nil {
		t.Fatalf("ConfigureServerOptions() error = %v", err)
	}
	if options.BindAddress != input.BindAddress {
		t.Errorf("BindAddress = %q, want %q", options.BindAddress, input.BindAddress)
	}
	if options.ExtraHandlers["/debug"] == nil {
		t.Error("ExtraHandlers does not contain /debug")
	}
}

func TestConfigureServerOptions(t *testing.T) {
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
			certFiles:         []string{defaultCertName},
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

			options, err := ConfigureServerOptions(metricsserver.Options{
				BindAddress:   ":8443",
				SecureServing: tt.secureServing,
				CertDir:       certPath,
				TLSOpts:       []func(*tls.Config){tlsOpt},
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("ConfigureServerOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantNotExistError && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("ConfigureServerOptions() error = %v, want os.ErrNotExist", err)
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

func TestConfigureServerOptionsWithCertificateFiles(t *testing.T) {
	certPath := t.TempDir()
	for _, fileName := range []string{defaultCertName, defaultKeyName} {
		if err := os.WriteFile(filepath.Join(certPath, fileName), []byte("test"), 0o600); err != nil {
			t.Fatalf("write %s: %v", fileName, err)
		}
	}

	options, err := ConfigureServerOptions(metricsserver.Options{BindAddress: ":8443", SecureServing: true, CertDir: certPath})
	if err != nil {
		t.Fatalf("ConfigureServerOptions() error = %v", err)
	}
	if options.CertDir != certPath {
		t.Errorf("CertDir = %q, want %q", options.CertDir, certPath)
	}
	if options.FilterProvider == nil {
		t.Error("FilterProvider is nil")
	}
}

func TestConfigureServerOptionsWithCustomCertificateFiles(t *testing.T) {
	certPath := t.TempDir()
	const (
		certName = "metrics.crt"
		keyName  = "metrics.key"
	)
	for _, fileName := range []string{certName, keyName} {
		if err := os.WriteFile(filepath.Join(certPath, fileName), []byte("test"), 0o600); err != nil {
			t.Fatalf("write %s: %v", fileName, err)
		}
	}

	options, err := ConfigureServerOptions(metricsserver.Options{
		SecureServing: true,
		CertDir:       certPath,
		CertName:      certName,
		KeyName:       keyName,
	})
	if err != nil {
		t.Fatalf("ConfigureServerOptions() error = %v", err)
	}
	if options.CertName != certName {
		t.Errorf("CertName = %q, want %q", options.CertName, certName)
	}
	if options.KeyName != keyName {
		t.Errorf("KeyName = %q, want %q", options.KeyName, keyName)
	}
}

func TestConfigureServerOptionsRejectsCertificateDirectory(t *testing.T) {
	certPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(certPath, defaultCertName), 0o700); err != nil {
		t.Fatalf("create certificate directory: %v", err)
	}

	_, err := ConfigureServerOptions(metricsserver.Options{BindAddress: ":8443", SecureServing: true, CertDir: certPath})
	if err == nil {
		t.Fatal("ConfigureServerOptions() error = nil, want certificate file error")
	}
}
