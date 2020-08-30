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
)

type Watcher struct {
	configDir    string
	modelTracker map[string]modelWrapper
	ModelEvents  chan ModelOp
}

func NewWatcher(configDir string, modelDir string) Watcher {
	modelTracker, err := SyncModelDir(modelDir)
	if err != nil {
		log.Fatal(err)
	}
	return Watcher{
		configDir:    configDir,
		modelTracker: modelTracker,
		ModelEvents:  make(chan ModelOp),
	}
}

type modelWrapper struct {
	Spec  *v1beta1.ModelSpec
	stale bool
}

func (w *Watcher) Start() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()
	if err = watcher.Add(w.configDir); err != nil {
		log.Fatal(err)
	}
	// Add a first create event to the channel to force initial sync
	watcher.Events <- fsnotify.Event{
		Name: filepath.Join(w.configDir, "..data"),
		Op:   fsnotify.Create,
	}
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
					if err != nil {
						log.Println("Error in reading file", err)
					} else {
						modelConfigs := make(modelconfig.ModelConfigs, 0)
						err = json.Unmarshal([]byte(file), &modelConfigs)
						if err != nil {
							log.Println("unable to marshall for", event, "with error", err)
						} else {
							w.parseConfig(modelConfigs)
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
	log.Println("Watching", w.configDir)
	<-done
}

func (w *Watcher) parseConfig(modelConfigs modelconfig.ModelConfigs) {
	for _, modelConfig := range modelConfigs {
		name, spec := modelConfig.Name, modelConfig.Spec
		existing, exists := w.modelTracker[name]
		if !exists {
			// New - add
			w.modelTracker[name] = modelWrapper{Spec: &spec}
			w.modelAdded(name, &spec)
		} else if !cmp.Equal(spec, *existing.Spec) {
			existing.Spec, existing.stale = &spec, false
			// Changed - replace
			w.modelRemoved(name)
			w.modelAdded(name, &spec)
		}
	}
	for name, wrapper := range w.modelTracker {
		if wrapper.stale {
			// Gone - remove
			delete(w.modelTracker, name)
			w.modelRemoved(name)
		} else {
			wrapper.stale = true // reset for next iteration
		}
	}
}

func (w *Watcher) modelAdded(name string, spec *v1beta1.ModelSpec) {
	w.ModelEvents <- ModelOp{
		ModelName: name,
		Op:        Add,
		Spec:      spec,
	}
}

func (w *Watcher) modelRemoved(name string) {
	w.ModelEvents <- ModelOp{
		ModelName: name,
		Op:        Remove,
	}
}
