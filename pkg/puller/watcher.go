package puller

import (
	"encoding/json"
	"github.com/fsnotify/fsnotify"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
)

var p *Puller

type Puller struct {
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
	p = New()
}

func New() *Puller {
	p := new(Puller)
	// TODO: This should probably be overridable
	p.fileExtension = ".json"
	return p
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
}
func WatchConfig(modelDir string) {
	log.Println("Entering watch")
	p.modelDir = modelDir
	p.WatchConfig()
}

func OnConfigChange(run func(in EventWrapper)) {
	log.Println("Applying onConfigChange")
	p.onConfigChange = run
}

func (p *Puller) WatchConfig() {
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
				if p.onConfigChange != nil {
					ext := filepath.Ext(event.Name)
					isEdit := event.Op&writeOrCreateMask != 0
					isRemove := event.Op&fsnotify.Remove != 0
					isValidFile := ext == p.fileExtension
					if isValidFile && (isEdit || isRemove) {
						fileName := strings.TrimSuffix(filepath.Base(event.Name), ext)
						if isRemove {
							p.onConfigChange(EventWrapper{
								LoadState: ShouldUnload,
								ModelName: fileName,
							})
						} else {
							file, _ := ioutil.ReadFile(filepath.Clean(event.Name))
							modelDef := ModelDefinition{}
							err := json.Unmarshal([]byte(file), &modelDef)
							if err != nil {
								log.Println("unable to marshall\n", err)
							} else {
								p.onConfigChange(EventWrapper{
									ModelDef: &modelDef,
									LoadState: ShouldLoad,
									ModelName: fileName,
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
	err = watcher.Add(p.modelDir)
	if err != nil {
		log.Fatal(err)
	}
	<-done
}
