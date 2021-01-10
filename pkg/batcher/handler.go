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

package batcher

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/satori/go.uuid"
	"go.uber.org/zap"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"time"
)

const (
	SleepTime    = time.Microsecond * 100
	MaxBatchSize = 32
	MaxLatency   = 5000
)

type Request struct {
	Instances []interface{} `json:"instances"`
}

type Input struct {
	ContextInput *context.Context
	Path         string
	Instances    *[]interface{}
	ChannelOut   *chan Response
}

type InputInfo struct {
	ChannelOut *chan Response
	Index      []int
}

type Response struct {
	Message     string        `json:"message"`
	BatchID     string        `json:"batchId"`
	Predictions []interface{} `json:"predictions"`
}

type ResponseError struct {
	Message string `json:"message"`
}

type PredictionResponse struct {
	Predictions []interface{} `json:"predictions"`
}

type BatcherInfo struct {
	Path               string
	BatchID            string
	Request            *http.Request
	Instances          []interface{}
	PredictionResponse PredictionResponse
	ContextMap         map[*context.Context]InputInfo
	Start              time.Time
	Now                time.Time
	CurrentInputLen    int
}

func GetNowTime() time.Time {
	return time.Now().UTC()
}

func GenerateUUID() string {
	return uuid.NewV4().String()
}

func (batcherInfo *BatcherInfo) InitializeInfo() {
	batcherInfo.BatchID = ""
	batcherInfo.CurrentInputLen = 0
	batcherInfo.Instances = make([]interface{}, 0)
	batcherInfo.PredictionResponse = PredictionResponse{}
	batcherInfo.ContextMap = make(map[*context.Context]InputInfo)
	batcherInfo.Start = GetNowTime()
	batcherInfo.Now = batcherInfo.Start
}

func (handler *BatchHandler) batchPredict() {
	jsonStr, _ := json.Marshal(Request{
		handler.batcherInfo.Instances,
	})
	reader := bytes.NewReader(jsonStr)
	r := httptest.NewRequest("POST", handler.batcherInfo.Path, reader)
	rr := httptest.NewRecorder()
	handler.next.ServeHTTP(rr, r)
	responseBody := rr.Body.Bytes()
	if rr.Code != http.StatusOK {
		handler.log.Errorf("error response with code %v", rr)
		for _, v := range handler.batcherInfo.ContextMap {
			res := Response{
				Message:     string(responseBody),
				BatchID:     "",
				Predictions: nil,
			}
			*v.ChannelOut <- res
		}
	} else {
		handler.batcherInfo.BatchID = GenerateUUID()
		err := json.Unmarshal(responseBody, &handler.batcherInfo.PredictionResponse)
		if err != nil {
			for _, v := range handler.batcherInfo.ContextMap {
				res := Response{
					Message: err.Error(),
					BatchID: handler.batcherInfo.BatchID,
				}
				*v.ChannelOut <- res
			}
		} else {
			if len(handler.batcherInfo.PredictionResponse.Predictions) != len(handler.batcherInfo.Instances) {
				for _, v := range handler.batcherInfo.ContextMap {
					res := Response{
						Message: "size of prediction is not equal to the size of instances",
						BatchID: handler.batcherInfo.BatchID,
					}
					*v.ChannelOut <- res
				}
			} else {
				for _, v := range handler.batcherInfo.ContextMap {
					predictions := make([]interface{}, 0)
					for _, i := range v.Index {
						predictions = append(predictions, handler.batcherInfo.PredictionResponse.Predictions[i])
					}
					res := Response{
						Message:     "",
						BatchID:     handler.batcherInfo.BatchID,
						Predictions: predictions,
					}
					*v.ChannelOut <- res
				}
			}
		}
	}
	handler.batcherInfo.InitializeInfo()
}

func (handler *BatchHandler) batch() {
	handler.log.Info("Starting batch loop")
	for {
		select {
		case req := <-handler.channelIn:
			if len(handler.batcherInfo.Instances) == 0 {
				handler.batcherInfo.Start = GetNowTime()
			}
			handler.batcherInfo.Path = req.Path
			handler.batcherInfo.CurrentInputLen = len(handler.batcherInfo.Instances)
			handler.batcherInfo.Instances = append(handler.batcherInfo.Instances, *req.Instances...)
			var index = make([]int, 0)
			for i := 0; i < len(*req.Instances); i++ {
				index = append(index, handler.batcherInfo.CurrentInputLen+i)
			}
			handler.batcherInfo.ContextMap[req.ContextInput] = InputInfo{
				req.ChannelOut,
				index,
			}
			handler.batcherInfo.CurrentInputLen = len(handler.batcherInfo.Instances)
		case <-time.After(SleepTime):
		}
		handler.batcherInfo.Now = GetNowTime()
		if handler.batcherInfo.CurrentInputLen >= handler.MaxBatchSize ||
			(handler.batcherInfo.Now.Sub(handler.batcherInfo.Start).Milliseconds() >= int64(handler.MaxLatency) &&
				handler.batcherInfo.CurrentInputLen > 0) {
			handler.batchPredict()
		}
	}
}

func (handler *BatchHandler) Consume() {
	if handler.MaxBatchSize <= 0 {
		handler.MaxBatchSize = MaxBatchSize
	}
	if handler.MaxLatency <= 0 {
		handler.MaxLatency = MaxLatency
	}
	handler.batcherInfo.InitializeInfo()
	handler.batch()
}

type BatchHandler struct {
	next         http.Handler
	log          *zap.SugaredLogger
	channelIn    chan Input
	MaxBatchSize int
	MaxLatency   int
	batcherInfo  BatcherInfo
}

func New(maxBatchSize int, maxLatency int, handler http.Handler, logger *zap.SugaredLogger) *BatchHandler {
	batchHandler := BatchHandler{
		next:         handler,
		log:          logger,
		channelIn:    make(chan Input),
		MaxBatchSize: maxBatchSize,
		MaxLatency:   maxLatency,
	}
	go batchHandler.Consume()
	return &batchHandler
}

func (handler *BatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// only batch predict requests
	var predictVerb = regexp.MustCompile(`:predict$`)
	if !predictVerb.MatchString(r.URL.Path) {
		handler.next.ServeHTTP(w, r)
		return
	}
	var req Request
	var err error
	// Read Payload
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "can't Unmarshal body", http.StatusBadRequest)
		return
	}
	if len(req.Instances) == 0 {
		http.Error(w, "no instances in the request", http.StatusBadRequest)
		return
	}
	handler.log.Infof("serving request %s", r.URL.Path)
	var ctx = context.Background()
	var chl = make(chan Response)
	handler.channelIn <- Input{
		&ctx,
		r.URL.Path,
		&req.Instances,
		&chl,
	}

	response := <-chl
	close(chl)
	rspbytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	_, err = w.Write(rspbytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
