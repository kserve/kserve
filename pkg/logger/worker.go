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

package logger

import (
	"context"
	"fmt"
	"github.com/cloudevents/sdk-go"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/transport"
	"github.com/go-logr/logr"
	"net/http"
	"time"
)

const (
	CEInferenceRequest  = "org.kubeflow.serving.inference.request"
	CEInferenceResponse = "org.kubeflow.serving.inference.response"

	// cloud events extension attributes have to be lowercase alphanumeric
	//TODO: ideally request id would have its own header but make do with ce-id for now
	InferenceServiceAttr = "inferenceservicename"
	NamespaceAttr        = "namespace"
	//endpoint would be either default or canary
	EndpointAttr = "endpoint"
)

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker can add itself to whenever it is done its
// work.
func NewWorker(id int, workerQueue chan chan LogRequest, log logr.Logger) Worker {
	// Create, and return the worker.
	return Worker{
		Log:         log,
		ID:          id,
		Work:        make(chan LogRequest),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool),
		Client: http.Client{
			Timeout: 60 * time.Second,
		},
		CeCtx: cloudevents.ContextWithEncoding(context.Background(), cloudevents.Binary),
	}
}

type Worker struct {
	Log         logr.Logger
	ID          int
	Work        chan LogRequest
	WorkerQueue chan chan LogRequest
	QuitChan    chan bool
	Client      http.Client
	CeCtx       context.Context
	CeTransport transport.Transport
}

func (W *Worker) sendCloudEvent(logReq LogRequest) error {

	t, err := cloudevents.NewHTTPTransport(
		cloudevents.WithTarget(logReq.Url.String()),
		cloudevents.WithEncoding(cloudevents.HTTPBinaryV1),
	)

	if err != nil {
		return fmt.Errorf("while creating http transport: %s", err)
	}
	c, err := cloudevents.NewClient(t,
		cloudevents.WithTimeNow(),
	)
	if err != nil {
		return fmt.Errorf("while creating new cloudevents client: %s", err)
	}
	event := cloudevents.NewEvent(cloudevents.VersionV1)
	event.SetID(logReq.Id)
	if logReq.ReqType == InferenceRequest {
		event.SetType(CEInferenceRequest)
	} else {
		event.SetType(CEInferenceResponse)
	}

	event.SetExtension(InferenceServiceAttr, logReq.InferenceService)
	event.SetExtension(NamespaceAttr, logReq.Namespace)
	event.SetExtension(EndpointAttr, logReq.Endpoint)

	event.SetSource(logReq.SourceUri.String())
	event.SetDataContentType(logReq.ContentType)
	if err := event.SetData(*logReq.Bytes); err != nil {
		return fmt.Errorf("while setting cloudevents data: %s", err)
	}

	if _, _, err := c.Send(W.CeCtx, event); err != nil {
		return fmt.Errorf("while sending event: %s", err)
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
				w.Log.Info("Received work request", "workerId", w.ID, "url", work.Url.String(),
					"requestId", work.Id)

				if err := w.sendCloudEvent(work); err != nil {
					w.Log.Error(err, "Failed to send log", "URL", work.Url.String())
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
