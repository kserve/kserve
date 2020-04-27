/*
Copyright 2019 kubeflow.org.

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
	"github.com/go-logr/logr"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
	"errors"
	"context"
	"encoding/json"
	"github.com/astaxie/beego"
	"github.com/satori/go.uuid"
)

const (
	SleepTime    = time.Microsecond * 100
	MaxBatchsize = 32
	MaxLatency   = 1.0
)

var (
	channelIn = make(chan Input)
	batcherInfo BatcherInfo
	batcherHandler *BatcherHandler = nil
	mutex sync.Mutex
)

type MainController struct {
	beego.Controller
}

type Request struct {
	Instances []interface{} `json:"instances"`
}

type Input struct {
	ContextInput *context.Context `json:"contextInput"`
	Instances *[]interface{} `json:"instances"`
	ChannelOut *chan Response `json:"channelOut"`
}

type InputInfo struct {
	ChannelOut *chan Response `json:"channelOut"`
	Index []int `json:"index"`
}

type Response struct {
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
	MaxBatchsize int `json:"maxBatchsize"`
	MaxLatency float64 `json:"maxLatency"`
	BatchID string `json:"batchId"`
	Instances []interface{} `json:"instances"`
	Predictions Predictions `json:"predictions"`
	Info map[*context.Context] InputInfo `json:"info"`
	Start time.Time `json:"start"`
	Now time.Time `json:"now"`
	CurrentInputLen int `json:"currentInputLen"`
}

type BatcherHandler struct {
	Log              logr.Logger
	Port             string
	SvcHost          string
	SvcPort          string
	MaxBatchsize     int
	MaxLatency       float64
	Path             string
	ContentTpye      string
}

func New(log logr.Logger, port string, svcHost string, svcPort string,
	maxBatchsize int, maxLatency float64) {
	batcherHandler = &BatcherHandler{
		Log:              log,
		Port:             port,
		SvcHost:          svcHost,
		SvcPort:          svcPort,
		MaxBatchsize:     maxBatchsize,
		MaxLatency:       maxLatency,
		Path:             "",
		ContentTpye:      "",
	}
}

func GetNowTime() time.Time {
	return time.Now().UTC()
}

func GenerateUUID() string {
	if uuidStr, err := uuid.NewV4(); err == nil {
		return uuidStr.String()
	}
	return ""
}

func CallService() {
	var errStr string
	url := fmt.Sprintf("http://%s:%s%s", batcherHandler.SvcHost, batcherHandler.SvcPort, batcherHandler.Path)
	jsonStr, _ := json.Marshal(Request{
		batcherInfo.Instances,
	})
	batcherHandler.Log.Info("CallService", "URL", url)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		errStr = fmt.Sprintf("NewRequest create fail: %v", err)
		panic(errStr)
	}
	req.Header.Add("Content-Type", batcherHandler.ContentTpye)
	defer req.Body.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		errStr = fmt.Sprintf("NewRequest send fail: %v", err)
		panic(errStr)
	}
	defer resp.Body.Close()
	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errStr = fmt.Sprintf("Response read fail: %v", err)
		panic(errStr)
	}
	err = json.Unmarshal(result, &batcherInfo.Predictions)
	if err != nil {
		errStr = fmt.Sprintf("Response unmarshal fail: %v, %s", err, string(result))
		panic(errStr)
	}
	if jsonStr, err := json.Marshal(batcherInfo.Predictions); err == nil {
		batcherHandler.Log.Info("CallService", "Results", string(jsonStr))
	}
}

func BatchPredict(batcherInfo *BatcherInfo) {
	defer func() {
		if r := recover(); r != nil {
			var err error
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				err = x
			default:
				err = errors.New("predict fail")
			}
			if err != nil {
				batcherHandler.Log.Error(err, "")
			}
			for _, v := range batcherInfo.Info {
				res := Response{
					BatchID: "",
					Predictions: nil,
				}
				*v.ChannelOut <- res
			}
		}
		batcherInfo.BatchID = ""
		batcherInfo.Instances = make([]interface{}, 0)
		batcherInfo.Predictions = Predictions{}
		batcherInfo.Info = make(map[*context.Context] InputInfo)
		batcherInfo.CurrentInputLen = 0
	}()

	CallService()

	batcherInfo.BatchID = GenerateUUID()

	for _, v := range batcherInfo.Info {
		predictions := make([]interface{}, 0)
		for _, index := range v.Index {
			predictions = append(predictions, batcherInfo.Predictions.Predictions[index])
		}
		res := Response{
			BatchID: batcherInfo.BatchID,
			Predictions: predictions,
		}
		if jsonStr, err := json.Marshal(res); err == nil {
			batcherHandler.Log.Info("BatchPredict", "Results", string(jsonStr))
		}
		*v.ChannelOut <- res
	}
}

func Batcher(batcherInfo *BatcherInfo) {
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
	default:
		time.Sleep(SleepTime)
	}
	batcherInfo.Now = GetNowTime()
	if batcherInfo.CurrentInputLen >= batcherInfo.MaxBatchsize ||
		(float64(batcherInfo.Now.Sub(batcherInfo.Start).Milliseconds()) >= batcherInfo.MaxLatency &&
			batcherInfo.CurrentInputLen > 0) {
		BatchPredict(batcherInfo)
	}
}

func Consume() {
	batcherInfo.MaxBatchsize = MaxBatchsize
	batcherInfo.MaxLatency = MaxLatency
	if batcherHandler != nil {
		if batcherHandler.MaxBatchsize > 0 {
			batcherInfo.MaxBatchsize = batcherHandler.MaxBatchsize
		}
		if batcherHandler.MaxLatency > 0.0 {
			batcherInfo.MaxLatency = batcherHandler.MaxLatency
		}
	}
	batcherInfo.Info = make(map[*context.Context] InputInfo)
	batcherInfo.Start = GetNowTime()
	batcherInfo.Now = batcherInfo.Start
	batcherInfo.CurrentInputLen = 0
	for true {
		Batcher(&batcherInfo)
	}
}

func (c *MainController) Post() {
	var req Request
	var err error
	batcherHandler.Log.Info("Post", "Request Body Len", len(string(c.Ctx.Input.RequestBody)))
	if err = json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		batcherHandler.Log.Error(errors.New("unmarshal fail"), "")
		c.Abort("400")
	}
	if len(req.Instances) == 0 {
		batcherHandler.Log.Error(errors.New("instances empty"), "")
		c.Abort("400")
	}

	if batcherHandler != nil && batcherHandler.Path == "" {
		mutex.Lock()
		if batcherHandler != nil && batcherHandler.Path == "" {
			batcherHandler.Log.Info("Post", "Request Path", c.Ctx.Input.URL())
			batcherHandler.Path = c.Ctx.Input.URL()
			batcherHandler.ContentTpye = c.Ctx.Input.Header("Content-Type")
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

	if response.Predictions == nil || response.BatchID == "" {
		c.Data["json"] = &ResponseError{
			Message: "predict fail",
		}
	} else {
		c.Data["json"] = &response
	}
	c.ServeJSON()
}

func init() {
	go Consume()
}
