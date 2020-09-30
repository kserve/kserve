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
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Syncer struct {
	Watcher Watcher
}

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
					baseSplit := strings.SplitN(base, ".", 4)
					if baseSplit[0] == "SUCCESS" {
						if spec, e := successParse(modelName, baseSplit); e != nil {
							return fmt.Errorf("error parsing SUCCESS file: %v", e)
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
				log.Println("failed to parse SUCCESS file:", ierr)
				return ierr
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error in syncing %s: %w", modelDir, err)
	}
	return modelTracker, nil
}

func successParse(modelName string, baseSplit []string) (*v1beta1.ModelSpec, error) {
	storageURI, err := unhash(baseSplit[1])
	errorMessage := "unable to unhash the SUCCESS file, maybe the SUCCESS file has been modified?: %v"
	if err != nil {
		return nil, fmt.Errorf(errorMessage, err)
	}
	framework, err := unhash(baseSplit[2])
	if err != nil {
		return nil, fmt.Errorf(errorMessage, err)
	}
	memory, err := unhash(baseSplit[3])
	if err != nil {
		return nil, fmt.Errorf(errorMessage, err)
	}
	memoryResource := resource.MustParse(memory)
	return &v1beta1.ModelSpec{
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
