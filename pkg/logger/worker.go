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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

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
	MetadataAttr         = "metadata"
	// endpoint would be either default or canary
	EndpointAttr = "endpoint"

	LoggerWorkerQueueSize = 100
	LoggerCaCertMountPath = "/etc/tls/logger"
	CloudEventsIdHeader   = "Ce-Id"
)

// A buffered channel that we can send work requests on.
var WorkQueue = make(chan []LogRequest, LoggerWorkerQueueSize)

// QueueLogRequest enqueues a single LogRequest.
func QueueLogRequest(req LogRequest) error {
	WorkQueue <- []LogRequest{req}
	return nil
}

// QueueCombinedLogRequest queues a combined request and response to send together.
func QueueCombinedLogRequest(req LogRequest, resp LogRequest) error {
	WorkQueue <- []LogRequest{req, resp}
	return nil
}

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker can add itself to whenever it is done its
// work.
func NewWorker(id int, workerQueue chan chan []LogRequest, logger *zap.SugaredLogger) Worker {
	// Create, and return the worker.
	return Worker{
		Log:         logger,
		ID:          id,
		Work:        make(chan []LogRequest),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool),
		CeCtx:       cloudevents.WithEncodingBinary(context.Background()),
	}
}

type Worker struct {
	Log         *zap.SugaredLogger
	ID          int
	Work        chan []LogRequest
	WorkerQueue chan chan []LogRequest
	QuitChan    chan bool
	CeCtx       context.Context
}

// prepareEvent creates a cloudevent from the LogRequest setting the extensions
// body and metadata. Event defaulter functions should be applied here since
// a cloudevent client is not used.
func (w *Worker) prepareEvent(logReq LogRequest) (cloudevents.Event, error) {
	event := cloudevents.NewEvent(cloudevents.VersionV1)
	event.SetID(logReq.Id)
	event.SetType(logReq.ReqType)

	event.SetExtension(InferenceServiceAttr, logReq.InferenceService)
	event.SetExtension(NamespaceAttr, logReq.Namespace)
	event.SetExtension(ComponentAttr, logReq.Component)
	event.SetExtension(EndpointAttr, logReq.Endpoint)

	encodedMetadata, err := json.Marshal(logReq.Metadata)
	if err != nil {
		return event, fmt.Errorf("could not encode metadata as json: %w", err)
	}
	event.SetExtension(MetadataAttr, string(encodedMetadata))

	event.SetSource(logReq.SourceUri.String())
	if err := event.SetData(logReq.ContentType, *logReq.Bytes); err != nil {
		return event, fmt.Errorf("while setting cloudevents data: %w", err)
	}

	if event.Time().IsZero() {
		event.SetTime(time.Now())
	}
	return event, nil
}

// buildClient constructs an http client and transport to send cloudevent request.
func (w *Worker) buildClient(url *url.URL, certName string, tlsSkipVerify bool) (*http.Client, error) {
	c := &http.Client{
		Timeout: time.Second * 10,
	}

	if url.Scheme == "https" {
		caCertFilePath := filepath.Join(LoggerCaCertMountPath, certName)
		caCertFile, err := os.ReadFile(caCertFilePath)
		// Do not fail if certificates not found, for backwards compatibility
		if err == nil {
			clientCertPool := x509.NewCertPool()
			if !clientCertPool.AppendCertsFromPEM(caCertFile) {
				return nil, fmt.Errorf("while parsing CA certificate")
			}

			tlsTransport := &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:            clientCertPool,
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: tlsSkipVerify, // #nosec G402
				},
			}
			c.Transport = tlsTransport
		} else {
			w.Log.Warnf("using https endpoint but could not find CA cert file %s", caCertFilePath)
		}
	}
	return c, nil
}

func (w *Worker) sendBatchCloudEvent(logReqs []LogRequest) error {
	events := make([]cloudevents.Event, 0, len(logReqs))

	for _, lr := range logReqs {
		event, err := w.prepareEvent(lr)
		if err != nil {
			return err
		}
		events = append(events, event)
	}

	c, err := w.buildClient(logReqs[0].Url, logReqs[0].CertName, logReqs[0].TlsSkipVerify)
	if err != nil {
		return fmt.Errorf("while creating client: %w", err)
	}

	httpReq, err := cloudevents.NewHTTPRequestFromEvents(w.CeCtx, logReqs[0].Url.String(), events)
	if err != nil {
		return fmt.Errorf("while creating http request: %w", err)
	}
	// Send event
	res, err := c.Do(httpReq)
	if err != nil {
		return fmt.Errorf("while sending event: %w", err)
	}
	// Close the body
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			w.Log.Warnf("unable to read body of response: %w", err)
		}

		w.Log.Errorf("Sent with status code %d, error: %v", res.StatusCode, errors.New(string(body)))
	} else {
		w.Log.Infof("Sent with status code %d", res.StatusCode)
	}
	return nil
}

func (w *Worker) sendCloudEvent(logReq LogRequest) error {
	// Create a format event
	event, err := w.prepareEvent(logReq)
	if err != nil {
		return err
	}

	// Create the client
	c, err := w.buildClient(logReq.Url, logReq.CertName, logReq.TlsSkipVerify)
	if err != nil {
		return fmt.Errorf("while creating client: %w", err)
	}

	// Generate Http request from event
	httpReq, err := cloudevents.NewHTTPRequestFromEvent(w.CeCtx, logReq.Url.String(), event)
	if err != nil {
		return fmt.Errorf("while creating http request: %w", err)
	}

	// Send event
	res, err := c.Do(httpReq)
	if err != nil {
		return fmt.Errorf("while sending event: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			w.Log.Warnf("unable to read body of response: %w", err)
		}
		w.Log.Errorf("Sent with status code %d, error: %v", res.StatusCode, errors.New(string(body)))
	} else {
		w.Log.Infof("Sent with status code %d", res.StatusCode)
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
				switch len(work) {
				case 1:
					workItem := work[0]
					// Receive a work request.
					w.Log.Infof("Received work request %d, url: %s, requestId: %s", w.ID, workItem.Url.String(), workItem.Id)

					if err := w.sendCloudEvent(workItem); err != nil {
						w.Log.Error(err, "Failed to send cloud event, url: %s", workItem.Url.String())
					}
				case 2:
					// Receive a work request.
					w.Log.Infof("Received work request %d, url: %s, requestIds: %s %s", w.ID, work[0].Url.String(), work[0].Id, work[1].Id)

					if err := w.sendBatchCloudEvent(work); err != nil {
						w.Log.Error(err, "Failed to send cloud event, url: %s", work[0].Url.String())
					}
				default:
					w.Log.Errorf("Invalid amount of work items, number of work items must be 1 or 2, not %d", len(work))
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
