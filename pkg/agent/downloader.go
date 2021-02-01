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
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Downloader struct {
	ModelDir  string
	Providers map[storage.Protocol]storage.Provider
	Logger    *zap.SugaredLogger
}

var SupportedProtocols = []storage.Protocol{storage.S3, storage.GCS}

func (d *Downloader) DownloadModel(modelName string, modelSpec *v1alpha1.ModelSpec) error {
	if modelSpec != nil {
		sha256 := storage.AsSha256(modelSpec)
		successFile := filepath.Join(d.ModelDir, modelName,
			fmt.Sprintf("SUCCESS.%s", sha256))
		d.Logger.Infof("Downloading %s to model dir %s", modelSpec.StorageURI, d.ModelDir)
		// Download if the event there is a success file and the event is one which we wish to Download
		_, err := os.Stat(successFile)
		if os.IsNotExist(err) {
			if err := d.download(modelName, modelSpec.StorageURI); err != nil {
				return errors.Wrapf(err, "failed to download model")
			}
			file, createErr := storage.Create(successFile)
			defer file.Close()
			if createErr != nil {
				return errors.Wrapf(createErr, "failed to create success file")
			}
			encodedJson, err := json.Marshal(modelSpec)
			if err != nil {
				return errors.Wrapf(createErr, "failed to encode model spec")
			}
			err = ioutil.WriteFile(successFile, encodedJson, 0644)
			if err != nil {
				return errors.Wrapf(createErr, "failed to write the success file")
			}
			d.Logger.Infof("Creating successFile %s", successFile)
		} else if err == nil {
			d.Logger.Infof("Model successFile exists already for %s", modelName)
		} else {
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
	provider, err := storage.GetProvider(d.Providers, protocol)
	if err != nil {
		return errors.Wrapf(err, "unable to create or get provider for protocol %s", protocol)
	}
	if err := provider.DownloadModel(d.ModelDir, modelName, storageUri); err != nil {
		return errors.Wrapf(err, "failed to download model")
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
