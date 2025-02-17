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

package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

type Downloader struct {
	ModelDir  string
	mu        sync.Mutex
	Providers map[storage.Protocol]storage.Provider
	Logger    *zap.SugaredLogger
}

func (d *Downloader) DownloadModel(modelName string, modelSpec *v1alpha1.ModelSpec) error {
	if modelSpec != nil {
		sha256 := storage.AsSha256(modelSpec)
		successFile := filepath.Join(d.ModelDir, modelName,
			"SUCCESS."+sha256)
		d.Logger.Infof("Downloading %s to model dir %s", modelSpec.StorageURI, d.ModelDir)
		// Download if the event there is a success file and the event is one which we wish to Download
		_, err := os.Stat(successFile)
		switch {
		case os.IsNotExist(err):
			if err := d.download(modelName, modelSpec.StorageURI); err != nil {
				return errors.Wrapf(err, "failed to download model")
			}
			file, createErr := storage.Create(successFile)
			if createErr != nil {
				return errors.Wrapf(createErr, "failed to create success file")
			}
			defer func(file *os.File) {
				err := file.Close()
				if err != nil {
					d.Logger.Errorf("Failed to close created file %v", err)
				}
			}(file)
			encodedJson, err := json.Marshal(modelSpec)
			if err != nil {
				return errors.Wrapf(createErr, "failed to encode model spec")
			}
			err = os.WriteFile(successFile, encodedJson, 0o644) // #nosec G306
			if err != nil {
				return errors.Wrapf(createErr, "failed to write the success file")
			}
			d.Logger.Infof("Creating successFile %s", successFile)
		case err == nil:
			d.Logger.Infof("Model successFile exists already for %s", modelName)
		default:
			d.Logger.Errorf("Model successFile error %v", err)
		}
	}
	return nil
}

func (d *Downloader) download(modelName string, storageUri string) error {
	protocol, err := extractProtocol(storageUri)
	if err != nil {
		return errors.Wrapf(err, "unsupported protocol")
	}
	d.mu.Lock()
	provider, err := storage.GetProvider(d.Providers, protocol)
	d.mu.Unlock()
	if err != nil {
		return errors.Wrapf(err, "unable to create or get provider for protocol %s", protocol)
	}
	if err := provider.DownloadModel(d.ModelDir, modelName, storageUri); err != nil {
		return errors.Wrapf(err, "failed to download model")
	}
	return nil
}

func extractProtocol(storageURI string) (storage.Protocol, error) {
	if storageURI == "" {
		return "", errors.New("there is no storageUri supplied")
	}

	if !regexp.MustCompile(`\w+?://`).MatchString(storageURI) {
		return "", errors.New("there is no protocol specified for the storageUri")
	}

	for _, prefix := range storage.SupportedProtocols {
		if strings.HasPrefix(storageURI, string(prefix)) {
			return prefix, nil
		}
	}
	return "", errors.New("protocol not supported for storageUri")
}
