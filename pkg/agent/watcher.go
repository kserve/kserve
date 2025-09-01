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
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/modelconfig"
)

type Watcher struct {
	configDir    string
	ModelTracker map[string]modelWrapper
	ModelEvents  chan ModelOp
	logger       *zap.SugaredLogger
}

func NewWatcher(configDir string, modelDir string, logger *zap.SugaredLogger) Watcher {
	modelTracker, err := SyncModelDir(modelDir, logger)
	if err != nil {
		logger.Errorf("Failed to sync model dir %v", err)
	}
	watcher := Watcher{
		configDir:    configDir,
		ModelTracker: modelTracker,
		ModelEvents:  make(chan ModelOp, 100),
		logger:       logger,
	}
	modelConfigFile := fmt.Sprintf("%s/%s", configDir, constants.ModelConfigFileName)
	err = watcher.syncModelConfig(modelConfigFile, true)
	if err != nil {
		logger.Errorf("Failed to sync model config file %v", err)
	}
	return watcher
}

type modelWrapper struct {
	Spec  *v1alpha1.ModelSpec
	stale bool
}

func (w *Watcher) syncModelConfig(modelConfigFile string, initializing bool) error {
	file, err := os.ReadFile(modelConfigFile)
	if err != nil {
		return err
	} else {
		modelConfigs := make(modelconfig.ModelConfigs, 0)
		err = json.Unmarshal(file, &modelConfigs)
		if err != nil {
			return err
		} else {
			w.parseConfig(modelConfigs, initializing)
		}
	}
	return nil
}

func (w *Watcher) Start() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.logger.Error(err, "Failed to create model dir watcher")
		panic(err)
	}
	defer func(watcher *fsnotify.Watcher) {
		closeErr := watcher.Close()
		if closeErr != nil {
			w.logger.Error(closeErr, "Failed to close watcher")
		}
	}(watcher)
	if err = watcher.Add(w.configDir); err != nil {
		w.logger.Error(err, "Failed to add watcher config dir")
	}
	w.logger.Info("Start to watch model config event")
	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				isCreate := event.Op&fsnotify.Create != 0
				eventPath := filepath.Clean(event.Name)
				isDataDir := filepath.Base(eventPath) == "..data"
				// TODO: Should we use atomic integer or timestamp??
				if isDataDir && isCreate {
					w.logger.Infof("Processing event %s", event)
					symlink, _ := filepath.EvalSymlinks(eventPath)
					modelConfigFile := filepath.Join(symlink, constants.ModelConfigFileName)
					err := w.syncModelConfig(modelConfigFile, false)
					if err != nil {
						w.logger.Error(err, "Failed to sync model config file")
					}
				}
			case err, ok := <-watcher.Errors:
				if ok { // 'Errors' channel is not closed
					w.logger.Error(err, "watcher error")
				}
				if !ok {
					return
				}
			}
		}
	}()
	// Add a first create event to the channel to force initial sync
	watchPath := filepath.Join(w.configDir, "../data", constants.ModelConfigFileName)
	watcher.Events <- fsnotify.Event{
		Name: filepath.Join(w.configDir, "..data/"+constants.ModelConfigFileName),
		Op:   fsnotify.Create,
	}
	w.logger.Infof("Watching %s", watchPath)
	<-done
}

func (w *Watcher) parseConfig(modelConfigs modelconfig.ModelConfigs, initializing bool) {
	for _, modelConfig := range modelConfigs {
		name, spec := modelConfig.Name, modelConfig.Spec
		existing, exists := w.ModelTracker[name]
		switch {
		case !exists:
			// New - add
			w.ModelTracker[name] = modelWrapper{Spec: &spec}
			w.modelAdded(name, &spec, initializing)
		case !cmp.Equal(spec, *existing.Spec):
			w.ModelTracker[name] = modelWrapper{
				Spec:  existing.Spec,
				stale: false,
			}
			// Changed - replace
			w.modelRemoved(name)
			w.modelAdded(name, &spec, initializing)
		default:
			// This model didn't change, mark the stale flag to false
			w.ModelTracker[name] = modelWrapper{
				Spec:  existing.Spec,
				stale: false,
			}
		}
	}
	for name, wrapper := range w.ModelTracker {
		if wrapper.stale {
			// Remove the models that are marked as stale
			delete(w.ModelTracker, name)
			w.modelRemoved(name)
		} else {
			// Mark all the models as stale by default, when the next CREATE event is triggered
			// the watcher will mark stale: false to all the models that didn't change so they won't
			// be removed.
			w.ModelTracker[name] = modelWrapper{
				Spec:  wrapper.Spec,
				stale: true,
			}
		}
	}
}

func (w *Watcher) modelAdded(name string, spec *v1alpha1.ModelSpec, initializing bool) {
	w.logger.Infof("adding model %s", name)
	w.ModelEvents <- ModelOp{
		OnStartup: initializing,
		ModelName: name,
		Op:        Add,
		Spec:      spec,
	}
}

func (w *Watcher) modelRemoved(name string) {
	w.logger.Infof("removing model %s", name)
	w.ModelEvents <- ModelOp{
		ModelName: name,
		Op:        Remove,
	}
}
