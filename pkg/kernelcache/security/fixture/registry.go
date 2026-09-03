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

package fixture

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

// StartRegistry starts an in-memory OCI registry and returns its host:port.
// The server is closed when the test ends.
func StartRegistry(tb testing.TB) string {
	tb.Helper()
	srv := httptest.NewServer(ggcrregistry.New())
	tb.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(tb, err)
	return u.Host
}

// PushImage pushes a small random image and returns its reference and digest.
func PushImage(tb testing.TB, host, repo string) (name.Reference, string) {
	tb.Helper()
	ref, err := name.ParseReference(host + "/" + repo + ":latest")
	require.NoError(tb, err)
	img, err := random.Image(256, 1)
	require.NoError(tb, err)
	require.NoError(tb, remote.Write(ref, img))
	d, err := img.Digest()
	require.NoError(tb, err)
	return ref, d.String()
}
