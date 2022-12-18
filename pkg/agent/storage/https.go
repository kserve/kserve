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
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	HEADER_SUFFIX = "-headers"
)

type HTTPSProvider struct {
	Client *http.Client
}

func (m *HTTPSProvider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	log.Info("Download model ", "modelName", modelName, "storageUri", storageUri, "modelDir", modelDir)
	uri, err := url.Parse(storageUri)
	if err != nil {
		return fmt.Errorf("unable to parse storage uri: %v", err)
	}
	HTTPSDownloader := &HTTPSDownloader{
		StorageUri: storageUri,
		ModelDir:   modelDir,
		ModelName:  modelName,
		Uri:        uri,
	}
	if err := HTTPSDownloader.Download(*m.Client); err != nil {
		return err
	}
	return nil
}

type HTTPSDownloader struct {
	StorageUri string
	ModelDir   string
	ModelName  string
	Uri        *url.URL
}

func (h *HTTPSDownloader) Download(client http.Client) error {
	// Create request
	req, err := http.NewRequest("GET", h.StorageUri, nil)
	if err != nil {
		return err
	}

	headers, err := h.extractHeaders()
	if err != nil {
		return err
	}
	for key, element := range headers {
		req.Header.Add(key, element)
	}

	// Query request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make a request: %v", err)
	}

	defer resp.Body.Close()
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
			return fmt.Errorf("unable to copy file content: %v", err)
		}
	}

	return nil
}

func (h *HTTPSDownloader) extractHeaders() (map[string]string, error) {
	var headers map[string]string
	hostname := h.Uri.Hostname()
	headerJSON := os.Getenv(hostname + HEADER_SUFFIX)
	json.Unmarshal([]byte(headerJSON), &headers)
	return headers, nil
}

func createNewFile(fileFullName string) (*os.File, error) {
	if FileExists(fileFullName) {
		if err := os.Remove(fileFullName); err != nil {
			return nil, fmt.Errorf("file is unable to be deleted: %v", err)
		}
	}

	file, err := Create(fileFullName)
	if err != nil {
		return nil, fmt.Errorf("file is already created: %v", err)
	}
	return file, nil
}

func extractZipFiles(reader io.Reader, dest string) error {
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("unable to create new reader: %v", err)
	}

	// Read all the files from zip archive
	for _, zipFile := range zipReader.File {
		fileFullPath := filepath.Join(dest, zipFile.Name)
		if !strings.HasPrefix(fileFullPath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", fileFullPath)
		}

		if zipFile.Mode().IsDir() {
			err = os.MkdirAll(fileFullPath, 0755)
			if err != nil {
				return fmt.Errorf("unable to create new directory %s", fileFullPath)
			}

			continue
		}

		file, err := createNewFile(fileFullPath)
		if err != nil {
			return err
		}

		rc, err := zipFile.Open()
		if err != nil {
			return fmt.Errorf("unable to open file: %v", err)
		}

		_, err = io.Copy(file, rc)
		file.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("unable to copy file content: %v", err)
		}
	}
	return nil
}

func extractTarFiles(reader io.Reader, dest string) error {
	gzr, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Read all the files from tar archive
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("unable to access next tar file: %v", err)
		}

		fileFullPath := filepath.Join(dest, header.Name)
		if header.Typeflag == tar.TypeDir {
			err = os.MkdirAll(fileFullPath, 0755)
			if err != nil {
				return fmt.Errorf("unable to create new directory %s", fileFullPath)
			}

			continue
		}

		newFile, err := createNewFile(fileFullPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(newFile, tr); err != nil {
			return fmt.Errorf("unable to copy contents to %s: %v", header.Name, err)
		}
	}
	return nil
}
