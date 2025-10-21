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
	"encoding/json"
	"io"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/kserve/kserve/pkg/logger/marshaller"
	"github.com/kserve/kserve/pkg/logger/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	pkglogging "knative.dev/pkg/logging"

	"github.com/kserve/kserve/pkg/agent/storage"
)

func mockStore(batchSize int) (*BlobStore, *MockS3Uploader) {
	uploader := &MockS3Uploader{
		ReceivedUploadObjectsChan: make(chan s3manager.BatchUploadObject),
	}

	log, _ := pkglogging.NewLogger("", "INFO")
	store := NewBlobStore("/logger", "json", &marshaller.JSONMarshaller{}, &storage.S3Provider{Uploader: uploader}, batchSize, log)
	return store, uploader
}

func TestNilUrl(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	store, _ := mockStore(DefaultBatchSize)
	err := store.Store(nil, types.LogRequest{})
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.MatchRegexp("url|URL"))
}

func TestMissingBucket(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	store, _ := mockStore(DefaultBatchSize)

	logUrl, err := url.Parse("s3://")
	g.Expect(err).ToNot(gomega.HaveOccurred())

	err = store.Store(logUrl, types.LogRequest{
		ReqType: CEInferenceRequest,
	})
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.MatchRegexp("[b|B]ucket"))
}

func TestConfiguredPrefix(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	store, uploader := mockStore(DefaultBatchSize)

	logUrl, err := url.Parse("s3://bucket/prefix")
	g.Expect(err).ToNot(gomega.HaveOccurred())

	err = store.Store(logUrl, types.LogRequest{
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

func TestBatchSize(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	store, uploader := mockStore(2)

	logUrl, err := url.Parse("s3://bucket/prefix")
	g.Expect(err).ToNot(gomega.HaveOccurred())

	err = store.Store(logUrl, types.LogRequest{
		Id:               "0123",
		Namespace:        "ns",
		InferenceService: "inference",
		Component:        "predictor",
		ReqType:          CEInferenceRequest,
	})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	err = store.Store(logUrl, types.LogRequest{
		Id:               "1234",
		Namespace:        "ns",
		InferenceService: "inference",
		Component:        "predictor",
		ReqType:          CEInferenceRequest,
	})
	g.Expect(err).ToNot(gomega.HaveOccurred())

	req := <-uploader.ReceivedUploadObjectsChan
	g.Expect(*req.Object.Bucket).To(gomega.Equal("bucket"))
	g.Expect(*req.Object.Key).To(gomega.MatchRegexp("prefix/ns/inference/predictor/logger/1234-request.json"))
	reader := req.Object.Body
	reqBytes, err := io.ReadAll(reader)
	if err != nil {
		assert.Fail(t, err.Error())
	}
	assert.Greater(t, len(reqBytes), 0, "failed to read bytes")
	result := make([]types.LogRequest, 0)
	err = json.Unmarshal(reqBytes, &result)
	if err != nil {
		assert.Fail(t, err.Error())
	}
	g.Expect(len(result)).To(gomega.Equal(2))
}
