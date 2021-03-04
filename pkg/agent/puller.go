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
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"time"
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
	logger      *zap.SugaredLogger
}

type ModelOp struct {
	ModelName string
	Op        OpType
	Spec      *v1.ModelSpec
}

func StartPullerAndProcessModels(downloader Downloader, commands <-chan ModelOp, modelTracker map[string]modelWrapper, logger *zap.SugaredLogger) {
	puller := Puller{
		channelMap:  make(map[string]*ModelChannel),
		completions: make(chan *ModelOp, 4),
		opStats:     make(map[string]map[OpType]int),
		Downloader:  downloader,
		logger:      logger,
	}
	go puller.processCommands(commands)
	for modelName, _ := range modelTracker {
		for puller.opStats[modelName][Add] != 1 {
			time.Sleep(1 * time.Second)
		}
	}
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
		p.logger.Infof("completion event for model %s, in flight ops %d", modelOp.ModelName, modelChan.opsInFlight)
	} else {
		p.logger.Infof("Op completion event did not find channel for %s", modelOp.ModelName)
	}
}

func (p *Puller) modelProcessor(modelName string, ops <-chan *ModelOp) {
	p.logger.Infof("Worker is started for %s", modelName)
	// TODO: Instead of going through each event, one-by-one, we need to drain and combine
	// this is important for handling Load --> Unload requests sent in tandem
	// Load --> Unload = 0 (cancel first load)
	// Load --> Unload --> Load = 1 Load (cancel second load?)
	for modelOp := range ops {
		switch modelOp.Op {
		case Add:
			p.logger.Infof("Downloading model from %s", modelOp.Spec.StorageURI)
			err := p.Downloader.DownloadModel(modelName, modelOp.Spec)
			if err != nil {
				// If there is an error, we will NOT send a request. As such, to know about errors, you will
				// need to call the error endpoint of the puller
				p.logger.Errorf("Failed to download model %s with err %v", modelName, err)
			} else {
				// Load the model onto the model server
				resp, err := http.Post(fmt.Sprintf("http://localhost:8080/v2/repository/models/%s/load", modelName),
					"application/json",
					bytes.NewBufferString("{}"))
				if err != nil {
					// handle error
					p.logger.Errorf("Failed to Load model %s", modelName)
				} else {
					defer resp.Body.Close()
					if resp.StatusCode == 200 {
						p.logger.Infof("Successfully loaded model %s", modelName)
					} else {
						body, err := ioutil.ReadAll(resp.Body)
						if err == nil {
							p.logger.Infof("Failed to load model %s with status [%d] and resp:%v", modelName, resp.StatusCode, body)
						}
					}
				}
			}
		case Remove:
			p.logger.Infof("unloading model %s", modelName)
			// If there is an error, we will NOT do a delete... that could be problematic
			if err := storage.RemoveDir(filepath.Join(p.Downloader.ModelDir, modelName)); err != nil {
				p.logger.Error(err, "failing to delete model directory")
			} else {
				// unload model from model server
				resp, err := http.Post(fmt.Sprintf("http://localhost:8080/v2/repository/models/%s/unload", modelName),
					"application/json",
					bytes.NewBufferString("{}"))
				if err != nil {
					// handle error
					p.logger.Errorf("Failed to Unload model %s", modelName)
				} else {
					defer resp.Body.Close()
					if resp.StatusCode == 200 {
						p.logger.Infof("Successfully unloaded model %s", modelName)
					} else {
						body, err := ioutil.ReadAll(resp.Body)
						if err == nil {
							p.logger.Infof("Failed to unload model %s with status [%d] and resp:%v", modelName, resp.StatusCode, body)
						}
					}
				}
			}
		}
		p.completions <- modelOp
	}
}
