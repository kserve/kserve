package agent

import (
	"encoding/json"
	"github.com/fsnotify/fsnotify"
	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	"io/ioutil"
	"log"
	"path/filepath"
	"time"
)

type Watcher struct {
	ConfigDir    string
	ModelTracker map[string]ModelWrapper
	Puller       Puller
}

type LoadState string

const (
	// State Related
	ShouldLoad   LoadState = "Load"
	ShouldUnload LoadState = "Unload"
)

type EventWrapper struct {
	ModelName      string
	ModelSpec      *v1beta1.ModelSpec
	LoadState      LoadState
	ShouldDownload bool
}

type ModelWrapper struct {
	ModelSpec  *v1beta1.ModelSpec
	Time       time.Time
	Stale      bool
	Redownload bool
}

func (w *Watcher) Start() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

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
					symlink, _ := filepath.EvalSymlinks(eventPath)
					file, err := ioutil.ReadFile(filepath.Join(symlink, constants.ModelConfigFileName))
					modelConfigs := make(modelconfig.ModelConfigs, 0)
					if err != nil {
						log.Println("Error in reading file", err)
					}
					err = json.Unmarshal([]byte(file), &modelConfigs)
					if err != nil {
						log.Println("unable to marshall for", event, "with error", err)
					}
					w.ParseConfig(modelConfigs)
				}
			case err, ok := <-watcher.Errors:
				if ok { // 'Errors' channel is not closed
					log.Println("watcher error", err)
				}
				if !ok {
					return
				}
			}
		}
	}()
	err = watcher.Add(w.ConfigDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Watching", w.ConfigDir)
	<-done
}

func (w *Watcher) ParseConfig(modelConfigs modelconfig.ModelConfigs) {
	timeNow := time.Now()
	for _, modelConfig := range modelConfigs {
		modelName := modelConfig.Name
		modelSpec := modelConfig.Spec
		log.Println("Name:", modelName, "Spec:", modelSpec)
		oldModel, ok := w.ModelTracker[modelName]
		if !ok {
			w.ModelTracker[modelName] = ModelWrapper{
				ModelSpec:  &modelSpec,
				Time:       timeNow,
				Stale:      true,
				Redownload: true,
			}
		} else {
			isStale := true
			reDownload := true
			if oldModel.ModelSpec != nil {
				isStale = !cmp.Equal(*oldModel.ModelSpec, modelSpec)
				reDownload = !cmp.Equal(oldModel.ModelSpec.StorageURI, modelSpec.StorageURI)
				log.Println("same", !isStale, *oldModel.ModelSpec, modelSpec)
			}
			// Need to store new time, TODO: maybe worth to have seperate map?
			w.ModelTracker[modelName] = ModelWrapper{
				ModelSpec:  &modelSpec,
				Time:       timeNow,
				Stale:      isStale,
				Redownload: reDownload,
			}
		}
	}
	// TODO: Maybe make parallel and more efficient?
	for modelName, modelWrapper := range w.ModelTracker {
		if modelWrapper.Time.Before(timeNow) {
			delete(w. ModelTracker, modelName)
			channel, ok := w.Puller.ChannelMap[modelName]
			if !ok {
				log.Println("Model", modelName, "was never added to channel map")
			} else {
				event := EventWrapper{
					ModelName:      modelName,
					ModelSpec:      nil,
					LoadState:      ShouldUnload,
					ShouldDownload: false,
				}
				log.Println("Sending event", event)
				channel.EventChannel <- event
			}
		} else {
			if modelWrapper.Stale {
				channel, ok := w.Puller.ChannelMap[modelName]
				if !ok {
					log.Println("Need to add model", modelName)
					channel = w.Puller.AddModel(modelName)
				}
				event := EventWrapper{
					ModelName:      modelName,
					ModelSpec:      modelWrapper.ModelSpec,
					LoadState:      ShouldLoad,
					ShouldDownload: modelWrapper.Redownload,
				}
				log.Println("Sending event", event)
				channel.EventChannel <- event
			}
		}
	}
}
