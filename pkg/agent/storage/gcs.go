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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gstorage "cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
	"google.golang.org/api/iterator"
)

type GCSProvider struct {
	Client stiface.Client
}

func (p *GCSProvider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	log.Info("Downloading model ", "modelName", modelName, "storageUri", storageUri, "modelDir", modelDir)
	gcsUri := strings.TrimPrefix(storageUri, string(GCS))
	tokens := strings.SplitN(gcsUri, "/", 2)
	prefix := ""
	if len(tokens) == 2 {
		prefix = tokens[1]
	}
	ctx := context.Background()
	gcsObjectDownloader := &GCSObjectDownloader{
		StorageUri: storageUri,
		ModelDir:   modelDir,
		ModelName:  modelName,
		Bucket:     tokens[0],
		Item:       prefix,
	}
	it, err := gcsObjectDownloader.GetObjectIterator(ctx, p.Client)
	if err != nil {
		return fmt.Errorf("unable to get object iterator because: %w", err)
	}
	if err := gcsObjectDownloader.Download(ctx, p.Client, it); err != nil {
		return fmt.Errorf("unable to download object/s because: %w", err)
	}
	return nil
}

type GCSObjectDownloader struct {
	StorageUri string
	ModelDir   string
	ModelName  string
	Bucket     string
	Item       string
}

func (g *GCSObjectDownloader) GetObjectIterator(ctx context.Context, client stiface.Client) (stiface.ObjectIterator, error) {
	query := &gstorage.Query{Prefix: g.Item}
	return client.Bucket(g.Bucket).Objects(ctx, query), nil
}

func (g *GCSObjectDownloader) Download(ctx context.Context, client stiface.Client, it stiface.ObjectIterator) error {
	var errs []error
	// flag to help determine if query prefix returned an empty iterator
	foundObject := false

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("an error occurred while iterating: %w", err)
		}
		objectValue := strings.TrimPrefix(attrs.Name, g.Item)
		fileName := filepath.Join(g.ModelDir, g.ModelName, objectValue)

		foundObject = true
		if FileExists(fileName) {
			log.Info("Deleting file", "name", fileName)
			if err := os.Remove(fileName); err != nil {
				return fmt.Errorf("file is unable to be deleted: %w", err)
			}
		}
		file, err := Create(fileName)
		if err != nil {
			return fmt.Errorf("file is already created: %w", err)
		}
		if err := g.DownloadFile(ctx, client, attrs, file); err != nil {
			errs = append(errs, err)
		}
	}
	if !foundObject {
		return gstorage.ErrObjectNotExist
	}
	if len(errs) > 0 {
		return awserr.NewBatchError("GCSDownloadIncomplete", "some objects failed to download.", errs)
	}
	return nil
}

func (g *GCSObjectDownloader) DownloadFile(ctx context.Context, client stiface.Client, attrs *gstorage.ObjectAttrs, file *os.File) error {
	reader, err := client.Bucket(attrs.Bucket).Object(attrs.Name).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create reader for object(%s) in bucket(%s): %w",
			attrs.Name,
			attrs.Bucket,
			err,
		)
	}
	defer func(reader stiface.Reader) {
		closeErr := reader.Close()
		if closeErr != nil {
			log.Error(closeErr, "failed to close reader")
		}
	}(reader)
	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf(
			"failed to copy object(%s) from bucket(%s) to file: %w",
			attrs.Name,
			attrs.Bucket,
			err,
		)
	}
	log.Info("Wrote " + attrs.Prefix + " to file " + file.Name())
	return nil
}
