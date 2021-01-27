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
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type FileError error

var NoSuccessFile FileError = fmt.Errorf("no success file can be found")

func SyncModelDir(modelDir string) (map[string]modelWrapper, error) {
	modelTracker := make(map[string]modelWrapper)
	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			modelName := info.Name()
			ierr := filepath.Walk(path, func(path string, f os.FileInfo, _ error) error {
				if !f.IsDir() {
					base := filepath.Base(path)
					baseSplit := strings.SplitN(base, ".", 2)
					if baseSplit[0] == "SUCCESS" {
						jsonFile, err := os.Open(path)
						if err != nil {
							return errors.Wrapf(err, "failed to parse success file")
						}
						byteValue, _ := ioutil.ReadAll(jsonFile)
						modelSpec := &v1alpha1.ModelSpec{}
						err = json.Unmarshal(byteValue, &modelSpec)
						if err != nil {
							return errors.Wrapf(err, "failed to unmarshal model spec")
						}
						modelTracker[modelName] = modelWrapper{
							Spec:  modelSpec,
							stale: true,
						}
						return nil
					}
				}
				return NoSuccessFile
			})
			switch ierr {
			case NoSuccessFile:
				return nil
			default:
				return errors.Wrapf(ierr, "failed to parse success file")
			}
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error in syncing model dir")
	}
	return modelTracker, nil
}

