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

package agent

import (
	"encoding/hex"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"log"
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
		hashModelUri := hash(modelUri)
		hashFramework := hash(modelSpec.Framework)
		hashMemory := hash(modelSpec.Memory.String())
		log.Println("Processing:", modelUri, "=", hashModelUri, hashFramework, hashMemory)
		successFile := filepath.Join(d.ModelDir, modelName,
			fmt.Sprintf("SUCCESS.%s.%s.%s", hashModelUri, hashFramework, hashMemory))
		// Download if the event there is a success file and the event is one which we wish to Download
		if !storage.FileExists(successFile) && event.ShouldDownload {
			// TODO: Handle retry logic
			if err := d.download(modelName, modelUri); err != nil {
				return fmt.Errorf("download error: %v", err)
			}
			file, createErr := storage.Create(successFile)
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
	provider, ok := d.Providers[protocol]
	if !ok {
		return fmt.Errorf("protocol manager for %s is not initialized", protocol)
	}
	if err := provider.Download(d.ModelDir, modelName, storageUri); err != nil {
		return fmt.Errorf("failure on download: %v", err)
	}

	return nil
}

func hash(s string) string {
	src := []byte(s)
	dst := make([]byte, hex.EncodedLen(len(src)))
	hex.Encode(dst, src)
	return string(dst)
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
