package agent

import (
	"encoding/json"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	"io/ioutil"
	"log"
	"path/filepath"
)

type Syncer struct {
	Watcher Watcher
}

func (s *Syncer) Start() {
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
