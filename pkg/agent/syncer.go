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
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

type FileError error

var ErrNoSuccessFile FileError = errors.New("no success file can be found")

func SyncModelDir(modelDir string, logger *zap.SugaredLogger) (map[string]modelWrapper, error) {
	logger.Infof("Syncing from model dir %s", modelDir)
	modelTracker := make(map[string]modelWrapper)
	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			fileName := info.Name()
			if strings.HasPrefix(fileName, "SUCCESS.") {
				logger.Infof("Syncing from model success file %v", fileName)
				dir := filepath.Dir(path)
				dirSplit := strings.Split(dir, "/")
				if len(dirSplit) < 2 {
					return errors.Wrapf(err, "invalid model path")
				}
				modelName := dirSplit[len(dirSplit)-1]

				jsonFile, err := os.Open(path)
				if err != nil {
					return errors.Wrapf(err, "failed to parse success file")
				}
				byteValue, err := io.ReadAll(jsonFile)
				if err != nil {
					return errors.Wrapf(err, "failed to read from model spec")
				}
				modelSpec := &v1alpha1.ModelSpec{}
				err = json.Unmarshal(byteValue, &modelSpec)
				if err != nil {
					return errors.Wrapf(err, "failed to unmarshal model spec")
				}
				modelTracker[dirSplit[len(dirSplit)-1]] = modelWrapper{
					Spec:  modelSpec,
					stale: true,
				}
				logger.Infof("recovered model %s with spec %+v", modelName, modelSpec)
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error in syncing model dir")
	}
	return modelTracker, nil
}
