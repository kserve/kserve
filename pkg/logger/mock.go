package logger

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/s3/s3manager/s3manageriface"
	"net/url"
)

type MockStore struct {
	StoreConfig  *StoreConfig
	ResponseChan chan *LogRequest
}

func NewMockStore(storeConfig *StoreConfig) *MockStore {
	return &MockStore{
		StoreConfig:  storeConfig,
		ResponseChan: make(chan *LogRequest),
	}
}

func (m MockStore) Store(_ *url.URL, logRequest LogRequest) error {
	if m.ResponseChan != nil {
		m.ResponseChan <- &logRequest
	}
	return nil
}

func (m MockStore) GetConfig() *StoreConfig {
	return m.StoreConfig
}

var _ Store = &MockStore{}

type MockS3Uploader struct {
	ReceivedUploadObjectsChan chan s3manager.BatchUploadObject
}

func (m *MockS3Uploader) UploadWithIterator(_ aws.Context, iterator s3manager.BatchUploadIterator, _ ...func(*s3manager.Uploader)) error {
	go func() {
		for iterator.Next() {
			obj := iterator.UploadObject()
			m.ReceivedUploadObjectsChan <- obj
		}
	}()
	return nil
}

var _ s3manageriface.UploadWithIterator = &MockS3Uploader{}
