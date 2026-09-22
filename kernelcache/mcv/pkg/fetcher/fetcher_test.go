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

package fetcher

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
)

// --- Mocks ---
type mockDockerClient struct {
	shouldFail bool
}

func (m *mockDockerClient) ImageSave(ctx context.Context, imgs []string, options ...client.ImageSaveOption) (io.ReadCloser, error) {
	if m.shouldFail {
		return nil, errors.New("mock docker failure")
	}
	return io.NopCloser(bytes.NewReader([]byte("fake image data"))), nil
}

func (m *mockDockerClient) Close() error {
	return nil
}

type mockPodmanClient struct {
	exists    bool
	exportErr error
}

func (m *mockPodmanClient) Exists(ctx context.Context, name string, opts *images.ExistsOptions) (bool, error) {
	return m.exists, nil
}

func (m *mockPodmanClient) Export(ctx context.Context, names []string, w io.Writer, opts *images.ExportOptions) error {
	if m.exportErr != nil {
		return m.exportErr
	}
	_, _ = w.Write([]byte("mock image"))
	return nil
}

// --- Tests ---
// func TestDockerFetcher_Success(t *testing.T) {
// 	df := &dockerFetcher{
// 		client: &mockDockerClient{},
// 	}

// 	img, err := df.FetchImg("quay.io/gkm/vector-add-cache:rocm")
// 	assert.NoError(t, err)
// 	assert.NotNil(t, img)
// }

func TestDockerFetcher_Failure(t *testing.T) {
	df := &dockerFetcher{
		client: &mockDockerClient{shouldFail: true},
	}

	img, err := df.FetchImg("mock/image:tag")
	assert.Error(t, err)
	assert.Nil(t, img)
}

// func TestPodmanFetcher_Success(t *testing.T) {
// 	pf := &podmanFetcher{
// 		client: &mockPodmanClient{exists: true},
// 	}

// 	img, err := pf.FetchImg("quay.io/gkm/vector-add-cache:rocm")
// 	assert.NoError(t, err)
// 	assert.NotNil(t, img)
// }

func TestPodmanFetcher_ImageNotFound(t *testing.T) {
	pf := &podmanFetcher{
		client: &mockPodmanClient{exists: false},
	}

	img, err := pf.FetchImg("mock/image:tag")
	assert.Error(t, err)
	assert.Nil(t, img)
}

func TestRemoteFetcher(t *testing.T) {
	rf := &remoteFetcher{}

	_, err := rf.FetchImg("quay.io/gkm/vector-add-cache:rocm")
	if err != nil {
		t.Log("Expected error in offline test:", err)
	}
}

func TestFallbackToRemote(t *testing.T) {
	f := NewFetcher() // This will use actual local clients if available

	img, err := f.FetchImg("quay.io/gkm/vector-add-cache:rocm")
	if err != nil {
		t.Log("Expected error if offline:", err)
	} else {
		assert.NotNil(t, img)
	}
}

func TestLoadImageFromTarball(t *testing.T) {
	_, err := loadImageFromTarball("/tmp/nonexistent.tar")
	assert.Error(t, err)
}
