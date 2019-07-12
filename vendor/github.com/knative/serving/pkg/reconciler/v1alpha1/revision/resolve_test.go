/*
Copyright 2018 The Knative Authors.

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

package revision

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	fakeclient "k8s.io/client-go/kubernetes/fake"
)

var emptyRegistrySet = sets.NewString()

func mustDigest(t *testing.T, img v1.Image) v1.Hash {
	h, err := img.Digest()
	if err != nil {
		t.Fatalf("Digest() = %v", err)
	}
	return h
}

func mustRawManifest(t *testing.T, img v1.Image) []byte {
	m, err := img.RawManifest()
	if err != nil {
		t.Fatalf("RawManifest() = %v", err)
	}
	return m
}

func fakeRegistry(t *testing.T, repo, username, password string, img v1.Image) *httptest.Server {
	manifestPath := fmt.Sprintf("/v2/%s/manifests/latest", repo)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			// Issue a "Basic" auth challenge, so we can check the auth sent to the registry.
			w.Header().Set("WWW-Authenticate", `Basic `)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		case manifestPath:
			// Check that we get an auth header with base64 encoded username:password
			hdr := r.Header.Get("Authorization")
			if !strings.HasPrefix(hdr, "Basic ") {
				t.Errorf("Header.Get(Authorization); got %v, want Basic prefix", hdr)
			}
			if want := base64.StdEncoding.EncodeToString([]byte(username + ":" + password)); !strings.HasSuffix(hdr, want) {
				t.Errorf("Header.Get(Authorization); got %v, want suffix %v", hdr, want)
			}
			if r.Method != http.MethodGet {
				t.Errorf("Method; got %v, want %v", r.Method, http.MethodGet)
			}
			w.Write(mustRawManifest(t, img))
		default:
			t.Fatalf("Unexpected path: %v", r.URL.Path)
		}
	}))
}

func fakeRegistryPingFailure(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			http.Error(w, "Oops", http.StatusInternalServerError)
		default:
			t.Fatalf("Unexpected path: %v", r.URL.Path)
		}
	}))
}

func fakeRegistryManifestFailure(t *testing.T, repo string) *httptest.Server {
	manifestPath := fmt.Sprintf("/v2/%s/manifests/latest", repo)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			// Issue a "Basic" auth challenge, so we can check the auth sent to the registry.
			w.Header().Set("WWW-Authenticate", `Basic `)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		case manifestPath:
			http.Error(w, "Boom", http.StatusInternalServerError)
		default:
			t.Fatalf("Unexpected path: %v", r.URL.Path)
		}
	}))
}

func TestResolve(t *testing.T) {
	username, password := "foo", "bar"
	ns, svcacct := "user-project", "user-robot"

	img, err := random.Image(3, 1024)
	if err != nil {
		t.Fatalf("random.Image() = %v", err)
	}

	// Stand up a fake registry
	expectedRepo := "booger/nose"
	server := fakeRegistry(t, expectedRepo, username, password, img)
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%v) = %v", server.URL, err)
	}

	// Create a tag pointing to an image on our fake registry
	tag, err := name.NewTag(fmt.Sprintf("%s/%s:latest", u.Host, expectedRepo), name.WeakValidation)
	if err != nil {
		t.Fatalf("NewTag() = %v", err)
	}

	// Set up a fake service account with pull secrets for our fake registry
	client := fakeclient.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcacct,
			Namespace: ns,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{{
			Name: "secret",
		}},
	}, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: ns,
		},
		Type: corev1.SecretTypeDockercfg,
		Data: map[string][]byte{
			corev1.DockerConfigKey: []byte(
				fmt.Sprintf(`{%q: {"username": %q, "password": %q}}`,
					tag.RegistryStr(), username, password),
			),
		},
	})

	// Resolve our tag on the fake registry to the digest of the random.Image()
	dr := &digestResolver{client: client, transport: http.DefaultTransport}
	opt := k8schain.Options{
		Namespace:          ns,
		ServiceAccountName: svcacct,
	}
	resolvedDigest, err := dr.Resolve(tag.String(), opt, emptyRegistrySet)
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}

	// Make sure that we get back the appropriate digest.
	digest, err := name.NewDigest(resolvedDigest, name.WeakValidation)
	if err != nil {
		t.Fatalf("NewDigest() = %v", err)
	}
	if got, want := digest.DigestStr(), mustDigest(t, img).String(); got != want {
		t.Fatalf("Resolve() = %v, want %v", got, want)
	}
}

func TestResolveWithDigest(t *testing.T) {
	ns, svcacct := "foo", "default"
	client := fakeclient.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: ns,
		},
	})
	originalDigest := "ubuntu@sha256:e7def0d56013d50204d73bb588d99e0baa7d69ea1bc1157549b898eb67287612"
	dr := &digestResolver{client: client, transport: http.DefaultTransport}
	opt := k8schain.Options{
		Namespace:          ns,
		ServiceAccountName: svcacct,
	}
	resolvedDigest, err := dr.Resolve(originalDigest, opt, emptyRegistrySet)
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}

	if diff := cmp.Diff(originalDigest, resolvedDigest); diff != "" {
		t.Errorf("Digest should not change (-want +got): %s", diff)
	}
}

func TestResolveWithBadTag(t *testing.T) {
	ns, svcacct := "foo", "default"
	client := fakeclient.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: ns,
		},
	})
	dr := &digestResolver{client: client, transport: http.DefaultTransport}

	opt := k8schain.Options{
		Namespace:          ns,
		ServiceAccountName: svcacct,
	}

	// Invalid character
	invalidImage := "ubuntu%latest"
	if resolvedDigest, err := dr.Resolve(invalidImage, opt, emptyRegistrySet); err == nil {
		t.Fatalf("Resolve() = %v, want error", resolvedDigest)
	}
}

func TestResolveWithPingFailure(t *testing.T) {
	ns, svcacct := "user-project", "user-robot"

	// Stand up a fake registry
	expectedRepo := "booger/nose"
	server := fakeRegistryPingFailure(t)
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%v) = %v", server.URL, err)
	}

	// Create a tag pointing to an image on our fake registry
	tag, err := name.NewTag(fmt.Sprintf("%s/%s:latest", u.Host, expectedRepo), name.WeakValidation)
	if err != nil {
		t.Fatalf("NewTag() = %v", err)
	}

	// Set up a fake service account with pull secrets for our fake registry
	client := fakeclient.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcacct,
			Namespace: ns,
		},
	})

	// Resolve our tag on the fake registry to the digest of the random.Image()
	dr := &digestResolver{client: client, transport: http.DefaultTransport}
	opt := k8schain.Options{
		Namespace:          ns,
		ServiceAccountName: svcacct,
	}
	if resolvedDigest, err := dr.Resolve(tag.String(), opt, emptyRegistrySet); err == nil {
		t.Fatalf("Resolve() = %v, want error", resolvedDigest)
	}
}

func TestResolveWithManifestFailure(t *testing.T) {
	ns, svcacct := "user-project", "user-robot"

	// Stand up a fake registry
	expectedRepo := "booger/nose"
	server := fakeRegistryManifestFailure(t, expectedRepo)
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%v) = %v", server.URL, err)
	}

	// Create a tag pointing to an image on our fake registry
	tag, err := name.NewTag(fmt.Sprintf("%s/%s:latest", u.Host, expectedRepo), name.WeakValidation)
	if err != nil {
		t.Fatalf("NewTag() = %v", err)
	}

	// Set up a fake service account with pull secrets for our fake registry
	client := fakeclient.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcacct,
			Namespace: ns,
		},
	})

	// Resolve our tag on the fake registry to the digest of the random.Image()
	dr := &digestResolver{client: client, transport: http.DefaultTransport}
	opt := k8schain.Options{
		Namespace:          ns,
		ServiceAccountName: svcacct,
	}
	if resolvedDigest, err := dr.Resolve(tag.String(), opt, emptyRegistrySet); err == nil {
		t.Fatalf("Resolve() = %v, want error", resolvedDigest)
	}
}

func TestResolveNoAccess(t *testing.T) {
	ns, svcacct := "foo", "default"
	client := fakeclient.NewSimpleClientset()
	dr := &digestResolver{client: client, transport: http.DefaultTransport}
	opt := k8schain.Options{
		Namespace:          ns,
		ServiceAccountName: svcacct,
	}
	// If there is a failure accessing the ServiceAccount for this Pod, then we should see an error.
	if resolvedDigest, err := dr.Resolve("ubuntu:latest", opt, emptyRegistrySet); err == nil {
		t.Fatalf("Resolve() = %v, want error", resolvedDigest)
	}
}

func TestResolveSkippingRegistry(t *testing.T) {
	username, password := "foo", "bar"
	ns, svcacct := "user-project", "user-robot"

	// Set up a fake service account with pull secrets for our fake registry
	client := fakeclient.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcacct,
			Namespace: ns,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{{
			Name: "secret",
		}},
	}, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret",
			Namespace: ns,
		},
		Type: corev1.SecretTypeDockercfg,
		Data: map[string][]byte{
			corev1.DockerConfigKey: []byte(
				fmt.Sprintf(`{%q: {"username": %q, "password": %q}}`,
					"localhost:5000", username, password),
			),
		},
	})
	dr := &digestResolver{
		client:    client,
		transport: http.DefaultTransport,
	}

	registriesToSkip := sets.NewString("localhost:5000")

	opt := k8schain.Options{
		Namespace:          ns,
		ServiceAccountName: svcacct,
	}

	resolvedDigest, err := dr.Resolve("localhost:5000/ubuntu:latest", opt, registriesToSkip)
	if err != nil {
		t.Fatalf("Resolve() = %v", err)
	}

	if got, want := resolvedDigest, ""; got != want {
		t.Fatalf("Resolve() got %q want of %q", got, want)
	}
}

func TestNewResolverTransport(t *testing.T) {
	// Cert stolen from crypto/x509/example_test.go
	const certPEM = `
-----BEGIN CERTIFICATE-----
MIIDujCCAqKgAwIBAgIIE31FZVaPXTUwDQYJKoZIhvcNAQEFBQAwSTELMAkGA1UE
BhMCVVMxEzARBgNVBAoTCkdvb2dsZSBJbmMxJTAjBgNVBAMTHEdvb2dsZSBJbnRl
cm5ldCBBdXRob3JpdHkgRzIwHhcNMTQwMTI5MTMyNzQzWhcNMTQwNTI5MDAwMDAw
WjBpMQswCQYDVQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwN
TW91bnRhaW4gVmlldzETMBEGA1UECgwKR29vZ2xlIEluYzEYMBYGA1UEAwwPbWFp
bC5nb29nbGUuY29tMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEfRrObuSW5T7q
5CnSEqefEmtH4CCv6+5EckuriNr1CjfVvqzwfAhopXkLrq45EQm8vkmf7W96XJhC
7ZM0dYi1/qOCAU8wggFLMB0GA1UdJQQWMBQGCCsGAQUFBwMBBggrBgEFBQcDAjAa
BgNVHREEEzARgg9tYWlsLmdvb2dsZS5jb20wCwYDVR0PBAQDAgeAMGgGCCsGAQUF
BwEBBFwwWjArBggrBgEFBQcwAoYfaHR0cDovL3BraS5nb29nbGUuY29tL0dJQUcy
LmNydDArBggrBgEFBQcwAYYfaHR0cDovL2NsaWVudHMxLmdvb2dsZS5jb20vb2Nz
cDAdBgNVHQ4EFgQUiJxtimAuTfwb+aUtBn5UYKreKvMwDAYDVR0TAQH/BAIwADAf
BgNVHSMEGDAWgBRK3QYWG7z2aLV29YG2u2IaulqBLzAXBgNVHSAEEDAOMAwGCisG
AQQB1nkCBQEwMAYDVR0fBCkwJzAloCOgIYYfaHR0cDovL3BraS5nb29nbGUuY29t
L0dJQUcyLmNybDANBgkqhkiG9w0BAQUFAAOCAQEAH6RYHxHdcGpMpFE3oxDoFnP+
gtuBCHan2yE2GRbJ2Cw8Lw0MmuKqHlf9RSeYfd3BXeKkj1qO6TVKwCh+0HdZk283
TZZyzmEOyclm3UGFYe82P/iDFt+CeQ3NpmBg+GoaVCuWAARJN/KfglbLyyYygcQq
0SgeDh8dRKUiaW3HQSoYvTvdTuqzwK4CXsr3b5/dAOY8uMuG/IAR3FgwTbZ1dtoW
RvOTa8hYiU6A475WuZKyEHcwnGYe57u2I2KbMgcKjPniocj4QzgYsVAVKW3IwaOh
yE+vPxsiUkvQHdO2fojCkY8jg70jxM+gu59tPDNbw3Uh/2Ij310FgTHsnGQMyA==
-----END CERTIFICATE-----`

	cases := []struct {
		name string

		certBundle         string
		certBundleContents []byte

		wantErr bool
	}{{
		name:               "valid cert",
		certBundle:         "valid-cert.crt",
		certBundleContents: []byte(certPEM),
		wantErr:            false,
	}, {
		// Fails with file not found for path.
		name:               "cert not found",
		certBundle:         "not-found.crt",
		certBundleContents: nil,
		wantErr:            true,
	}, {
		// Fails with invalid cert for path.
		name:               "invalid cert",
		certBundle:         "invalid-cert.crt",
		certBundleContents: []byte("this will not parse"),
		wantErr:            true,
	}}

	tmpDir, err := ioutil.TempDir("", "TestNewResolverTransport-")
	if err != nil {
		t.Fatalf("failed to create tempdir for certs: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for i, tc := range cases {
		i, tc := i, tc
		t.Run(fmt.Sprintf("cases[%d]", i), func(t *testing.T) {
			// Setup.
			path, err := writeCertFile(tmpDir, tc.certBundle, tc.certBundleContents)
			if err != nil {
				t.Fatalf("Failed to write cert bundle file: %v", err)
			}

			// The actual test.
			if tr, err := newResolverTransport(path); err != nil && !tc.wantErr {
				t.Errorf("Got unexpected err: %v", err)
			} else if tc.wantErr && err == nil {
				t.Error("Didn't get an error when we wanted it")
			} else if err == nil {
				// If we didn't get an error, make sure everything we wanted to happen happened.
				subjects := tr.TLSClientConfig.RootCAs.Subjects()

				if !containsSubject(t, subjects, tc.certBundleContents) {
					t.Error("cert pool does not contain certBundleContents")
				}
			}
		})
	}
}

func writeCertFile(dir, path string, contents []byte) (string, error) {
	fp := filepath.Join(dir, path)
	if contents != nil {
		if err := ioutil.WriteFile(fp, contents, os.ModePerm); err != nil {
			return "", err
		}
	}
	return fp, nil
}

func containsSubject(t *testing.T, subjects [][]byte, contents []byte) bool {
	block, _ := pem.Decode([]byte(contents))
	if block == nil {
		t.Fatal("failed to parse certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	for _, b := range subjects {
		if bytes.EqualFold(b, cert.RawSubject) {
			return true
		}
	}

	return false
}
