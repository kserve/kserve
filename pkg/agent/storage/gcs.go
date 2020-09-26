package storage

import (
	gstorage "cloud.google.com/go/storage"
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
	"google.golang.org/api/iterator"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
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
	gcsObjectDownloader := &GCSObjectDownloader {
		Context: ctx,
		StorageUri: storageUri,
		ModelDir: modelDir,
		ModelName: modelName,
		Bucket: tokens[0],
		Item: prefix,
	}
	it, err := gcsObjectDownloader.GetObjectIterator(p.Client)
	if err != nil {
		return fmt.Errorf("unable to get object iterator because: %v", err)
	}
	if err := gcsObjectDownloader.Download(p.Client, it); err != nil {
		return fmt.Errorf("unable to download object/s %v", err)
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

func (g *GCSObjectDownloader) GetObjectIterator(client stiface.Client) (stiface.ObjectIterator, error) {
	query := &gstorage.Query{Prefix: g.Item}
	return client.Bucket(g.Bucket).Objects(g.Context, query), nil
}

func (g *GCSObjectDownloader) Download(client stiface.Client, it stiface.ObjectIterator) error {
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
			log.Info("Deleting", fileName)
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

func (g *GCSObjectDownloader) DownloadFile(client stiface.Client, attrs *gstorage.ObjectAttrs, file *os.File) error {
	reader, err := client.Bucket(attrs.Bucket).Object(attrs.Name).NewReader(g.Context)
	if err != nil {
		return fmt.Errorf("failed to create reader for object(%s) in bucket(%s): %v",
			attrs.Name,
			attrs.Bucket,
			err,
		)
	}
	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read object(%s) in bucket(%s): %v",
			attrs.Name,
			attrs.Bucket,
			err,
		)
	}
	return g.WriteToFile(data, attrs, file)
}

func (g *GCSObjectDownloader) WriteToFile(data []byte, attrs *gstorage.ObjectAttrs, file *os.File) error {
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
	log.Info("Wrote " + attrs.Prefix + " to file " + file.Name())
	return nil
}
