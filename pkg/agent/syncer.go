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
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
	"path/filepath"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strings"
)

type FileError error

var NoSuccessFile FileError = fmt.Errorf("no success file can be found")

func SyncModelDir(modelDir string) (map[string]modelWrapper, error) {
	log := logf.Log.WithName("Syncer")
	log.Info("Syncing model directory..", "modelDir", modelDir)
	modelTracker := make(map[string]modelWrapper)
	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			modelName := info.Name()
			ierr := filepath.Walk(path, func(path string, f os.FileInfo, _ error) error {
				if !f.IsDir() {
					base := filepath.Base(path)
					baseSplit := strings.SplitN(base, ".", 4)
					if baseSplit[0] == "SUCCESS" {
						if spec, e := successParse(baseSplit); e != nil {
							return errors.Wrapf(err, "error parsing SUCCESS file")
						} else {
							modelTracker[modelName] = modelWrapper{
								Spec:  spec,
								stale: true,
							}
							return nil
						}
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

func successParse(baseSplit []string) (*v1alpha1.ModelSpec, error) {
	storageURI, err := unhash(baseSplit[1])
	errorMessage := "unable to unhash the SUCCESS file, maybe the SUCCESS file has been modified?"
	if err != nil {
		return nil, errors.Wrapf(err, errorMessage)
	}
	framework, err := unhash(baseSplit[2])
	if err != nil {
		return nil, errors.Wrapf(err, errorMessage)
	}
	memory, err := unhash(baseSplit[3])
	if err != nil {
		return nil, errors.Wrapf(err, errorMessage)
	}
	memoryResource := resource.MustParse(memory)
	return &v1alpha1.ModelSpec{
		StorageURI: storageURI,
		Framework:  framework,
		Memory:     memoryResource,
	}, nil
}

func unhash(s string) (string, error) {
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return "", nil
	}
	return string(decoded), nil
}
