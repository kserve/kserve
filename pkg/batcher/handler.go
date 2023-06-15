/*
Copyright 2021 The KServe Authors.

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
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.uber.org/zap"
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
	ContextInput    *context.Context
	Path            string
	Instances       *[]interface{}
	ChannelOut      *chan Response
	ProtocolVersion string
}

type V2Input struct {
	ContextInput    *context.Context
	Path            string
	InferRequest    *InferRequest
	ChannelOut      *chan V2Response
	ProtocolVersion string
}

type InputInfo struct {
	ChannelOut *chan Response
	Index      []int
}

type Response struct {
	Message      string        `json:"message"`
	BatchID      string        `json:"batchId"`
	Predictions  []interface{} `json:"predictions"`
	ResponseCode int           `json:"response_code"`
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
	V2Instances        []InferInput
	V2ContextMap       map[*context.Context]V2InputInfo
	InferRequest       InferRequest
	InferResponse      InferResponse
	IsV2Initialized    bool
}

type InferRequest struct {
	ID         *string                 `json:"id"`
	Inputs     []InferInput            `json:"inputs"`
	Outputs    *[]interface{}          `json:"outputs"`
	Parameters *map[string]interface{} `json:"parameters"`
	ModelName  *string                 `json:"model_name"`
}

type InferInput struct {
	Name       string                  `json:"name"`
	Shape      []int                   `json:"shape"`
	DataType   string                  `json:"datatype"`
	Data       []interface{}           `json:"data"`
	Parameters *map[string]interface{} `json:"parameters"`
}

type V2InputInfo struct {
	ChannelOut   *chan V2Response
	Index        []int
	InferRequest InferRequest
}

type V2Response struct {
	Message      string         `json:"message"`
	BatchID      string         `json:"batchId"`
	Predictions  *InferResponse `json:"predictions"`
	ResponseCode int            `json:"response_code"`
}
type InferResponse struct {
	ModelName    string                  `json:"model_name"`
	ModelVerison *string                 `json:"model_version"`
	ID           string                  `json:"id"`
	Parameters   *map[string]interface{} `json:"parameters"`
	Outputs      []InferOutput           `json:"outputs"`
}

type InferOutput struct {
	Name       string                  `json:"name"`
	Shape      []int                   `json:"shape"`
	DataType   string                  `json:"datatype"`
	Data       []interface{}           `json:"data"`
	Parameters *map[string]interface{} `json:"parameters"`
}

func GetNowTime() time.Time {
	return time.Now().UTC()
}

func GenerateUUID() string {
	return uuid.Must(uuid.NewV4()).String()
}

func (batcherInfo *BatcherInfo) InitializeInfo() {
	batcherInfo.BatchID = ""
	batcherInfo.CurrentInputLen = 0
	batcherInfo.Instances = make([]interface{}, 0)
	batcherInfo.PredictionResponse = PredictionResponse{}
	batcherInfo.ContextMap = make(map[*context.Context]InputInfo)
	batcherInfo.Start = GetNowTime()
	batcherInfo.Now = batcherInfo.Start
	batcherInfo.IsV2Initialized = false
	batcherInfo.V2ContextMap = make(map[*context.Context]V2InputInfo)
}

func (handler *BatchHandler) batchPredict() {
	jsonStr, _ := json.Marshal(Request{
		handler.batcherInfo.Instances,
	})
	reader := bytes.NewReader(jsonStr)
	r := httptest.NewRequest(http.MethodPost, handler.batcherInfo.Path, reader)
	rr := httptest.NewRecorder()
	handler.next.ServeHTTP(rr, r)
	responseBody := rr.Body.Bytes()
	if rr.Code != http.StatusOK {
		handler.log.Errorf("error response with code %v", rr)
		for _, v := range handler.batcherInfo.ContextMap {
			res := Response{
				Message:      string(responseBody),
				BatchID:      "",
				Predictions:  nil,
				ResponseCode: rr.Code,
			}
			*v.ChannelOut <- res
		}
	} else {
		handler.batcherInfo.BatchID = GenerateUUID()
		err := json.Unmarshal(responseBody, &handler.batcherInfo.PredictionResponse)
		if err != nil {
			for _, v := range handler.batcherInfo.ContextMap {
				res := Response{
					Message:      err.Error(),
					BatchID:      handler.batcherInfo.BatchID,
					ResponseCode: http.StatusInternalServerError,
				}
				*v.ChannelOut <- res
			}
		} else {
			// As of now we are providing prediction inside list for make generic code(both v1, v2) in model prediction(predict api side)
			if len(handler.batcherInfo.PredictionResponse.Predictions) != len(handler.batcherInfo.Instances) {
				for _, v := range handler.batcherInfo.ContextMap {
					res := Response{
						Message:      "size of prediction is not equal to the size of instances",
						BatchID:      handler.batcherInfo.BatchID,
						ResponseCode: http.StatusInternalServerError,
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
						Message:      "",
						BatchID:      handler.batcherInfo.BatchID,
						Predictions:  predictions,
						ResponseCode: http.StatusOK,
					}
					*v.ChannelOut <- res
				}
			}
		}
	}
	handler.batcherInfo.InitializeInfo()
}

func (handler *BatchHandler) batch() {
	handler.log.Infof("Starting batch loop maxLatency:%d, maxBatchSize:%d", handler.MaxLatency, handler.MaxBatchSize)
	var protocolVersion string
	for {
		select {
		case req := <-handler.channelIn:
			protocolVersion = req.ProtocolVersion

			if len(handler.batcherInfo.Instances) == 0 {
				handler.batcherInfo.Start = GetNowTime()
			}
			handler.batcherInfo.Path = req.Path
			handler.batcherInfo.CurrentInputLen = len(handler.batcherInfo.Instances)
			handler.batcherInfo.Instances = append(handler.batcherInfo.Instances, *req.Instances...)
			index := make([]int, 0)
			for i := range len(*req.Instances) {
				index = append(index, handler.batcherInfo.CurrentInputLen+i)
			}
			handler.batcherInfo.ContextMap[req.ContextInput] = InputInfo{
				req.ChannelOut,
				index,
			}
			handler.batcherInfo.CurrentInputLen = len(handler.batcherInfo.Instances)

		case req := <-handler.V2channelIn:
			index := make([]int, 0)
			protocolVersion = req.ProtocolVersion

			if len(handler.batcherInfo.Instances) == 0 {
				handler.batcherInfo.Start = GetNowTime()
			}

			handler.batcherInfo.Path = req.Path
			handler.batcherInfo.CurrentInputLen = len(handler.batcherInfo.V2Instances)
			handler.batcherInfo.V2Instances = append(handler.batcherInfo.V2Instances, req.InferRequest.Inputs...)

			// for batching, we wrap all the inputs and make them as single request. So One time is enough to create v2 structure.
			if !handler.batcherInfo.IsV2Initialized {
				handler.batcherInfo.InferRequest = InferRequest{
					ID:         req.InferRequest.ID,
					Inputs:     []InferInput{},
					Outputs:    req.InferRequest.Outputs,
					Parameters: req.InferRequest.Parameters,
					ModelName:  req.InferRequest.ModelName,
				}
				handler.batcherInfo.IsV2Initialized = true
			}

			handler.batcherInfo.InferRequest.Inputs = handler.batcherInfo.V2Instances

			for i := 0; i < len(req.InferRequest.Inputs); i++ {
				index = append(index, handler.batcherInfo.CurrentInputLen+i)
			}
			handler.log.Infof("index is %v ", index)
			handler.batcherInfo.V2ContextMap[req.ContextInput] = V2InputInfo{
				req.ChannelOut,
				index,
				*req.InferRequest,
			}
			handler.batcherInfo.CurrentInputLen = len(handler.batcherInfo.V2Instances)
			handler.log.Infof("V2ContextMap is %v ", handler.batcherInfo.V2ContextMap[req.ContextInput])

		case <-time.After(SleepTime):
		}
		handler.batcherInfo.Now = GetNowTime()
		if handler.batcherInfo.CurrentInputLen >= handler.MaxBatchSize ||
			(handler.batcherInfo.Now.Sub(handler.batcherInfo.Start).Milliseconds() >= int64(handler.MaxLatency) &&
				handler.batcherInfo.CurrentInputLen > 0) {
			handler.log.Infof("Request batching for %s protocol version", protocolVersion)

			if protocolVersion == "v2" {
				handler.log.Infof("batch predict with size %d %s", len(handler.batcherInfo.V2Instances), handler.batcherInfo.Path)
				handler.v2BatchPredict()
			} else {
				handler.log.Infof("batch predict with size %d %s", len(handler.batcherInfo.Instances), handler.batcherInfo.Path)
				handler.batchPredict()
			}
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
	V2channelIn  chan V2Input
	MaxBatchSize int
	MaxLatency   int
	batcherInfo  BatcherInfo
}

func New(maxBatchSize int, maxLatency int, handler http.Handler, logger *zap.SugaredLogger) *BatchHandler {
	batchHandler := BatchHandler{
		next:         handler,
		log:          logger,
		channelIn:    make(chan Input),
		V2channelIn:  make(chan V2Input),
		MaxBatchSize: maxBatchSize,
		MaxLatency:   maxLatency,
	}
	go batchHandler.Consume()
	return &batchHandler
}

func (handler *BatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// only v1 and v2 batch predict requests allowed
	predictVerb := regexp.MustCompile(`(:predict|/infer)$`)
	if !predictVerb.MatchString(r.URL.Path) {
		handler.next.ServeHTTP(w, r)
		return
	}

	if regexp.MustCompile(`:predict$`).MatchString(r.URL.Path) {
		var req Request
		var err error
		// Read Payload
		body, err := io.ReadAll(r.Body)
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
		ctx := context.Background()
		chl := make(chan Response)
		handler.channelIn <- Input{
			&ctx,
			r.URL.Path,
			&req.Instances,
			&chl,
			"v1",
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

	if regexp.MustCompile(`/infer$`).MatchString(r.URL.Path) {
		var req InferRequest
		var err error
		// Read Payload
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "can't read body", http.StatusBadRequest)
			return
		}
		if err = json.Unmarshal(body, &req); err != nil {
			http.Error(w, "can't Unmarshal body", http.StatusBadRequest)
			return
		}
		if len(req.Inputs) == 0 {
			http.Error(w, "no instances in the request", http.StatusBadRequest)
			return
		}
		handler.log.Infof("serving request %s", r.URL.Path)
		ctx := context.Background()
		chl := make(chan V2Response)
		handler.V2channelIn <- V2Input{
			&ctx,
			r.URL.Path,
			&req,
			&chl,
			"v2",
		}

		response := <-chl
		close(chl)
		rspbytes, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		// Set response status code if not 200
		if response.ResponseCode != 200 {
			handler.log.Infof("set response code %v", response.ResponseCode)
			w.WriteHeader(response.ResponseCode)
		}

		_, err = w.Write(rspbytes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (handler *BatchHandler) v2BatchPredict() {
	jsonStr, _ := json.Marshal(handler.batcherInfo.InferRequest)
	reader := bytes.NewReader(jsonStr)
	r := httptest.NewRequest("POST", handler.batcherInfo.Path, reader)
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.next.ServeHTTP(rr, r)
	responseBody := rr.Body.Bytes()

	switch {
	case rr.Code != http.StatusOK:
		handler.log.Errorf("error response with code %v", rr)
		for _, v := range handler.batcherInfo.V2ContextMap {
			res := V2Response{
				Message:      string(responseBody),
				BatchID:      "",
				Predictions:  nil,
				ResponseCode: rr.Code,
			}
			*v.ChannelOut <- res
		}
	case len(handler.batcherInfo.InferResponse.Outputs) == len(handler.batcherInfo.V2Instances):
		handler.batcherInfo.BatchID = GenerateUUID()
		err := json.Unmarshal(responseBody, &handler.batcherInfo.InferResponse)
		if err != nil {
			for _, v := range handler.batcherInfo.V2ContextMap {
				res := V2Response{
					Message:      err.Error(),
					BatchID:      handler.batcherInfo.BatchID,
					ResponseCode: http.StatusInternalServerError,
				}
				*v.ChannelOut <- res
			}
		} else {
			v2BatchOutputs := handler.batcherInfo.InferResponse.Outputs
			handler.log.Infof("batchOutput length is   %v", len(v2BatchOutputs))
			for _, v := range handler.batcherInfo.V2ContextMap {
				predictions := []InferOutput{}
				for _, i := range v.Index {
					predictions = append(predictions, v2BatchOutputs[i])
				}
				id := handler.batcherInfo.InferResponse.ID
				if v.InferRequest.ID != nil {
					id = *v.InferRequest.ID
				}
				res := V2Response{
					Message: "",
					BatchID: handler.batcherInfo.BatchID,
					Predictions: &InferResponse{
						ModelName:    handler.batcherInfo.InferResponse.ModelName,
						ModelVerison: handler.batcherInfo.InferResponse.ModelVerison,
						ID:           id,
						Parameters:   handler.batcherInfo.InferResponse.Parameters,
						Outputs:      predictions,
					},
					ResponseCode: http.StatusOK,
				}
				*v.ChannelOut <- res
			}
		}
	case len(handler.batcherInfo.V2Instances) == handler.batcherInfo.InferResponse.Outputs[0].Shape[0]:
		outputData := handler.batcherInfo.InferResponse.Outputs[0]
		v2BatchOutputs := outputData.Data
		handler.log.Infof("batchOutput length is   %v", len(v2BatchOutputs))
		for _, v := range handler.batcherInfo.V2ContextMap {
			predictions := []InferOutput{}
			for _, i := range v.Index {
				InferOutput := InferOutput{
					Name:     outputData.Name + strconv.Itoa(i),
					Shape:    []int{},
					DataType: outputData.DataType,
					Data:     []interface{}{},
				}
				InferOutput.Data = append(InferOutput.Data, v2BatchOutputs[i])
				InferOutput.Shape = append(InferOutput.Shape, len(InferOutput.Data))
				predictions = append(predictions, InferOutput)
			}
			id := handler.batcherInfo.InferResponse.ID
			if v.InferRequest.ID != nil {
				id = *v.InferRequest.ID
			}
			res := V2Response{
				Message: "",
				BatchID: handler.batcherInfo.BatchID,
				Predictions: &InferResponse{
					ModelName:    handler.batcherInfo.InferResponse.ModelName,
					ModelVerison: handler.batcherInfo.InferResponse.ModelVerison,
					ID:           id,
					Parameters:   handler.batcherInfo.InferResponse.Parameters,
					Outputs:      predictions,
				},
				ResponseCode: http.StatusOK,
			}
			*v.ChannelOut <- res
		}
	default:
		for _, v := range handler.batcherInfo.V2ContextMap {
			res := V2Response{
				Message:      "size of prediction is not equal to the size of instances",
				BatchID:      handler.batcherInfo.BatchID,
				ResponseCode: http.StatusInternalServerError,
			}
			*v.ChannelOut <- res
		}
	}

	handler.batcherInfo.InitializeInfo()
}
