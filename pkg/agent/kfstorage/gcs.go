package kfstorage

import (
	"cloud.google.com/go/storage"
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/kubeflow/kfserving/pkg/agent/test/mockapi"
	"google.golang.org/api/iterator"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type GCSProvider struct {
	Client mockapi.Client
}

func (p *GCSProvider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	gcsUri := strings.TrimPrefix(storageUri, string(GCS))
	path := strings.Split(gcsUri, "/")
	ctx := context.Background()
	gcsObjectDownloader := &GCSObjectDownloader {
		Context: ctx,
		StorageUri: storageUri,
		ModelDir: modelDir,
		ModelName: modelName,
		Bucket: path[0],
		Item: path[1],
	}
	it := gcsObjectDownloader.GetObjectIterator(p.Client)
	if err := gcsObjectDownloader.Download(p.Client, it); err != nil {
		return fmt.Errorf("unable to download objects %v", err)
	}
	return nil
}

type GCSObjectDownloader struct {
	Context    context.Context
	StorageUri string
	ModelDir   string
	ModelName  string
	Bucket     string
	Item       string
}

func (g *GCSObjectDownloader) GetObjectIterator(client mockapi.Client) mockapi.ObjectIterator {
	query := &storage.Query{Prefix: g.Item}
	return client.Bucket(g.Bucket).Objects(g.Context, query)
}

func (g *GCSObjectDownloader) Download(client mockapi.Client, it mockapi.ObjectIterator) error {
	var errs []error
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("an error occurred while iterating: %v", err)
		}
		fileName := filepath.Join(g.ModelDir, g.ModelName, attrs.Name)
		if FileExists(fileName) {
			log.Println("Deleting", fileName)
			if err := os.Remove(fileName); err != nil {
				return fmt.Errorf("file is unable to be deleted: %v", err)
			}
		}
		file, err := Create(fileName)
		if err != nil {
			return fmt.Errorf("file is already created: %v", err)
		}
		if err := g.DownloadFile(client, attrs, file); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return awserr.NewBatchError("GCSDownloadIncomplete", "some objects failed to download.", errs)
	}
	return nil
}

func (g *GCSObjectDownloader) DownloadFile(client mockapi.Client, attrs *storage.ObjectAttrs, file *os.File) error {
	rc, err := client.Bucket(attrs.Bucket).Object(attrs.Name).NewReader(g.Context)
	if err != nil {
		return fmt.Errorf("failed to create reader for object(%s) in bucket(%s): %v",
			attrs.Name,
			attrs.Bucket,
			err,
		)
	}
	defer rc.Close()
	data, err := ioutil.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read object(%s) in bucket(%s): %v",
			attrs.Name,
			attrs.Bucket,
			err,
		)
	}
	return g.WriteToFile(data, attrs, file)
}

func (g *GCSObjectDownloader) WriteToFile(data []byte, attrs *storage.ObjectAttrs, file *os.File) error {
	_, err := file.Write(data)
	defer file.Close()
	if err != nil {
		return fmt.Errorf("failed to write data to file(%s): from object(%s) in bucket(%s): %v",
			file.Name(),
			attrs.Name,
			attrs.Bucket,
			err,
		)
	}
	return nil
}
