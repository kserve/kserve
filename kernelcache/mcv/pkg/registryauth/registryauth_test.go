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

package registryauth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

// Accepts canonical and alias registries and rejects invalid scopes.
func TestValidateTokenScope(t *testing.T) {
	for _, test := range []struct {
		name       string
		target     string
		configured string
		wantErr    string
	}{
		{name: "matching registry", target: "registry.example.com", configured: "registry.example.com"},
		{name: "matching registry alias", target: "docker.io", configured: "index.docker.io"},
		{name: "matching registry case", target: "Registry.Example.com", configured: "registry.example.com"},
		{name: "different registry", target: "registry.example.com", configured: "registry.internal.example.com", wantErr: "scoped"},
		{name: "missing scope", target: "registry.example.com", configured: "", wantErr: "required"},
		{name: "invalid configured registry", target: "registry.example.com", configured: "https://registry.example.com", wantErr: "invalid"},
		{name: "invalid target registry", target: "https://registry.example.com", configured: "registry.example.com", wantErr: "invalid target"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(tokenRegistryEnv, test.configured)
			err := ValidateTokenScope(test.target)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

// Selects registry credentials and rejects invalid credential configuration.
func TestRemoteOptions(t *testing.T) {
	const target = "registry.example.com"
	tests := []struct {
		name      string
		configure func(t *testing.T)
		wantErr   string
	}{
		{
			name: "uses keychain when no credentials are configured",
		},
		{
			name: "accepts legacy access JSON",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(legacyAccessFileEnv, writeAccessFile(t, publishingCredential{
					Registry: target, Username: "user", Token: "token", ExpiresAt: time.Now().Add(time.Hour),
				}))
			},
		},
		{
			name: "rejects malformed access JSON",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(accessFileEnv, writeFile(t, "access.json", []byte("{")))
			},
			wantErr: "invalid registry access JSON",
		},
		{
			name: "rejects expired access JSON",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(accessFileEnv, writeAccessFile(t, publishingCredential{
					Registry: target, Username: "user", Token: "token", ExpiresAt: time.Now().Add(-time.Hour),
				}))
			},
			wantErr: "missing or expired",
		},
		{
			name: "rejects incomplete access JSON",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(accessFileEnv, writeAccessFile(t, publishingCredential{Registry: target}))
			},
			wantErr: "missing or expired",
		},
		{
			name: "rejects access registry mismatch",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(accessFileEnv, writeAccessFile(t, publishingCredential{
					Registry: "other.example.com", Username: "user", Token: "token", ExpiresAt: time.Now().Add(time.Hour),
				}))
			},
			wantErr: "target mismatch",
		},
		{
			name: "rejects access scope mismatch",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, "other.example.com")
				t.Setenv(accessFileEnv, writeAccessFile(t, publishingCredential{
					Registry: target, Username: "user", Token: "token", ExpiresAt: time.Now().Add(time.Hour),
				}))
			},
			wantErr: "scoped",
		},
		{
			name: "rejects missing access file",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(accessFileEnv, filepath.Join(t.TempDir(), "missing.json"))
			},
			wantErr: "read registry access",
		},
		{
			name: "requires access file when auth is mandatory",
			configure: func(t *testing.T) {
				t.Setenv("MCV_REGISTRY_AUTH_REQUIRED", "true")
			},
			wantErr: "access file is required",
		},
		{
			name: "rejects empty token file",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(tokenFileEnv, writeFile(t, "token", []byte(" \n")))
			},
			wantErr: "token file is empty",
		},
		{
			name: "rejects missing token file",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(tokenFileEnv, filepath.Join(t.TempDir(), "missing-token"))
			},
			wantErr: "read registry token",
		},
		{
			name: "rejects token without scope",
			configure: func(t *testing.T) {
				t.Setenv(tokenFileEnv, writeFile(t, "token", []byte("token")))
			},
			wantErr: "required",
		},
		{
			name: "rejects token scope mismatch",
			configure: func(t *testing.T) {
				t.Setenv(tokenRegistryEnv, "other.example.com")
				t.Setenv(tokenFileEnv, writeFile(t, "token", []byte("token")))
			},
			wantErr: "scoped",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearRegistryEnvironment(t)
			if test.configure != nil {
				test.configure(t)
			}
			options, err := RemoteOptions(context.Background(), target)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, options)
		})
	}
}

// Sends access-file and service-account credentials through remote options.
func TestRemoteOptionsAuth(t *testing.T) {
	for _, test := range []struct {
		name      string
		username  string
		password  string
		configure func(t *testing.T, target string)
	}{
		{
			name:     "access file",
			username: "user",
			password: "access-token",
			configure: func(t *testing.T, target string) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(accessFileEnv, writeAccessFile(t, publishingCredential{
					Registry: target, Username: "user", Token: "access-token", ExpiresAt: time.Now().Add(time.Hour),
				}))
			},
		},
		{
			name:     "token file",
			username: "serviceaccount",
			password: "service-token",
			configure: func(t *testing.T, target string) {
				t.Setenv(tokenRegistryEnv, target)
				t.Setenv(tokenFileEnv, writeFile(t, "token", []byte("service-token")))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearRegistryEnvironment(t)
			server := newAuthServer(t, test.username, test.password)
			target := strings.TrimPrefix(server.URL, "http://")
			test.configure(t, target)
			options, err := RemoteOptions(context.Background(), target)
			require.NoError(t, err)
			ref, err := name.NewTag(target+"/repo:tag", name.Insecure)
			require.NoError(t, err)
			_, err = remote.Get(ref, options...)
			require.NoError(t, err)
		})
	}
}

// Builds a TLS transport that trusts the configured registry CA.
func TestRegistryTransport(t *testing.T) {
	t.Run("missing CA file", func(t *testing.T) {
		_, err := registryTransport(filepath.Join(t.TempDir(), "missing-ca.pem"))
		require.ErrorContains(t, err, "read registry CA")
	})

	t.Run("invalid CA file", func(t *testing.T) {
		caFile := writeFile(t, "ca.pem", []byte("not a certificate"))
		_, err := registryTransport(caFile)
		require.ErrorContains(t, err, "contains no valid certificates")
	})

	t.Run("connects with configured CA", func(t *testing.T) {
		server := newTestTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		caFile := writeFile(t, "ca.pem", pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: server.Certificate().Raw,
		}))

		roundTripper, err := registryTransport(caFile)
		require.NoError(t, err)
		transport, ok := roundTripper.(*http.Transport)
		require.True(t, ok)
		require.EqualValues(t, tls.VersionTLS12, transport.TLSClientConfig.MinVersion)
		require.NotNil(t, transport.TLSClientConfig.RootCAs)

		response, err := (&http.Client{Transport: roundTripper}).Get(server.URL)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusNoContent, response.StatusCode)
	})
}

func clearRegistryEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		caFileEnv, tokenFileEnv, tokenRegistryEnv, accessFileEnv, legacyAccessFileEnv, "MCV_REGISTRY_AUTH_REQUIRED",
	} {
		t.Setenv(key, "")
	}
}

func writeAccessFile(t *testing.T, credential publishingCredential) string {
	t.Helper()
	data, err := json.Marshal(credential)
	require.NoError(t, err)
	return writeFile(t, "access.json", data)
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func newAuthServer(t *testing.T, expectedUsername, expectedPassword string) *httptest.Server {
	t.Helper()
	manifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","size":0},"layers":[]}`
	return newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != expectedUsername || password != expectedPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.Contains(r.URL.Path, "/manifests/") {
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			_, _ = io.WriteString(w, manifest)
			return
		}
		http.NotFound(w, r)
	}))
}

func newTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func newTestTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	server.Listener = listener
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}
