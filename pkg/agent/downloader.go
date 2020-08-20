package agent

import (
	"fmt"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Downloader struct {
	ModelDir  string
	Providers map[storage.Protocol]storage.Provider
}

var SupportedProtocols = []storage.Protocol{storage.S3}

func (d *Downloader) DownloadModel(event EventWrapper) error {
	modelSpec := event.ModelSpec
	modelName := event.ModelName
	if modelSpec != nil {
		modelUri := modelSpec.StorageURI
		hashString := hash(modelUri)
		log.Println("Processing:", modelUri, "=", hashString)
		successFile := filepath.Join(d.ModelDir, modelName, fmt.Sprintf("SUCCESS.%d", hashString))
		// Download if the event there is a success file and the event is one which we wish to Download
		if !storage.FileExists(successFile) && event.ShouldDownload {
			if err := d.download(modelName, modelUri); err != nil {
				return fmt.Errorf("download error: %v", err)
			}
			file, createErr := os.Create(successFile)
			if createErr != nil {
				return fmt.Errorf("create file error: %v", createErr)
			}
			defer file.Close()
		} else if !event.ShouldDownload {
			log.Println("Model", modelName, "does not need to be re-downloaded")
		} else {
			log.Println("Model", modelSpec.StorageURI, "exists already")
		}
	}
	return nil
}

func (d *Downloader) download(modelName string, storageUri string) error {
	log.Println("Downloading: ", storageUri)
	protocol, err := extractProtocol(storageUri)
	if err != nil {
		return fmt.Errorf("unsupported protocol: %v", err)
	}
	manager, ok := d.Providers[protocol]
	if !ok {
		return fmt.Errorf("protocol manager for %s is not initialized", protocol)
	}
	// TODO: Back-off retries
	if err := manager.Download(d.ModelDir, modelName, storageUri); err != nil {
		return fmt.Errorf("failure on download: %v", err)
	}

	return nil
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func extractProtocol(storageURI string) (storage.Protocol, error) {
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
