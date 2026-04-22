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

package mocks

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type MockS3Client struct{}

func (m *MockS3Client) ListObjectsV2(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	return &s3.ListObjectsV2Output{
		Contents: []s3types.Object{
			{
				Key: aws.String("model.pt"),
			},
		},
	}, nil
}

type MockS3TransferClient struct{}

func (m *MockS3TransferClient) DownloadObject(_ context.Context, _ *transfermanager.DownloadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.DownloadObjectOutput, error) {
	return &transfermanager.DownloadObjectOutput{}, nil
}

func (m *MockS3TransferClient) UploadObject(_ context.Context, _ *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
	return &transfermanager.UploadObjectOutput{}, nil
}

type MockS3FailTransferClient struct {
	Err error
}

func (m *MockS3FailTransferClient) DownloadObject(_ context.Context, _ *transfermanager.DownloadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.DownloadObjectOutput, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return nil, errors.New("failed to download")
}

func (m *MockS3FailTransferClient) UploadObject(_ context.Context, _ *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
	return &transfermanager.UploadObjectOutput{}, nil
}
