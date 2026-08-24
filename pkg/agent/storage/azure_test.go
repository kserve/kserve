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
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

type azureTraversalClient struct {
	blobName string
}

func (c *azureTraversalClient) NewListBlobsFlatPager(bucket string, _ *azblob.ListBlobsFlatOptions) *runtime.Pager[azblob.ListBlobsFlatResponse] {
	return runtime.NewPager(runtime.PagingHandler[azblob.ListBlobsFlatResponse]{
		More: func(azblob.ListBlobsFlatResponse) bool { return false },
		Fetcher: func(context.Context, *azblob.ListBlobsFlatResponse) (azblob.ListBlobsFlatResponse, error) {
			return azblob.ListBlobsFlatResponse{
				ListBlobsFlatSegmentResponse: container.ListBlobsFlatSegmentResponse{
					ContainerName: &bucket,
					Segment: &container.BlobFlatListSegment{
						BlobItems: []*container.BlobItem{{Name: &c.blobName}},
					},
				},
			}, nil
		},
	})
}

func (c *azureTraversalClient) DownloadFile(context.Context, string, string, *os.File, *azblob.DownloadFileOptions) (int64, error) {
	return 0, nil
}

func (c *azureTraversalClient) UploadBuffer(context.Context, string, string, []byte, *azblob.UploadBufferOptions) (azblob.UploadBufferResponse, error) {
	return azblob.UploadBufferResponse{}, nil
}

func TestAzureDownloadModelRejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	outsidePath := filepath.Join(tmpDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("do not overwrite"), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := AzureProvider{Client: &azureTraversalClient{blobName: "prefix/../../../outside.txt"}}
	err := provider.DownloadModel(filepath.Join(tmpDir, "models"), "model1", "https://container/prefix/")
	if err == nil {
		t.Fatal("expected path traversal blob to be rejected")
	}
	got, readErr := os.ReadFile(outsidePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "do not overwrite" {
		t.Fatalf("outside file contents = %q, want %q", string(got), "do not overwrite")
	}
}
