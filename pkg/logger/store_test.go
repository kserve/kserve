/*
Copyright 2023 The KServe Authors.
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
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/onsi/gomega"
	pkglogging "knative.dev/pkg/logging"

	"github.com/kserve/kserve/pkg/agent/storage"
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
	g.Expect(err).ToNot(gomega.HaveOccurred())

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
	g.Expect(err).ToNot(gomega.HaveOccurred())

	err = store.Store(logUrl, LogRequest{
		Id:               "0123",
		Namespace:        "ns",
		InferenceService: "inference",
		Component:        "predictor",
		ReqType:          CEInferenceRequest,
	})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	req := <-uploader.ReceivedUploadObjectsChan
	g.Expect(*req.Object.Bucket).To(gomega.Equal("bucket"))
	g.Expect(*req.Object.Key).To(gomega.MatchRegexp("prefix/ns/inference/predictor/logger/0123-request.json"))
}
