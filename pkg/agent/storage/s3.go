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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/s3/s3manager/s3manageriface"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type S3Provider struct {
	Client     s3iface.S3API
	Downloader s3manageriface.DownloadWithIterator
}

var log = logf.Log.WithName("modelAgent")

var _ Provider = (*S3Provider)(nil)

type S3ObjectDownloader struct {
	StorageUri string
	ModelDir   string
	ModelName  string
	Bucket     string
	Prefix     string
	downloader s3manageriface.DownloadWithIterator
}

func (m *S3Provider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	log.Info("Download model ", "modelName", modelName, "storageUri", storageUri, "modelDir", modelDir)
	s3Uri := strings.TrimPrefix(storageUri, string(S3))
	tokens := strings.SplitN(s3Uri, "/", 2)
	prefix := ""
	if len(tokens) == 2 {
		prefix = tokens[1]
	}
	s3ObjectDownloader := &S3ObjectDownloader{
		StorageUri: storageUri,
		ModelDir:   modelDir,
		ModelName:  modelName,
		Bucket:     tokens[0],
		Prefix:     prefix,
		downloader: m.Downloader,
	}
	objects, err := s3ObjectDownloader.GetAllObjects(m.Client)
	if err != nil {
		return fmt.Errorf("unable to get batch objects %w", err)
	}
	if err := s3ObjectDownloader.Download(objects); err != nil {
		return err
	}
	return nil
}

func (s *S3ObjectDownloader) GetAllObjects(s3Svc s3iface.S3API) ([]s3manager.BatchDownloadObject, error) {
	resp, err := s3Svc.ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(s.Bucket),
		Prefix: aws.String(s.Prefix),
	})
	if err != nil {
		return nil, err
	}
	results := make([]s3manager.BatchDownloadObject, 0)

	if len(resp.Contents) == 0 {
		return nil, fmt.Errorf("%s has no objects or does not exist", s.StorageUri)
	}

	foundObject := false

	for _, object := range resp.Contents {
		if strings.HasSuffix(*object.Key, "/") {
			continue
		}
		subObjectKey := strings.TrimPrefix(*object.Key, s.Prefix)
		fileName := filepath.Join(s.ModelDir, s.ModelName, subObjectKey)

		if FileExists(fileName) {
			// File got corrupted or is mid-download :(
			// TODO: Figure out if we can maybe continue?
			if err := os.Remove(fileName); err != nil {
				return nil, fmt.Errorf("file is unable to be deleted: %w", err)
			}
		}
		file, err := Create(fileName)
		if err != nil {
			return nil, fmt.Errorf("file is already created: %w", err)
		}
		object := s3manager.BatchDownloadObject{
			Object: &s3.GetObjectInput{
				Key:    aws.String(*object.Key),
				Bucket: aws.String(s.Bucket),
			},
			Writer: file,
			After: func() error {
				defer func(file *os.File) {
					closeErr := file.Close()
					if closeErr != nil {
						log.Error(closeErr, "failed to close file")
					}
				}(file)
				return nil
			},
		}
		foundObject = true
		results = append(results, object)
	}

	if !foundObject {
		return nil, fmt.Errorf("%s has no objects or does not exist", s.StorageUri)
	}

	return results, nil
}

func (s *S3ObjectDownloader) Download(objects []s3manager.BatchDownloadObject) error {
	iter := &s3manager.DownloadObjectsIterator{Objects: objects}
	if err := s.downloader.DownloadWithIterator(aws.BackgroundContext(), iter); err != nil {
		return err
	}
	return nil
}
