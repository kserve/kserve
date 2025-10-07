package mocks

import (
	"context"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/kserve/kserve/pkg/agent/storage"
)

type mockAzureObject struct {
	blobItem *container.BlobItem
	buffer   []byte
}

type mockAzureBucket struct {
	objects map[string]*mockAzureObject
}

type mockAzureClient struct {
	buckets map[string]*mockAzureBucket
}

func (m mockAzureClient) NewListBlobsFlatPager(bucket string, options *azblob.ListBlobsFlatOptions) *runtime.Pager[azblob.ListBlobsFlatResponse] {
	pager := runtime.NewPager[azblob.ListBlobsFlatResponse](
		runtime.PagingHandler[azblob.ListBlobsFlatResponse]{
			More: func(page azblob.ListBlobsFlatResponse) bool {
				return false
			},
			Fetcher: func(context.Context, *azblob.ListBlobsFlatResponse) (azblob.ListBlobsFlatResponse, error) {
				containerBucket := m.buckets[bucket]
				blobItems := make([]*container.BlobItem, 0)
				for _, obj := range containerBucket.objects {
					blobItems = append(blobItems, obj.blobItem)
				}
				return azblob.ListBlobsFlatResponse{
					ListBlobsFlatSegmentResponse: container.ListBlobsFlatSegmentResponse{
						ContainerName: &bucket,
						Segment: &container.BlobFlatListSegment{
							BlobItems: blobItems,
						},
					},
				}, nil
			},
		},
	)
	return pager
}

func (m mockAzureClient) DownloadFile(ctx context.Context, bucket string, prefix string, file *os.File, options *azblob.DownloadFileOptions) (int64, error) {
	containerBucket := m.buckets[bucket]
	for key, obj := range containerBucket.objects {
		if key == prefix {
			file.Write(obj.buffer)
			return int64(len(obj.buffer)), nil
		}
	}
	return 0, nil
}

func (m mockAzureClient) UploadBuffer(ctx context.Context, bucket string, key string, object []byte, o *azblob.UploadBufferOptions) (azblob.UploadBufferResponse, error) {
	m.buckets[bucket].objects[key] = &mockAzureObject{
		blobItem: &container.BlobItem{
			Name: &key,
		},
		buffer: object,
	}
	return azblob.UploadBufferResponse{}, nil
}

var _ storage.AzureClient = &mockAzureClient{}

func NewMockAzureClient() storage.AzureClient {
	return &mockAzureClient{buckets: map[string]*mockAzureBucket{}}
}
