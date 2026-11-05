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

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/kserve/kserve/pkg/agent/mocks"
)

func TestDownloadModel_SinglePage(t *testing.T) {
	syscall.Umask(0)
	modelDir := t.TempDir()

	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"models/model1/model.pt", "models/model1/config.json"},
			},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(modelDir, "model1", "s3://test-bucket/models/model1/")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	// Verify both files were created
	for _, name := range []string{"model.pt", "config.json"} {
		path := filepath.Join(modelDir, "model1", name)
		if !FileExists(path) {
			t.Errorf("expected file %s to exist", path)
		}
	}
}

func TestDownloadModel_MultiplePages(t *testing.T) {
	syscall.Umask(0)
	modelDir := t.TempDir()

	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"models/model1/shard-0001.bin", "models/model1/shard-0002.bin"},
				{"models/model1/shard-0003.bin", "models/model1/config.json"},
			},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(modelDir, "model1", "s3://test-bucket/models/model1/")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	expected := []string{"shard-0001.bin", "shard-0002.bin", "shard-0003.bin", "config.json"}
	for _, name := range expected {
		path := filepath.Join(modelDir, "model1", name)
		if !FileExists(path) {
			t.Errorf("expected file %s to exist (pagination should have fetched all pages)", path)
		}
	}
}

func TestDownloadModel_ThreePages(t *testing.T) {
	syscall.Umask(0)
	modelDir := t.TempDir()

	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"prefix/a.bin"},
				{"prefix/b.bin"},
				{"prefix/c.bin"},
			},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(modelDir, "mymodel", "s3://bucket/prefix/")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	for _, name := range []string{"a.bin", "b.bin", "c.bin"} {
		path := filepath.Join(modelDir, "mymodel", name)
		if !FileExists(path) {
			t.Errorf("expected file %s to exist", path)
		}
	}
}

func TestDownloadModel_SkipsDirectoryKeys(t *testing.T) {
	syscall.Umask(0)
	modelDir := t.TempDir()

	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"models/model1/", "models/model1/model.pt"},
			},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(modelDir, "model1", "s3://test-bucket/models/model1/")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	path := filepath.Join(modelDir, "model1", "model.pt")
	if !FileExists(path) {
		t.Error("expected model.pt to exist")
	}
}

func TestDownloadModel_EmptyPrefix(t *testing.T) {
	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(t.TempDir(), "model1", "s3://empty-bucket/nonexistent/")
	if err == nil {
		t.Fatal("expected error for empty prefix, got nil")
	}
}

func TestDownloadModel_OnlyDirectoryKeys(t *testing.T) {
	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"models/model1/"},
			},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(t.TempDir(), "model1", "s3://bucket/models/model1/")
	if err == nil {
		t.Fatal("expected error when all keys are directories, got nil")
	}
}

func TestDownloadModel_ListError(t *testing.T) {
	provider := &S3Provider{
		Client:         &mocks.MockS3FailClient{Err: errors.New("access denied")},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(t.TempDir(), "model1", "s3://bucket/prefix/")
	if err == nil {
		t.Fatal("expected error from ListObjectsV2, got nil")
	}
	if got := err.Error(); got != "unable to list objects: access denied" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestDownloadModel_TransferError(t *testing.T) {
	syscall.Umask(0)

	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"prefix/model.pt"},
			},
		},
		TransferClient: &mocks.MockS3FailTransferClient{Err: errors.New("network timeout")},
	}

	err := provider.DownloadModel(t.TempDir(), "model1", "s3://bucket/prefix/")
	if err == nil {
		t.Fatal("expected error from download, got nil")
	}
}

func TestDownloadModel_OverwritesExistingFile(t *testing.T) {
	syscall.Umask(0)
	modelDir := t.TempDir()

	// Pre-create the file to simulate a corrupted/partial download
	dir := filepath.Join(modelDir, "model1")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "model.pt")
	if err := os.WriteFile(existing, []byte("corrupted"), 0o640); err != nil {
		t.Fatal(err)
	}

	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"prefix/model.pt"},
			},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(modelDir, "model1", "s3://bucket/prefix/")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	if !FileExists(existing) {
		t.Error("expected file to exist after re-download")
	}
}

func TestDownloadModel_NoPrefixInURI(t *testing.T) {
	syscall.Umask(0)
	modelDir := t.TempDir()

	provider := &S3Provider{
		Client: &mocks.MockS3PaginatedClient{
			Pages: [][]string{
				{"model.pt"},
			},
		},
		TransferClient: &mocks.MockS3TransferClient{},
	}

	err := provider.DownloadModel(modelDir, "model1", "s3://bucket-only")
	if err != nil {
		t.Fatalf("DownloadModel failed: %v", err)
	}

	path := filepath.Join(modelDir, "model1", "model.pt")
	if !FileExists(path) {
		t.Error("expected model.pt to exist")
	}
}
