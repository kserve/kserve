package puller

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	d *Downloader
)

type Downloader struct {
	NumRetries int
	ModelDir   string
}

type Protocol string

const (
	// Supported Protocols
	S3    Protocol = "s3://"
	GS    Protocol = "gs://"
	PVC   Protocol = "pvc://"
	File  Protocol = "file://"
	HTTPS Protocol = "https://"
)

// TODO: Add more supported protocols
var SupportedProtocols = []Protocol{S3}

func init() {
	d = NewDownloader()
}

func NewDownloader() *Downloader {
	d := new(Downloader)
	return d
}

func SetEnvs(numRetries int, modelDir string) {
	d.NumRetries = numRetries
	d.ModelDir = modelDir
}

func DownloadModel(event EventWrapper) error {
	modelSpec := event.ModelSpec
	if modelSpec != nil {
		modelUri := modelSpec.StorageURI
		hashString := hash(modelUri)
		log.Println("Processing:", modelUri, "=", hashString)
		successFile := filepath.Join(d.ModelDir, fmt.Sprintf("SUCCESS.%d", hashString))
		ok := fileExists(successFile)
		if !ok {
			downloadErr := download(modelUri)
			if downloadErr != nil {
				return fmt.Errorf("download error: %v", downloadErr)
			} else {
				file, createErr := os.Create(successFile)
				if createErr != nil {
					return fmt.Errorf("create file error: %v", createErr)
				}
				defer file.Close()
			}
		} else {
			log.Println("Model", modelSpec.StorageURI, "exists already")
		}
	}
	return nil
}

func download(storageUri string) error {
	log.Println("Downloading: ", storageUri)
	prefix, err := validateStorageURI(storageUri)
	if err != nil {
		return err
	}
	switch prefix {
	case S3:
		s3Uri := strings.TrimPrefix(storageUri, string(S3))
		path := strings.SplitN(s3Uri, "/", 2)
		bucket := path[0]
		item := path[1]
		log.Println("do an s3 request on", bucket, "for key", item)
		fileName := filepath.Join(d.ModelDir, item)
		ok := fileExists(fileName)
		if ok {
			// File got corrupted or is mid-download :(
			// TODO: Figure out if we can maybe continue?
			log.Println("Deleting", fileName)
			err := os.Remove(fileName)
			if err != nil {
				return fmt.Errorf("file is unable to be deleted: %v", err)
			}
		}
		file, err := create(fileName)
		if err != nil {
			return fmt.Errorf("file is already created: %v", err)
		}
		defer file.Close()

		// TODO: Should the S3 client be shared?
		sess, err := session.NewSession()
		if err != nil {
			return fmt.Errorf("unable to get new s3 session %v", err)
		}
		s3Svc := s3.New(sess)
		downloader := s3manager.NewDownloaderWithClient(s3Svc, func(d *s3manager.Downloader) {
			d.Concurrency = 1             // TODO: Override?
			d.PartSize = 64 * 1024 * 1024 // TODO: Override?
		})
		numBytes, err := downloader.Download(file,
			&s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(item),
			})
		if err != nil {
			return fmt.Errorf("unable to download %s: %v", s3Uri, err)
		}
		fileState, err := os.Stat(fileName)
		if err != nil {
			return fmt.Errorf("unable to get file info %s: %v", fileName, err)
		}
		log.Println("File size:", fileState.Size(), "num bytes:", numBytes)
	}
	return nil
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func validateStorageURI(storageURI string) (Protocol, error) {
	if storageURI == "" {
		return "", fmt.Errorf("there is no storageUri supplied")
	}

	if !regexp.MustCompile("\\w+?://").MatchString(storageURI) {
		return "", fmt.Errorf("there is no protocol specificed for the storageUri")
	}

	for _, prefix := range SupportedProtocols {
		if strings.HasPrefix(storageURI, string(prefix)) {
			return prefix, nil
		}
	}
	return "", fmt.Errorf("protocol not supported for storageUri")
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func create(fileName string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(fileName), 0770); err != nil {
		return nil, err
	}
	return os.Create(fileName)
}
