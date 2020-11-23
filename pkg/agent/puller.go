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
	"bytes"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	v1 "github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"io/ioutil"
	"net/http"
	"path/filepath"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

type OpType string

const (
	Add    OpType = "Add"
	Remove OpType = "Remove"
)

type Puller struct {
	channelMap  map[string]*ModelChannel
	completions chan *ModelOp
	opStats     map[string]map[OpType]int
	Downloader  Downloader
}

type ModelOp struct {
	ModelName string
	Op        OpType
	Spec      *v1.ModelSpec
}

func StartPuller(downloader Downloader, commands <-chan ModelOp) {
	puller := Puller{
		channelMap:  make(map[string]*ModelChannel),
		completions: make(chan *ModelOp, 4),
		opStats:     make(map[string]map[OpType]int),
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
	log.V(10).Info("enqueue", "modelop", modelOp)
	modelChan, ok := p.channelMap[modelOp.ModelName]
	if !ok {
		modelChan = &ModelChannel{
			modelOps: make(chan *ModelOp, 8),
		}
		go p.modelProcessor(modelOp.ModelName, modelChan.modelOps)
		p.channelMap[modelOp.ModelName] = modelChan
	}
	modelChan.opsInFlight += 1
	modelChan.modelOps <- modelOp
}

func (p *Puller) modelOpComplete(modelOp *ModelOp, closed bool) {
	log := logf.Log.WithName("modelOnComplete")
	if opMap, ok := p.opStats[modelOp.ModelName]; ok {
		opMap[modelOp.Op] += 1
	} else {
		p.opStats[modelOp.ModelName] = make(map[OpType]int)
		p.opStats[modelOp.ModelName][modelOp.Op] = 1
	}
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
		log.Info("completion event for model", "modelName", modelOp.ModelName, "inFlight", modelChan.opsInFlight)
	} else {
		log.Info("Op completion event did not find channel for", "modelName", modelOp.ModelName)
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
			log.Info("Downloading model", "storageUri", modelOp.Spec.StorageURI)
			err := p.Downloader.DownloadModel(modelName, modelOp.Spec)
			if err != nil {
				// If there is an error, we will NOT send a request. As such, to know about errors, you will
				// need to call the error endpoint of the puller
				log.Error(err, "Fails to download model", "modelName", modelName)
			} else {
				// Load the model onto the model server
				resp, err := http.Post(fmt.Sprintf("http://localhost:8080/v2/repository/models/%s/load", modelName),
					"application/json",
					bytes.NewBufferString("{}"))
				if err != nil {
					// handle error
					log.Error(err, "Failed to Load model", "modelName", modelName)
				} else {
					defer resp.Body.Close()
					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						log.Info("Loaded model", "modelName", modelName, "resp", body)
					}
				}
			}
		case Remove:
			log.Info("unloading model", "modelName", modelName)
			// If there is an error, we will NOT do a delete... that could be problematic
			if err := storage.RemoveDir(filepath.Join(p.Downloader.ModelDir, modelName)); err != nil {
				log.Error(err, "failing to delete model directory")
			} else {
				// unload model from model server
				resp, err := http.Post(fmt.Sprintf("http://localhost:8080/v2/repository/models/%s/unload", modelName),
					"application/json",
					bytes.NewBufferString("{}"))
				if err != nil {
					// handle error
					log.Error(err, "Failed to Unload model", "modelName", modelName)
				} else {
					defer resp.Body.Close()
					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						log.Info("Unloaded model", "modelName", modelName, "resp", body)
					}
				}
			}
		}
		p.completions <- modelOp
	}
}
