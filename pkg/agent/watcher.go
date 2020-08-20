package agent

import (
	"encoding/json"
	"github.com/fsnotify/fsnotify"
	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	"golang.org/x/sync/syncmap"
	"io/ioutil"
	"log"
	"path/filepath"
	"time"
)

type Watcher struct {
	ConfigDir    string
	ModelTracker *syncmap.Map
	NumWorkers   int
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
	NumRetries     int32
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
				timeNow := time.Now()
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
					for _, modelConfig := range modelConfigs {
						modelName := modelConfig.Name
						modelSpec := modelConfig.Spec
						log.Println("Name:", modelName, "Spec:", modelSpec)
						oldModelInterface, ok := w.ModelTracker.Load(modelName)
						if !ok {
							w.ModelTracker.Store(modelName, ModelWrapper{
								ModelSpec:  &modelSpec,
								Time:       timeNow,
								Stale:      true,
								Redownload: true,
							})
						} else {
							oldModel := oldModelInterface.(ModelWrapper)
							isStale := true
							reDownload := true
							if oldModel.ModelSpec != nil {
								isStale = !cmp.Equal(*oldModel.ModelSpec, modelSpec)
								reDownload = !cmp.Equal(oldModel.ModelSpec.StorageURI, modelSpec.StorageURI)
								log.Println("same", !isStale, *oldModel.ModelSpec, modelSpec)
							}
							// Need to store new time, TODO: maybe worth to have seperate map?
							w.ModelTracker.Store(modelName, ModelWrapper{
								ModelSpec:  &modelSpec,
								Time:       timeNow,
								Stale:      isStale,
								Redownload: reDownload,
							})
						}
					}
					// TODO: Maybe make parallel and more efficient?
					w.ModelTracker.Range(func(key interface{}, value interface{}) bool {
						modelName, modelWrapper := key.(string), value.(ModelWrapper)
						if modelWrapper.Time.Before(timeNow) {
							w.ModelTracker.Delete(modelName)
							channel, ok := w.Puller.ChannelMap[modelName]
							if !ok {
								log.Println("Model", modelName, "was never added to channel map")
							} else {
								event := EventWrapper{
									ModelName:      modelName,
									ModelSpec:      nil,
									LoadState:      ShouldUnload,
									ShouldDownload: false,
									NumRetries:     0,
								}
								log.Println("Sending event", event)
								channel.EventChannel <- event
							}
						} else {
							if modelWrapper.Stale {
								channel, ok := w.Puller.ChannelMap[modelName]
								if !ok {
									log.Println("Need to add model", modelName)
									channel = w.Puller.AddModel(modelName, w.NumWorkers)
								}
								event := EventWrapper{
									ModelName:      modelName,
									ModelSpec:      modelWrapper.ModelSpec,
									LoadState:      ShouldLoad,
									ShouldDownload: modelWrapper.Redownload,
									NumRetries:     0,
								}
								log.Println("Sending event", event)
								channel.EventChannel <- event
							}
						}
						return true
					})
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
	<-done
}
