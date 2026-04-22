/*
Copyright 2025 The KServe Authors.
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

package logger

import (
	"context"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"

	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

type MockStore struct {
	StorageSpec  *v1beta1.StorageSpec
	ResponseChan chan *LogRequest
}

func NewMockStore(storageSpec *v1beta1.StorageSpec) *MockStore {
	return &MockStore{
		StorageSpec:  storageSpec,
		ResponseChan: make(chan *LogRequest),
	}
}

func (m MockStore) Store(_ *url.URL, batch []LogRequest) error {
	if m.ResponseChan != nil {
		for i := range batch {
			m.ResponseChan <- &batch[i]
		}
	}
	return nil
}

func (m MockStore) GetStorageSpec() *v1beta1.StorageSpec {
	return m.StorageSpec
}

var _ Store = &MockStore{}

type MockS3Uploader struct {
	ReceivedUploadObjectsChan chan *transfermanager.UploadObjectInput
}

func (m *MockS3Uploader) DownloadObject(_ context.Context, _ *transfermanager.DownloadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.DownloadObjectOutput, error) {
	return &transfermanager.DownloadObjectOutput{}, nil
}

func (m *MockS3Uploader) UploadObject(_ context.Context, input *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
	go func() {
		m.ReceivedUploadObjectsChan <- input
	}()
	return &transfermanager.UploadObjectOutput{}, nil
}

var _ storage.S3TransferClient = &MockS3Uploader{}
