/*
Copyright 2021 The KServe Authors.

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
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type AzureClient interface {
	NewListBlobsFlatPager(bucket string, options *azblob.ListBlobsFlatOptions) *runtime.Pager[azblob.ListBlobsFlatResponse]
	DownloadFile(ctx context.Context, bucket string, prefix string, file *os.File, options *azblob.DownloadFileOptions) (int64, error)
	UploadBuffer(ctx context.Context, bucket string, key string, object []byte, o *azblob.UploadBufferOptions) (azblob.UploadBufferResponse, error)
}

type AzureProvider struct {
	Client AzureClient
}

var _ Provider = (*AzureProvider)(nil)

func (a AzureProvider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	log.Info("Download model ", "modelName", modelName, "storageUri", storageUri, "modelDir", modelDir)
	uri := strings.TrimPrefix(storageUri, string(HTTPS))
	tokens := strings.SplitN(uri, "/", 2)
	prefix := ""
	if len(tokens) == 2 {
		prefix = tokens[1]
	}
	ctx := context.Background()
	bucket := tokens[0]
	pager := a.Client.NewListBlobsFlatPager(bucket, &azblob.ListBlobsFlatOptions{
		Prefix: &prefix,
	})

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, _blob := range resp.Segment.BlobItems {
			fileName := filepath.Join(modelDir, modelName, *_blob.Name)
			log.Info("Downloading blob %s", fileName)
			file, err := os.Create(fileName)
			if err != nil {
				return err
			}
			defer func(destFile *os.File) {
				err = destFile.Close()
				log.Error(err, "Error closing file")
			}(file)

			_, err = a.Client.DownloadFile(ctx, bucket, prefix, file,
				&azblob.DownloadFileOptions{
					// If Progress is non-nil, this function is called periodically as bytes are uploaded.
					Progress: func(bytesTransferred int64) {
						log.Info("Downloaded %d bytes of object %s", bytesTransferred, prefix)
					},
				})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (a AzureProvider) UploadObject(bucket string, key string, object []byte) error {
	log.Info("Upload object ", "bucket", bucket, "key", key, "length", len(object))

	_, err := a.Client.UploadBuffer(context.Background(), bucket, key, object, nil)
	if err != nil {
		return err
	}
	log.Info("Wrote object to bucket ", "bucket", bucket, "key", key, "length", len(object))
	return nil
}
