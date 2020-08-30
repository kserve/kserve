package agent

import (
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	v1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"log"
	"path/filepath"
)

type OpType string

const (
	Add    OpType = "Add"
	Remove OpType = "Remove"
)

// sentinel to represent model absence
var remove v1.ModelSpec = v1.ModelSpec{}

type Puller struct {
	channelMap  map[string]ModelChannel
	completions chan string
	Downloader  Downloader
}

type ModelOp struct {
	ModelName string
	Op        OpType
	Spec      *v1.ModelSpec
}

func StartPuller(downloader Downloader, commands <-chan ModelOp) {
	puller := Puller{
		channelMap:  map[string]ModelChannel{},
		completions: make(chan string, 128),
		Downloader:  downloader,
	}
	go func() {
		var finished bool
		for {
			select {
			case modelOp, ok := <-commands:
				if !ok {
					finished = true
				} else {
					switch modelOp.Op {
					case Add:
						puller.enqueueModelOp(modelOp.ModelName, modelOp.Spec)
					case Remove:
						puller.enqueueModelOp(modelOp.ModelName, &remove)
					}
				}
			case completed := <-puller.completions:
				puller.modelOpComplete(completed, finished)
			}
		}
	}()
}

type ModelChannel struct {
	modelOps    chan *v1.ModelSpec
	opsInFlight int
}

func (p *Puller) enqueueModelOp(modelName string, modelSpec *v1.ModelSpec) {
	modelChan, ok := p.channelMap[modelName]
	if !ok {
		modelChan = ModelChannel{
			modelOps: make(chan *v1.ModelSpec, 8),
		}
		go p.modelProcessor(modelName, modelChan.modelOps)
		p.channelMap[modelName] = modelChan
	}
	modelChan.opsInFlight += 1
	modelChan.modelOps <- modelSpec
}

func (p *Puller) modelOpComplete(modelName string, closed bool) {
	modelChan, ok := p.channelMap[modelName]
	if ok {
		modelChan.opsInFlight -= 1
		if modelChan.opsInFlight == 0 {
			close(modelChan.modelOps)
			delete(p.channelMap, modelName)
			if closed && len(p.channelMap) == 0 {
				// this was the final completion, close the channel
				close(p.completions)
			}
		}
	} else {
		//TODO log warning, shouldn't happen
	}
}

func (p *Puller) modelProcessor(modelName string, ops <-chan *v1.ModelSpec) {
	log.Println("worker for", modelName, "is initialized")
	// TODO: Instead of going through each event, one-by-one, we need to drain and combine
	// this is important for handling Load --> Unload requests sent in tandem
	// Load --> Unload = 0 (cancel first load)
	// Load --> Unload --> Load = 1 Load (cancel second load?)
	for modelSpec := range ops {
		if modelSpec != &remove {
			// Load
			log.Println("Should download", modelSpec.StorageURI)
			err := p.Downloader.DownloadModel(modelName, modelSpec)
			if err != nil {
				log.Println("Download of model", modelName, "failed because: ", err)
			} else {
				// If there is an error, we will NOT send a request. As such, to know about errors, you will
				// need to call the error endpoint of the puller
				// TODO: Do request logic
				log.Println("Now doing load request for", modelName)
			}
		} else {
			// Unload
			// TODO: Do request logic
			log.Println("Now doing unload request for", modelName)
			// If there is an error, we will NOT do a delete... that could be problematic
			log.Println("Should unload", modelName)
			if err := storage.RemoveDir(filepath.Join(p.Downloader.ModelDir, modelName)); err != nil {
				log.Printf("failing to delete model directory: %v", err)
			}
		}
		p.completions <- modelName
	}
}
