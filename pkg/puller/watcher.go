package puller

import (
	"encoding/json"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/syncmap"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var w *Watcher

type Watcher struct {
	configDir    string
	ModelTracker *syncmap.Map
	numWorkers   int
}

type ModelDefinition struct {
	// TODO: This needs to be defined by the ConfigMap PR
	StorageUri string `json:"storageUri"`
	Framework  string `json:"framework"`
}

func init() {
	w = NewWatcher()
}

func NewWatcher() *Watcher {
	w := new(Watcher)
	m := new(syncmap.Map)
	w.ModelTracker = m
	return w
}

type LoadState string

const (
	// State Related
	ShouldLoad   LoadState = "Load"
	ShouldUnload LoadState = "Unload"
)

type EventWrapper struct {
	ModelDef       *ModelDefinition
	LoadState      LoadState
	ShouldDownload bool
}

type ModelWrapper struct {
	ModelDef *ModelDefinition
	Time     time.Time
	Stale    bool
	Success  bool
}

func WatchConfig(configDir string, numWorkers int) {
	log.Println("Entering watch")
	w.configDir = configDir
	w.numWorkers = numWorkers
	w.WatchConfig()
}

func (w *Watcher) WatchConfig() {
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
				if isDataDir && isCreate {
					symlink, _ := filepath.EvalSymlinks(eventPath)
					timeNow := time.Now()
					err := filepath.Walk(symlink, func(path string, info os.FileInfo, err error) error {
						// TODO: Filter SUCCESS files when they are added
						if !info.IsDir() {
							file, _ := ioutil.ReadFile(path)
							modelName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
							modelDef := ModelDefinition{}
							err := json.Unmarshal([]byte(file), &modelDef)
							if err != nil {
								return fmt.Errorf("unable to marshall for %v, error: %v", event, err)
							} else {
								log.Println("Model:", modelName, "storage:", modelDef.StorageUri)
							}
							oldModelInterface, ok := w.ModelTracker.Load(modelName)
							if !ok {
								w.ModelTracker.Store(modelName, ModelWrapper{
									ModelDef: &modelDef,
									Time:     timeNow,
									Stale:    true,
									Success:  false,
								})
							} else {
								oldModel := oldModelInterface.(ModelWrapper)
								isSame := false
								if oldModel.ModelDef != nil {
									isSame = cmp.Equal(*oldModel.ModelDef, modelDef)
									log.Println("same", isSame, *oldModel.ModelDef, modelDef)
								}
								if isSame {
									// Need to store new time, maybe worth to have seperate map?
									w.ModelTracker.Store(modelName, ModelWrapper{
										ModelDef: &modelDef,
										Time:     timeNow,
										Stale:    false,
										Success:  false,
									})
								} else {
									w.ModelTracker.Store(modelName, ModelWrapper{
										ModelDef: &modelDef,
										Time:     timeNow,
										Stale:    true,
										Success:  false,
									})
								}

							}
						}
						return nil
					})
					// TODO: Maybe make parallel and more efficient?
					w.ModelTracker.Range(func(key interface{}, value interface{}) bool {
						modelName, modelWrapper := key.(string), value.(ModelWrapper)
						if modelWrapper.Time.Before(timeNow) {
							log.Println("Delete", modelName)
							w.ModelTracker.Delete(modelName)
							channel, ok := p.ChannelMap[modelName]
							if !ok {
								log.Println("Model", modelName, "was never added to channel map")
							} else {
								event := EventWrapper{
									ModelDef:       nil,
									LoadState:      ShouldUnload,
									ShouldDownload: !modelWrapper.Success,
								}
								log.Println("Sending event", event)
								channel.EventChannel <- event
							}
						} else {
							if modelWrapper.Stale {
								channel, ok := p.ChannelMap[modelName]
								if !ok {
									log.Println("Need to add model", modelName)
									// TODO: Maybe have more workers per Channel?
									channel = p.AddModel(modelName, w.numWorkers)
								}
								event := EventWrapper{
									ModelDef:       modelWrapper.ModelDef,
									LoadState:      ShouldLoad,
									ShouldDownload: !modelWrapper.Success,
								}
								log.Println("Sending event", event)
								channel.EventChannel <- event
							}
						}
						return true
					})
					if err != nil {
						log.Println("Error in filepath walk", err)
					}
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
	err = watcher.Add(w.configDir)
	if err != nil {
		log.Fatal(err)
	}
	<-done
}
