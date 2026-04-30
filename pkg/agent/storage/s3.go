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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type S3Provider struct {
	Client         S3ListClient
	TransferClient S3TransferClient
}

var log = logf.Log.WithName("modelAgent")

var _ Provider = (*S3Provider)(nil)

func (m *S3Provider) UploadObject(bucket string, key string, object []byte) error {
	ctx := context.Background()
	_, err := m.TransferClient.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(string(object)),
	})
	return err
}

func (m *S3Provider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	log.Info("Download model", "modelName", modelName, "storageUri", storageUri, "modelDir", modelDir)
	ctx := context.Background()

	s3Uri := strings.TrimPrefix(storageUri, string(S3))
	tokens := strings.SplitN(s3Uri, "/", 2)
	prefix := ""
	if len(tokens) == 2 {
		prefix = tokens[1]
	}
	bucket := tokens[0]

	resp, err := m.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return fmt.Errorf("unable to list objects: %w", err)
	}

	if len(resp.Contents) == 0 {
		return fmt.Errorf("%s has no objects or does not exist", storageUri)
	}

	foundObject := false

	for _, object := range resp.Contents {
		if strings.HasSuffix(*object.Key, "/") {
			continue
		}
		subObjectKey := strings.TrimPrefix(*object.Key, prefix)
		fileName := filepath.Join(modelDir, modelName, subObjectKey)

		if FileExists(fileName) {
			// File got corrupted or is mid-download :(
			// TODO: Figure out if we can maybe continue?
			if err := os.Remove(fileName); err != nil {
				return fmt.Errorf("file is unable to be deleted: %w", err)
			}
		}
		file, err := Create(fileName)
		if err != nil {
			return fmt.Errorf("file is already created: %w", err)
		}

		if _, err := m.TransferClient.DownloadObject(ctx, &transfermanager.DownloadObjectInput{
			Key:      object.Key,
			Bucket:   aws.String(bucket),
			WriterAt: file,
		}); err != nil {
			_ = file.Close()
			return fmt.Errorf("failed to download %s: %w", *object.Key, err)
		}
		if err := file.Close(); err != nil {
			log.Error(err, "failed to close file")
		}
		foundObject = true
	}

	if !foundObject {
		return fmt.Errorf("%s has no objects or does not exist", storageUri)
	}

	return nil
}

// S3ListClient abstracts the S3 ListObjectsV2 operation for dependency injection and testing.
type S3ListClient interface {
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// S3TransferClient abstracts the S3 transfer manager operations for download and upload.
type S3TransferClient interface {
	DownloadObject(ctx context.Context, input *transfermanager.DownloadObjectInput, opts ...func(*transfermanager.Options)) (*transfermanager.DownloadObjectOutput, error)
	UploadObject(ctx context.Context, input *transfermanager.UploadObjectInput, opts ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error)
}
