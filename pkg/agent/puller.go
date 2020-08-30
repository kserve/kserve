/*
Copyright 2020 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	v1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"path/filepath"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

type OpType string

const (
	Add    OpType = "Add"
	Remove OpType = "Remove"
)

type Puller struct {
	channelMap  map[string]ModelChannel
	completions chan *ModelOp
	Downloader  Downloader
}

type ModelOp struct {
	ModelName string
	Op        OpType
	Spec      *v1.ModelSpec
}

func StartPuller(downloader Downloader, commands <-chan ModelOp) {
	puller := Puller{
		channelMap:  make(map[string]ModelChannel),
		completions: make(chan *ModelOp, 4),
		Downloader:  downloader,
	}
	go puller.processCommands(commands)
}

func (p *Puller) processCommands(commands <-chan ModelOp) {
	// channelMap accessed only by this goroutine
	for {
		select {
		case modelOp, ok := <-commands:
			if ok {
				p.enqueueModelOp(&modelOp)
			} else {
				commands = nil
			}
		case completed := <-p.completions:
			p.modelOpComplete(completed, commands == nil)
		}
	}
}

type ModelChannel struct {
	modelOps    chan *ModelOp
	opsInFlight int
}

func (p *Puller) enqueueModelOp(modelOp *ModelOp) {
	modelChan, ok := p.channelMap[modelOp.ModelName]
	if !ok {
		modelChan = ModelChannel{
			modelOps: make(chan *ModelOp, 8),
		}
		go p.modelProcessor(modelOp.ModelName, modelChan.modelOps)
		p.channelMap[modelOp.ModelName] = modelChan
	}
	modelChan.opsInFlight += 1
	modelChan.modelOps <- modelOp
}

func (p *Puller) modelOpComplete(modelOp *ModelOp, closed bool) {
	log := logf.Log.WithName("modelOp")
	modelChan, ok := p.channelMap[modelOp.ModelName]
	if ok {
		modelChan.opsInFlight -= 1
		if modelChan.opsInFlight == 0 {
			close(modelChan.modelOps)
			delete(p.channelMap, modelOp.ModelName)
			if closed && len(p.channelMap) == 0 {
				// this was the final completion, close the channel
				close(p.completions)
			}
		}
	} else {
		log.Info("Op completion event for model", modelOp.ModelName, "not found in channelMap")
	}
}

func (p *Puller) modelProcessor(modelName string, ops <-chan *ModelOp) {
	log := logf.Log.WithName("modelProcessor")
	log.Info("worker is started for", "model", modelName)
	// TODO: Instead of going through each event, one-by-one, we need to drain and combine
	// this is important for handling Load --> Unload requests sent in tandem
	// Load --> Unload = 0 (cancel first load)
	// Load --> Unload --> Load = 1 Load (cancel second load?)
	for modelOp := range ops {
		switch modelOp.Op {
		case Add:
			// Load
			log.Info("Should download", modelOp.Spec.StorageURI)
			err := p.Downloader.DownloadModel(modelName, modelOp.Spec)
			if err != nil {
				log.Info("Download of model", modelName, "failed because: ", err)
			} else {
				// If there is an error, we will NOT send a request. As such, to know about errors, you will
				// need to call the error endpoint of the puller
				// TODO: Do request logic
				log.Info("Now doing load request for", modelName)
			}
		case Remove:
			// Unload
			// TODO: Do request logic
			log.Info("Now doing unload request for", modelName)
			// If there is an error, we will NOT do a delete... that could be problematic
			log.Info("Should unload", modelName)
			if err := storage.RemoveDir(filepath.Join(p.Downloader.ModelDir, modelName)); err != nil {
				log.Info("failing to delete model directory: %v", err)
			}
		}
		p.completions <- modelOp
	}
}
