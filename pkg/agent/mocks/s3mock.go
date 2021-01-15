package mocks

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/golang/protobuf/proto"
)

type MockS3Client struct {
	s3iface.S3API
}

func (m *MockS3Client) ListObjects(*s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
	return &s3.ListObjectsOutput{
		Contents: []*s3.Object{
			{
				Key: proto.String("model.pt"),
			},
		},
	}, nil
}

type MockS3Downloader struct {
}

func (m *MockS3Downloader) DownloadWithIterator(aws.Context, s3manager.BatchDownloadIterator, ...func(*s3manager.Downloader)) error {
	return nil
}

type MockS3FailDownloader struct {
	Err error
}

func (m *MockS3FailDownloader) DownloadWithIterator(aws.Context, s3manager.BatchDownloadIterator, ...func(*s3manager.Downloader)) error {
	var errs []s3manager.Error
	errs = append(errs, s3manager.Error{
		OrigErr: fmt.Errorf("failed to download"),
		Bucket:  aws.String("modelRepo"),
		Key:     aws.String("model1/model.pt"),
	})
	return s3manager.NewBatchError("BatchedDownloadIncomplete", "some objects have failed to download.", errs)
}
