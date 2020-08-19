package protocols

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type S3Manager struct {
	S3 *s3.S3
}

func (m *S3Manager) Download(modelDir string, modelName string, storageUri string) error {
	s3Uri := strings.TrimPrefix(storageUri, string(S3))
	path := strings.SplitN(s3Uri, "/", 2)
	bucket := path[0]
	item := path[1]
	s3ObjectLoactor := &S3ObjectDownloader{
		ModelDir: modelDir,
		ModelName: modelName,
		Bucket: bucket,
		Item: item,
	}
	objects, err := s3ObjectLoactor.GetAllObjects(m.S3)
	if err != nil {
		return fmt.Errorf("unable to get batch objects %v", err)
	}
	err = s3ObjectLoactor.Download(m.S3, objects)
	if err != nil {
		return fmt.Errorf("unable to get download objects %v", err)
	}
	return nil
}

var _ ProtocolManager = (*S3Manager)(nil)

type S3ObjectDownloader struct {
	ModelDir string
	ModelName string
	Bucket string
	Item string
}

func (s *S3ObjectDownloader) GetAllObjects(s3Svc *s3.S3) ([]s3manager.BatchDownloadObject, error){
	resp, err := s3Svc.ListObjects(&s3.ListObjectsInput{
		Bucket:    aws.String(s.Bucket),
		Prefix:    aws.String(s.Item),
	})
	if err != nil {
		return nil, err
	}
	results := make([]s3manager.BatchDownloadObject, 0)

	for _, object := range resp.Contents {
		fileName := filepath.Join(s.ModelDir, s.ModelName, *object.Key)
		ok := FileExists(fileName)
		if ok {
			// File got corrupted or is mid-download :(
			// TODO: Figure out if we can maybe continue?
			log.Println("Deleting", fileName)
			err := os.Remove(fileName)
			if err != nil {
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
				return nil
			},
		}
		results = append(results, object)
	}
	return results, nil
}

func (s *S3ObjectDownloader) Download(s3Svc *s3.S3, objects []s3manager.BatchDownloadObject) error {
	iter := &s3manager.DownloadObjectsIterator{Objects: objects}
	downloader := s3manager.NewDownloaderWithClient(s3Svc, func(d *s3manager.Downloader) {
		d.Concurrency = 5             // TODO: Override?
		d.PartSize = 64 * 1024 * 1024 // TODO: Override?
	})
	if err := downloader.DownloadWithIterator(aws.BackgroundContext(), iter); err != nil {
		return err
	}
	return nil
}
