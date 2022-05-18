/*
Copyright 2022 The KServe Authors.

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
	"fmt"
	logger "log"
	"os"
	"path/filepath"
	"testing"

	gstorage "cloud.google.com/go/storage"
	"github.com/onsi/gomega"
)

func TestDownloadModel(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	storageURIs := []string{
		"gs://kfserving-examples/models/torchserve/image_classifier/v1",
		"gs://kfserving-examples/models/torchserve/image_classifier/no-object",
		"gs://bucket-not-exist/models/tensorflow/flowers",
	}
	modelName := "image-classifier"
	modelDir := t.TempDir()
	providers := map[Protocol]Provider{}
	provider, err := GetProvider(providers, GCS)
	if err != nil {
		logger.Fatal(err)
	}

	err = provider.DownloadModel(modelDir, modelName, storageURIs[0])
	g.Expect(err).To(gomega.BeNil())
	g.Expect(dirSize(modelDir)).ShouldNot(gomega.BeZero())

	// Test case for model already exist in modelDir
	err = provider.DownloadModel(modelDir, modelName, storageURIs[0])
	g.Expect(err).To(gomega.BeNil())
	g.Expect(dirSize(modelDir)).ShouldNot(gomega.BeZero())

	// Test case for no object exist
	err = provider.DownloadModel(modelDir, "test-1", storageURIs[1])
	g.Expect(err).Should(gomega.MatchError(fmt.Errorf("unable to download object/s because: %v", gstorage.ErrObjectNotExist)))

	// Test case for invalid bucket
	err = provider.DownloadModel(modelDir, "test-2", storageURIs[2])
	g.Expect(err).Should(gomega.MatchError(fmt.Errorf("unable to download object/s because: %s %v", "an error occurred while iterating:", gstorage.ErrBucketNotExist)))
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}
