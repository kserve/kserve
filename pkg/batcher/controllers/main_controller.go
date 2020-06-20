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

package controllers

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
	"errors"
	"context"
	"encoding/json"
	"github.com/astaxie/beego"
	"github.com/go-logr/logr"
	"github.com/satori/go.uuid"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	SleepTime    = time.Microsecond * 100
	MaxBatchSize = 32
	MaxLatency   = 5000
)

var (
	log logr.Logger
	channelIn = make(chan Input)
	batcherInfo BatcherInfo
	mutex sync.Mutex
)

type MainController struct {
	beego.Controller
}

type Request struct {
	Instances []interface{} `json:"instances"`
}

type Input struct {
	ContextInput *context.Context
	Instances *[]interface{}
	ChannelOut *chan Response
}

type InputInfo struct {
	ChannelOut *chan Response
	Index []int
}

type Response struct {
	Message string `json:"message"`
	BatchID string `json:"batchId"`
	Predictions []interface{} `json:"predictions"`
}

type ResponseError struct {
	Message string `json:"message"`
}

type Predictions struct {
	Predictions []interface{} `json:"predictions"`
}

type BatcherInfo struct {
	MaxBatchSize int
	MaxLatency int
	Port string
	SvcHost string
	SvcPort string
	Timeout time.Duration
	Path string
	ContentType string
	BatchID string
	Instances []interface{}
	Predictions Predictions
	Info map[*context.Context] InputInfo
	Start time.Time
	Now time.Time
	CurrentInputLen int
}

func Config(port string, svcHost string, svcPort string,
	maxBatchSize int, maxLatency int, timeout int) {
	batcherInfo.Port = port
	batcherInfo.SvcHost = svcHost
	batcherInfo.SvcPort = svcPort
	batcherInfo.MaxBatchSize = maxBatchSize
	batcherInfo.MaxLatency = maxLatency
	batcherInfo.Timeout = time.Duration(timeout) * time.Second
}

func GetNowTime() time.Time {
	return time.Now().UTC()
}

func GenerateUUID() string {
	return uuid.NewV4().String()
}

func (batcherInfo *BatcherInfo) CallService() *string {
	var errStr string
	url := fmt.Sprintf("http://%s:%s%s", batcherInfo.SvcHost, batcherInfo.SvcPort, batcherInfo.Path)
	jsonStr, _ := json.Marshal(Request{
		batcherInfo.Instances,
	})
	log.Info("CallService", "URL", url)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		errStr = fmt.Sprintf("NewRequest create fail: %v", err)
		return &errStr
	}
	req.Header.Add("Content-Type", batcherInfo.ContentType)
	defer req.Body.Close()
	client := &http.Client{Timeout: batcherInfo.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		errStr = fmt.Sprintf("NewRequest send fail: %v", err)
		return &errStr
	}
	defer resp.Body.Close()
	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errStr = fmt.Sprintf("Response read fail: %v", err)
		return &errStr
	}
	err = json.Unmarshal(result, &batcherInfo.Predictions)
	if err != nil {
		errStr = fmt.Sprintf("Response unmarshal fail: %v, %s", err, string(result))
		return &errStr
	} else {
		log.Info("CallService", "Results", string(result))
	}
	return nil
}

func (batcherInfo *BatcherInfo) InitializeInfo() {
	batcherInfo.BatchID = ""
	batcherInfo.CurrentInputLen = 0
	batcherInfo.Instances = make([]interface{}, 0)
	batcherInfo.Predictions = Predictions{}
	batcherInfo.Info = make(map[*context.Context] InputInfo)
	batcherInfo.Start = GetNowTime()
	batcherInfo.Now = batcherInfo.Start
}

func (batcherInfo *BatcherInfo) BatchPredict() {
	err := batcherInfo.CallService()
	if err != nil {
		log.Error(errors.New(*err), "")
		for _, v := range batcherInfo.Info {
			res := Response{
				Message: *err,
				BatchID: "",
				Predictions: nil,
			}
			*v.ChannelOut <- res
		}
	} else {
		batcherInfo.BatchID = GenerateUUID()
		for _, v := range batcherInfo.Info {
			predictions := make([]interface{}, 0)
			for _, index := range v.Index {
				predictions = append(predictions, batcherInfo.Predictions.Predictions[index])
			}
			res := Response{
				Message: "",
				BatchID: batcherInfo.BatchID,
				Predictions: predictions,
			}
			if jsonStr, err := json.Marshal(res); err == nil {
				log.Info("BatchPredict", "Results", string(jsonStr))
			} else {
				log.Error(errors.New("marshal response fail"), "")
			}
			*v.ChannelOut <- res
		}
	}
	batcherInfo.InitializeInfo()
}

func (batcherInfo *BatcherInfo) Batcher() {
	for {
		select {
		case req := <- channelIn:
			if len(batcherInfo.Instances) == 0 {
				batcherInfo.Start = GetNowTime()
			}
			batcherInfo.CurrentInputLen = len(batcherInfo.Instances)
			batcherInfo.Instances = append(batcherInfo.Instances, *req.Instances...)
			var index = make([]int, 0)
			for i := 0; i < len(*req.Instances); i++ {
				index = append(index, batcherInfo.CurrentInputLen + i)
			}
			batcherInfo.Info[req.ContextInput] = InputInfo{
				req.ChannelOut,
				index,
			}
			batcherInfo.CurrentInputLen = len(batcherInfo.Instances)
		case <- time.After(SleepTime):
		}
		batcherInfo.Now = GetNowTime()
		if batcherInfo.CurrentInputLen >= batcherInfo.MaxBatchSize ||
			(batcherInfo.Now.Sub(batcherInfo.Start).Milliseconds() >= int64(batcherInfo.MaxLatency) &&
				batcherInfo.CurrentInputLen > 0) {
			batcherInfo.BatchPredict()
		}
	}
}

func (batcherInfo *BatcherInfo)  Consume() {
	log.Info("Start Consume")
	if batcherInfo.MaxBatchSize <= 0 {
		batcherInfo.MaxBatchSize = MaxBatchSize
	}
	if batcherInfo.MaxLatency <= 0 {
		batcherInfo.MaxLatency = MaxLatency
	}
	batcherInfo.InitializeInfo()
	batcherInfo.Batcher()
}

func (c *MainController) Post() {
	var req Request
	var err error
	log.Info("Post", "Request Body Len", len(string(c.Ctx.Input.RequestBody)))
	if err = json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		log.Error(errors.New("unmarshal fail"), "")
		c.Abort("400")
	}
	if len(req.Instances) == 0 {
		log.Error(errors.New("instances empty"), "")
		c.Abort("400")
	}

	if batcherInfo.Path == "" {
		mutex.Lock()
		if batcherInfo.Path == "" {
			log.Info("Post", "Request Path", c.Ctx.Input.URL())
			batcherInfo.Path = c.Ctx.Input.URL()
			batcherInfo.ContentType = c.Ctx.Input.Header("Content-Type")
		}
		mutex.Unlock()
	}

	var ctx = context.Background()
	var chl = make(chan Response)
	channelIn <- Input {
		&ctx,
		&req.Instances,
		&chl,
	}

	response := <- chl
	close(chl)

	c.Data["json"] = &response
	c.ServeJSON()
}

func init() {
	logf.SetLogger(logf.ZapLogger(false))
	log = logf.Log.WithName("entrypoint")
	go batcherInfo.Consume()
}
