/*
Copyright 2023 The KServe Authors.

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

package storage

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCreateNewFileValidation(t *testing.T) {
	tests := []struct {
		name          string
		inputPath     string
		shouldSucceed bool
		description   string
	}{
		{
			name:          "Valid absolute path",
			inputPath:     "/tmp/test/model.bin",
			shouldSucceed: true,
			description:   "Should allow valid absolute paths",
		},
		{
			name:          "Valid relative path",
			inputPath:     "models/pytorch_model.bin",
			shouldSucceed: true,
			description:   "Should allow valid relative paths",
		},
		{
			name:          "Current directory reference",
			inputPath:     ".",
			shouldSucceed: false,
			description:   "Should reject current directory reference",
		},
		{
			name:          "Parent directory reference",
			inputPath:     "..",
			shouldSucceed: false,
			description:   "Should reject parent directory reference",
		},
		{
			name:          "Empty string",
			inputPath:     "",
			shouldSucceed: false,
			description:   "Should reject empty paths",
		},
		{
			name:          "Path that resolves to current dir",
			inputPath:     "./././.",
			shouldSucceed: false,
			description:   "Should reject paths that resolve to current directory",
		},
		{
			name:          "Path that resolves to parent dir",
			inputPath:     "../../../..",
			shouldSucceed: false,
			description:   "Should reject paths that resolve to parent directory",
		},
		{
			name:          "Path with traversal but valid destination",
			inputPath:     "/tmp/../model.bin",
			shouldSucceed: false,
			description:   "Should reject any path containing '..' even if it resolves to valid location",
		},
		// Protected paths test cases
		{
			name:          "Direct /etc path",
			inputPath:     "/etc",
			shouldSucceed: false,
			description:   "Should reject direct access to /etc directory",
		},
		{
			name:          "File in /etc directory",
			inputPath:     "/etc/passwd",
			shouldSucceed: false,
			description:   "Should reject files in /etc directory",
		},
		{
			name:          "Subdirectory in /etc",
			inputPath:     "/etc/ssl/certs/ca-bundle.crt",
			shouldSucceed: false,
			description:   "Should reject files in /etc subdirectories",
		},
		{
			name:          "Direct /bin path",
			inputPath:     "/bin",
			shouldSucceed: false,
			description:   "Should reject direct access to /bin directory",
		},
		{
			name:          "File in /bin directory",
			inputPath:     "/bin/bash",
			shouldSucceed: false,
			description:   "Should reject files in /bin directory",
		},
		{
			name:          "Direct /dev path",
			inputPath:     "/dev",
			shouldSucceed: false,
			description:   "Should reject direct access to /dev directory",
		},
		{
			name:          "File in /dev directory",
			inputPath:     "/dev/null",
			shouldSucceed: false,
			description:   "Should reject files in /dev directory",
		},
		{
			name:          "Direct /usr/bin path",
			inputPath:     "/usr/bin",
			shouldSucceed: false,
			description:   "Should reject direct access to /usr/bin directory",
		},
		{
			name:          "File in /usr/bin directory",
			inputPath:     "/usr/bin/python",
			shouldSucceed: false,
			description:   "Should reject files in /usr/bin directory",
		},
		{
			name:          "Direct /sbin path",
			inputPath:     "/sbin",
			shouldSucceed: false,
			description:   "Should reject direct access to /sbin directory",
		},
		{
			name:          "File in /sbin directory",
			inputPath:     "/sbin/init",
			shouldSucceed: false,
			description:   "Should reject files in /sbin directory",
		},
		{
			name:          "Direct /usr/sbin path",
			inputPath:     "/usr/sbin",
			shouldSucceed: false,
			description:   "Should reject direct access to /usr/sbin directory",
		},
		{
			name:          "File in /usr/sbin directory",
			inputPath:     "/usr/sbin/nginx",
			shouldSucceed: false,
			description:   "Should reject files in /usr/sbin directory",
		},
		// Valid paths that are NOT protected
		{
			name:          "Valid /tmp file",
			inputPath:     "/tmp/model.bin",
			shouldSucceed: true,
			description:   "Should allow files in /tmp directory",
		},
		{
			name:          "Valid /tmp subdirectory file",
			inputPath:     "/tmp/models/pytorch_model.bin",
			shouldSucceed: true,
			description:   "Should allow files in /tmp subdirectories",
		},
		{
			name:          "Valid relative path in current working dir",
			inputPath:     "workdir/model.bin",
			shouldSucceed: true,
			description:   "Should allow relative paths in working directory",
		},
		{
			name:          "Valid relative path with subdirs",
			inputPath:     "models/deep-learning/pytorch_model.bin",
			shouldSucceed: true,
			description:   "Should allow relative paths with subdirectories",
		},
		// Edge cases - paths that look similar but should be allowed
		{
			name:          "Path with 'etc' in middle",
			inputPath:     "etc-backup/config.json",
			shouldSucceed: true,
			description:   "Should allow paths that contain 'etc' but are not in /etc",
		},
		{
			name:          "Path with 'bin' in middle",
			inputPath:     "bin-files/binary.dat",
			shouldSucceed: true,
			description:   "Should allow paths that contain 'bin' but are not in /bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the actual createNewFile method
			file, err := createNewFile(tt.inputPath)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Test %s: expected success but got error: %v", tt.name, err)
				} else {
					// Clean up the created file
					if file != nil {
						if closeErr := file.Close(); closeErr != nil {
							t.Logf("Failed to close file: %v", closeErr)
						}
						// Only remove if it was actually created
						if _, statErr := os.Stat(tt.inputPath); statErr == nil {
							if removeErr := os.Remove(tt.inputPath); removeErr != nil {
								t.Logf("Failed to remove file: %v", removeErr)
							}
						}
					}
					t.Logf("Test %s: SUCCESS - %s", tt.name, tt.inputPath)
				}
			} else {
				if err == nil {
					// Clean up if file was unexpectedly created
					if file != nil {
						if closeErr := file.Close(); closeErr != nil {
							t.Logf("Failed to close file: %v", closeErr)
						}
						if removeErr := os.Remove(tt.inputPath); removeErr != nil {
							t.Logf("Failed to remove file: %v", removeErr)
						}
					}
					t.Errorf("Test %s: expected error but file was created: %s", tt.name, tt.inputPath)
				} else {
					t.Logf("Test %s: REJECTED - %s (error: %v)", tt.name, tt.inputPath, err)
				}
			}
		})
	}
}

func TestValidateHTTPURLRejectsUnsafeDestinations(t *testing.T) {
	tests := []struct {
		name       string
		storageURI string
	}{
		{
			name:       "IPv4 loopback",
			storageURI: "http://127.0.0.1/model.joblib",
		},
		{
			name:       "localhost",
			storageURI: "http://localhost/model.joblib",
		},
		{
			name:       "cloud metadata endpoint",
			storageURI: "http://169.254.169.254/latest/meta-data",
		},
		{
			name:       "private IPv4",
			storageURI: "https://10.0.0.1/model.joblib",
		},
		{
			name:       "IPv6 loopback",
			storageURI: "http://[::1]/model.joblib",
		},
		{
			name:       "IPv6 unique local",
			storageURI: "http://[fd00::1]/model.joblib",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := GetProvider(map[Protocol]Provider{}, HTTP)
			if err != nil {
				t.Fatalf("failed to get HTTP provider: %v", err)
			}

			err = provider.DownloadModel(t.TempDir(), "model", tt.storageURI)
			if err == nil {
				t.Fatalf("expected %s to be rejected", tt.storageURI)
			}
			if !strings.Contains(err.Error(), "blocked unsafe HTTP(S) storage destination") {
				t.Fatalf("expected unsafe destination error, got: %v", err)
			}
		})
	}
}

func TestRestrictedHTTPTransportAllowsPublicDestination(t *testing.T) {
	const (
		modelContents = "model contents"
		storageURI    = "http://93.184.216.34/model.joblib"
	)

	client := &http.Client{
		Transport: restrictedHTTPTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != storageURI {
					t.Fatalf("request URL = %q, want %q", req.URL.String(), storageURI)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
					Body:       io.NopCloser(strings.NewReader(modelContents)),
					Request:    req,
				}, nil
			}),
		},
		CheckRedirect: checkHTTPStorageRedirect,
	}

	provider := &HTTPSProvider{Client: client}
	modelDir := t.TempDir()
	if err := provider.DownloadModel(modelDir, "model", storageURI); err != nil {
		t.Fatalf("expected public destination download to succeed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(modelDir, "model", "model.joblib"))
	if err != nil {
		t.Fatalf("failed to read downloaded model: %v", err)
	}
	if string(got) != modelContents {
		t.Fatalf("downloaded contents = %q, want %q", string(got), modelContents)
	}
}

func TestHTTPStorageClientRejectsRedirectToUnsafeDestination(t *testing.T) {
	const storageURI = "http://93.184.216.34/model.joblib"
	requests := 0
	client := &http.Client{
		Transport: restrictedHTTPTransport{
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"http://127.0.0.1/metadata"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			}),
		},
		CheckRedirect: checkHTTPStorageRedirect,
	}

	resp, err := client.Get(storageURI)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected redirect to unsafe destination to be rejected")
	}
	if !strings.Contains(err.Error(), "blocked unsafe HTTP(S) storage destination") {
		t.Fatalf("expected unsafe destination error, got: %v", err)
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1", requests)
	}
}
