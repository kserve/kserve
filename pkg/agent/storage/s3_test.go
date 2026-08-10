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
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/onsi/gomega"
)

// fakeS3ListClient returns a fixed set of keys regardless of the request,
// allowing tests to simulate a bucket containing sibling prefixes.
type fakeS3ListClient struct {
	s3iface.S3API
	keys []string
}

func (f *fakeS3ListClient) ListObjects(*s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
	contents := make([]*s3.Object, 0, len(f.keys))
	for _, k := range f.keys {
		contents = append(contents, &s3.Object{Key: aws.String(k)})
	}
	return &s3.ListObjectsOutput{Contents: contents}, nil
}

func TestS3ObjectDownloader_GetAllObjects_SiblingPrefixCollision(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tmpDir := t.TempDir()
	client := &fakeS3ListClient{
		keys: []string{
			"merlinite-7b-lab/config.json",
			"merlinite-7b-lab-hf/config.json",
		},
	}

	downloader := &S3ObjectDownloader{
		StorageUri: "s3://modelRepo/merlinite-7b-lab",
		ModelDir:   tmpDir,
		ModelName:  "model",
		Bucket:     "modelRepo",
		Prefix:     "merlinite-7b-lab",
	}

	objects, err := downloader.GetAllObjects(client)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(objects).To(gomega.HaveLen(1))
	g.Expect(*objects[0].Object.Key).To(gomega.Equal("merlinite-7b-lab/config.json"))
}

func TestS3ObjectDownloader_GetAllObjects_SingleObjectKeyMatch(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tmpDir := t.TempDir()
	client := &fakeS3ListClient{
		keys: []string{"model.pt"},
	}

	downloader := &S3ObjectDownloader{
		StorageUri: "s3://modelRepo/model.pt",
		ModelDir:   tmpDir,
		ModelName:  "model",
		Bucket:     "modelRepo",
		Prefix:     "model.pt",
	}

	objects, err := downloader.GetAllObjects(client)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(objects).To(gomega.HaveLen(1))
	g.Expect(*objects[0].Object.Key).To(gomega.Equal("model.pt"))
}

func TestS3ObjectDownloader_GetAllObjects_EmptyPrefixMatchesEverything(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tmpDir := t.TempDir()
	client := &fakeS3ListClient{
		keys: []string{"a/model.pt", "b/model.pt"},
	}

	downloader := &S3ObjectDownloader{
		StorageUri: "s3://modelRepo",
		ModelDir:   tmpDir,
		ModelName:  "model",
		Bucket:     "modelRepo",
		Prefix:     "",
	}

	objects, err := downloader.GetAllObjects(client)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(objects).To(gomega.HaveLen(2))
}
