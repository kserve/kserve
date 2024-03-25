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
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type OCIProvider struct {
	Client *http.Client
}

func (m *OCIProvider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	log.Info("Download model ", "modelName", modelName, "storageUri", storageUri, "modelDir", modelDir)
	uri, err := url.Parse(storageUri)
	if err != nil {
		return fmt.Errorf("unable to parse storage uri: %w", err)
	}
	OCIDownloader := &OCIDownloader{
		StorageUri: storageUri,
		ModelDir:   modelDir,
		ModelName:  modelName,
		Uri:        uri,
	}
	if err := OCIDownloader.Download(*m.Client); err != nil {
		return err
	}
	return nil
}

type OCIDownloader struct {
	StorageUri string
	ModelDir   string
	ModelName  string
	Uri        *url.URL
}

func (h *OCIDownloader) Download(client http.Client) error {
	// Create request
	req, err := http.NewRequest("GET", h.StorageUri, nil)
	if err != nil {
		return err
	}

	// Query request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make a request: %w", err)
	}

	defer func(Body io.ReadCloser) {
		closeErr := Body.Close()
		if closeErr != nil {
			log.Error(closeErr, "failed to close body")
		}
	}(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("URI: %s returned a %d response code", h.StorageUri, resp.StatusCode)
	}

	// Write content into file(s)
	contentType := resp.Header.Get("Content-type")
	fileDirectory := filepath.Join(h.ModelDir, h.ModelName)

	if strings.Contains(contentType, "application/zip") {
		if err := extractZipFiles(resp.Body, fileDirectory); err != nil {
			return err
		}
	} else if strings.Contains(contentType, "application/x-tar") || strings.Contains(contentType, "application/x-gtar") ||
		strings.Contains(contentType, "application/x-gzip") || strings.Contains(contentType, "application/gzip") {
		if err := extractTarFiles(resp.Body, fileDirectory); err != nil {
			return err
		}
	} else {
		paths := strings.Split(h.Uri.Path, "/")
		fileName := paths[len(paths)-1]
		fileFullName := filepath.Join(fileDirectory, fileName)
		file, err := createNewFile(fileFullName)
		if err != nil {
			return err
		}
		if _, err = io.Copy(file, resp.Body); err != nil {
			return fmt.Errorf("unable to copy file content: %w", err)
		}
	}

	return nil
}
