package logger

import (
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/onsi/gomega"
	pkglogging "knative.dev/pkg/logging"
	"net/url"
	"testing"
)

func mockStore() (*S3Store, *MockS3Uploader) {
	uploader := &MockS3Uploader{
		ReceivedUploadObjectsChan: make(chan s3manager.BatchUploadObject),
	}

	log, _ := pkglogging.NewLogger("", "INFO")
	store := NewS3Store("/logger", "json", &JSONMarshaller{}, &storage.S3Provider{Uploader: uploader}, log)
	return store, uploader
}

func TestNilUrl(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	store, _ := mockStore()
	err := store.Store(nil, LogRequest{})
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.MatchRegexp("url|URL"))
}

func TestMissingBucket(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	store, _ := mockStore()

	logUrl, err := url.Parse("s3://")
	g.Expect(err).To(gomega.BeNil())

	err = store.Store(logUrl, LogRequest{
		ReqType: CEInferenceRequest,
	})
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.MatchRegexp("[b|B]ucket"))
}

func TestConfiguredPrefix(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	store, uploader := mockStore()

	logUrl, err := url.Parse("s3://bucket/prefix")
	g.Expect(err).To(gomega.BeNil())

	err = store.Store(logUrl, LogRequest{
		Id:               "0123",
		Namespace:        "ns",
		InferenceService: "inference",
		Component:        "predictor",
		ReqType:          CEInferenceRequest,
	})
	g.Expect(err).To(gomega.BeNil())

	req := <-uploader.ReceivedUploadObjectsChan
	g.Expect(*req.Object.Bucket).To(gomega.Equal("bucket"))
	g.Expect(*req.Object.Key).To(gomega.MatchRegexp("prefix/ns/inference/predictor/0123-request"))
}
