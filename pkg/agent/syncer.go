package agent

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/resource"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Syncer struct {
	Watcher Watcher
}

func (s *Syncer) Start() {
	modelDir := filepath.Clean(s.Watcher.Puller.Downloader.ModelDir)
	timeNow := time.Now()
	err := filepath.Walk(modelDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			modelName := info.Name()
			ierr := filepath.Walk(path, func(path string, f os.FileInfo, _ error) error {
				if !f.IsDir() {
					base := filepath.Base(path)
					baseSplit := strings.SplitN(base,".", 4)
					if baseSplit[0] == "SUCCESS" {
						if e := s.successParse(timeNow, modelName, baseSplit); e != nil {
							return fmt.Errorf("error parsing SUCCESS file: %v", e)
						}
					}
				}
				return fmt.Errorf("did not find success file")
			})
			log.Println("result from walk:", ierr)
		}
		return nil
	})
	if err != nil {
		log.Println("error in going through:", modelDir, err)
	}
	filePath := filepath.Join(s.Watcher.ConfigDir, constants.ModelConfigFileName)
	log.Println("Syncing of", filePath)
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Println("Error in reading file", err)
	}
	modelConfigs := make(modelconfig.ModelConfigs, 0)
	err = json.Unmarshal([]byte(file), &modelConfigs)
	if err != nil {
		log.Println("unable to marshall for modelConfig with error", err)
	}
	s.Watcher.ParseConfig(modelConfigs)
}

func (s *Syncer) successParse(timeNow time.Time, modelName string, baseSplit []string) error {
	storageURI, err := unhash(baseSplit[1])
	errorMessage := "unable to unhash the SUCCESS file, maybe the SUCCESS file has been modified?: %v"
	if err != nil {
		return fmt.Errorf(errorMessage, err)
	}
	framework, err := unhash(baseSplit[2])
	if err != nil {
		return fmt.Errorf(errorMessage, err)
	}
	memory, err := unhash(baseSplit[3])
	if err != nil {
		return fmt.Errorf(errorMessage, err)
	}
	memoryResource := resource.MustParse(memory)

	s.Watcher.ModelTracker[modelName] = ModelWrapper{
		ModelSpec:  &v1beta1.ModelSpec{
			StorageURI: storageURI,
			Framework:  framework,
			Memory:     memoryResource,
		},
		Time:       timeNow,
		Stale:      true,
		Redownload: true,
	}
	return nil
}

func unhash(s string) (string, error) {
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return "", nil
	}
	return string(decoded), nil
}
