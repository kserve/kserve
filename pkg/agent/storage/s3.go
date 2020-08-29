/*
Copyright 2020 kubeflow.org.

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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/s3/s3manager/s3manageriface"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type S3Provider struct {
	Client     s3iface.S3API
	Downloader s3manageriface.DownloaderAPI
}

var _ Provider = (*S3Provider)(nil)

type S3ObjectDownloader struct {
	StorageUri string
	ModelDir   string
	ModelName  string
	Bucket     string
	Prefix     string
	downloader s3manageriface.DownloaderAPI
}

func (m *S3Provider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	s3Uri := strings.TrimPrefix(storageUri, string(S3))
	path := strings.Split(s3Uri, "/")
	s3ObjectDownloader := &S3ObjectDownloader{
		StorageUri: storageUri,
		ModelDir:   modelDir,
		ModelName:  modelName,
		Bucket:     path[0],
		Prefix:     path[1],
		downloader: m.Downloader,
	}
	objects, err := s3ObjectDownloader.GetAllObjects(m.Client)
	if err != nil {
		return fmt.Errorf("unable to get batch objects %v", err)
	}
	if err := s3ObjectDownloader.Download(objects); err != nil {
		return fmt.Errorf("unable to get download objects %v", err)
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

	for _, object := range resp.Contents {
		fileName := filepath.Join(s.ModelDir, s.ModelName, *object.Key)
		if FileExists(fileName) {
			// File got corrupted or is mid-download :(
			// TODO: Figure out if we can maybe continue?
			log.Println("Deleting", fileName)
			if err := os.Remove(fileName); err != nil {
				return nil, fmt.Errorf("file is unable to be deleted: %v", err)
			}
		}
		file, err := Create(fileName)
		if err != nil {
			return nil, fmt.Errorf("file is already created: %v", err)
		}
		object := s3manager.BatchDownloadObject{
			Object: &s3.GetObjectInput{
				Key:    aws.String(*object.Key),
				Bucket: aws.String(s.Bucket),
			},
			Writer: file,
			After: func() error {
				defer file.Close()
				log.Printf("Downloaded %v\n", aws.String(*object.Key))
				return nil
			},
		}
		results = append(results, object)
	}
	return results, nil
}

func (s *S3ObjectDownloader) Download(objects []s3manager.BatchDownloadObject) error {
	iter := &s3manager.DownloadObjectsIterator{Objects: objects}
	var errs []s3manager.Error
	for iter.Next() {
		object := iter.DownloadObject()
		if _, err := s.downloader.DownloadWithContext(aws.BackgroundContext(), object.Writer, object.Object); err != nil {
			errs = append(errs, s3manager.Error{
				OrigErr: err,
				Bucket:  object.Object.Bucket,
				Key:     object.Object.Key,
			})
		}

		if object.After == nil {
			continue
		}

		if err := object.After(); err != nil {
			errs = append(errs, s3manager.Error{
				OrigErr: err,
				Bucket:  object.Object.Bucket,
				Key:     object.Object.Key,
			})
		}
	}

	if len(errs) > 0 {
		return s3manager.NewBatchError("BatchedDownloadIncomplete", "some objects have failed to download.", errs)
	}
	return nil
}
