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

package logger

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
)

const (
	CEInferenceRequest  = "org.kubeflow.serving.inference.request"
	CEInferenceResponse = "org.kubeflow.serving.inference.response"

	// cloud events extension attributes have to be lowercase alphanumeric
	//TODO: ideally request id would have its own header but make do with ce-id for now
	InferenceServiceAttr = "inferenceservicename"
	NamespaceAttr        = "namespace"
	ComponentAttr        = "component"
	// endpoint would be either default or canary
	EndpointAttr = "endpoint"

	LoggerWorkerQueueSize = 100
	CloudEventsIdHeader   = "Ce-Id"
)

// A buffered channel that we can send work requests on.
var WorkQueue = make(chan LogRequest, LoggerWorkerQueueSize)

func QueueLogRequest(req LogRequest) error {
	WorkQueue <- req
	return nil
}

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker can add itself to whenever it is done its
// work.
func NewWorker(id int, workerQueue chan chan LogRequest, logger *zap.SugaredLogger) Worker {
	// Create, and return the worker.
	return Worker{
		Log:         logger,
		ID:          id,
		Work:        make(chan LogRequest),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool),
		CeCtx:       cloudevents.WithEncodingBinary(context.Background()),
	}
}

type Worker struct {
	Log         *zap.SugaredLogger
	ID          int
	Work        chan LogRequest
	WorkerQueue chan chan LogRequest
	QuitChan    chan bool
	CeCtx       context.Context
}

func (w *Worker) sendCloudEvent(logReq LogRequest) error {
	t, err := cloudevents.NewHTTP(
		cloudevents.WithTarget(logReq.Url.String()),
	)

	if err != nil {
		return fmt.Errorf("while creating http transport: %w", err)
	}

	c, err := cloudevents.NewClient(t,
		cloudevents.WithTimeNow(),
	)
	if err != nil {
		return fmt.Errorf("while creating new cloudevents client: %w", err)
	}
	event := cloudevents.NewEvent(cloudevents.VersionV1)
	event.SetID(logReq.Id)
	event.SetType(logReq.ReqType)

	event.SetExtension(InferenceServiceAttr, logReq.InferenceService)
	event.SetExtension(NamespaceAttr, logReq.Namespace)
	event.SetExtension(ComponentAttr, logReq.Component)
	event.SetExtension(EndpointAttr, logReq.Endpoint)

	event.SetSource(logReq.SourceUri.String())
	if err := event.SetData(logReq.ContentType, *logReq.Bytes); err != nil {
		return fmt.Errorf("while setting cloudevents data: %w", err)
	}

	if result := c.Send(w.CeCtx, event); cloudevents.IsUndelivered(result) {
		return fmt.Errorf("while sending event: %w", result)
	}
	return nil
}

// This function "starts" the worker by starting a goroutine, that is
// an infinite "for-select" loop.
func (w *Worker) Start() {
	go func() {
		for {
			// Add ourselves into the worker queue.
			w.WorkerQueue <- w.Work

			select {
			case work := <-w.Work:
				// Receive a work request.
				w.Log.Infof("Received work request %d, url: %s, requestId: %s", w.ID, work.Url.String(), work.Id)

				if err := w.sendCloudEvent(work); err != nil {
					w.Log.Error(err, "Failed to send cloud event, url: %s", work.Url.String())
				}

			case <-w.QuitChan:
				// We have been asked to stop.
				fmt.Printf("worker %d stopping\n", w.ID)
				return
			}
		}
	}()
}

// Stop tells the worker to stop listening for work requests.
//
// Note that the worker will only stop *after* it has finished its work.
func (w *Worker) Stop() {
	go func() {
		w.QuitChan <- true
	}()
}
