// Copyright 2018 Google LLC All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/types"
)

func TestGetSchema1(t *testing.T) {
	expectedRepo := "foo/bar"
	manifestPath := fmt.Sprintf("/v2/%s/manifests/latest", expectedRepo)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.WriteHeader(http.StatusOK)
		case manifestPath:
			if r.Method != http.MethodGet {
				t.Errorf("Method; got %v, want %v", r.Method, http.MethodGet)
			}
			w.Header().Set("Content-Type", string(types.DockerManifestSchema1))
			w.Write([]byte("doesn't matter"))
		default:
			t.Fatalf("Unexpected path: %v", r.URL.Path)
		}
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%v) = %v", server.URL, err)
	}

	tag := mustNewTag(t, fmt.Sprintf("%s/%s:latest", u.Host, expectedRepo))

	// Get should succeed even for invalid json. We don't parse the response.
	desc, err := Get(tag)
	if err != nil {
		t.Fatalf("Get(%s) = %v", tag, err)
	}

	// Should fail based on media type.
	if _, err := desc.Image(); err != ErrSchema1 {
		t.Errorf("Image() = %v, expected remote.ErrSchema1", err)
	}

	// Should fail based on media type.
	if _, err := desc.ImageIndex(); err != ErrSchema1 {
		t.Errorf("ImageIndex() = %v, expected remote.ErrSchema1", err)
	}
}

func TestGetImageAsIndex(t *testing.T) {
	expectedRepo := "foo/bar"
	manifestPath := fmt.Sprintf("/v2/%s/manifests/latest", expectedRepo)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.WriteHeader(http.StatusOK)
		case manifestPath:
			if r.Method != http.MethodGet {
				t.Errorf("Method; got %v, want %v", r.Method, http.MethodGet)
			}
			w.Header().Set("Content-Type", string(types.DockerManifestSchema2))
			w.Write([]byte("doesn't matter"))
		default:
			t.Fatalf("Unexpected path: %v", r.URL.Path)
		}
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%v) = %v", server.URL, err)
	}

	tag := mustNewTag(t, fmt.Sprintf("%s/%s:latest", u.Host, expectedRepo))

	// Get should succeed even for invalid json. We don't parse the response.
	desc, err := Get(tag)
	if err != nil {
		t.Fatalf("Get(%s) = %v", tag, err)
	}

	// Should fail based on media type.
	if _, err := desc.ImageIndex(); err == nil {
		t.Errorf("ImageIndex() = %v, expected err", err)
	}
}
