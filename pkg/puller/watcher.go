package puller

import (
	"encoding/json"
	"github.com/fsnotify/fsnotify"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
)

var w *Watcher

type Watcher struct {
	modelDir string
	onConfigChange func(EventWrapper)
	fileExtension string
}

type ModelDefinition struct {
	// TODO: This needs to be defined by the ConfigMap PR
	StorageUri string  `json:"storageUri"`
	Framework string  `json:"framework"`
	Memory string  `json:"memory"`
}

func init() {
	w = NewWatcher()
}

func NewWatcher() *Watcher {
	w := new(Watcher)
	// TODO: This should probably be overridable
	w.fileExtension = ".json"
	return w
}

type State string

const (
	// State Related
	ShouldLoad State = "Load"
	ShouldUnload State = "Unload"

 	writeOrCreateMask = fsnotify.Write | fsnotify.Create
)

type EventWrapper struct {
	ModelDef *ModelDefinition
	LoadState State
	ModelName string
	DownloadRetries int
}
func WatchConfig(modelDir string) {
	log.Println("Entering watch")
	w.modelDir = modelDir
	w.WatchConfig()
}

func OnConfigChange(run func(in EventWrapper)) {
	log.Println("Applying onConfigChange")
	w.onConfigChange = run
}

func (w*Watcher) WatchConfig() {
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
				// we only care about the model file:
				// 1 - if the model file was modified or created
				// 2 - if the model file was removed as a result of deletion or renaming
				if w.onConfigChange != nil {
					ext := filepath.Ext(event.Name)
					isEdit := event.Op&writeOrCreateMask != 0
					isRemove := event.Op&fsnotify.Remove != 0
					isValidFile := ext == w.fileExtension
					if isValidFile && (isEdit || isRemove) {
						fileName := strings.TrimSuffix(filepath.Base(event.Name), ext)
						if isRemove {
							w.onConfigChange(EventWrapper{
								LoadState: ShouldUnload,
								ModelName: fileName,
								DownloadRetries: 0,
							})
						} else {
							file, _ := ioutil.ReadFile(filepath.Clean(event.Name))
							modelDef := ModelDefinition{}
							err := json.Unmarshal([]byte(file), &modelDef)
							if err != nil {
								log.Println("unable to marshall\n", err)
							} else {
								w.onConfigChange(EventWrapper{
									ModelDef: &modelDef,
									LoadState: ShouldLoad,
									ModelName: fileName,
									DownloadRetries: 0,
								})
							}
						}
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
	err = watcher.Add(w.modelDir)
	if err != nil {
		log.Fatal(err)
	}
	<-done
}
