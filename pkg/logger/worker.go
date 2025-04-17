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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"go.uber.org/zap"

	"github.com/kserve/kserve/pkg/constants"
)

const (
	CEInferenceRequest  = "org.kubeflow.serving.inference.request"
	CEInferenceResponse = "org.kubeflow.serving.inference.response"

	// cloud events extension attributes have to be lowercase alphanumeric
	// TODO: ideally request id would have its own header but make do with ce-id for now
	InferenceServiceAttr = "inferenceservicename"
	NamespaceAttr        = "namespace"
	ComponentAttr        = "component"
	MetadataAttr         = "metadata"
	// endpoint would be either default or canary
	EndpointAttr = "endpoint"

	LoggerWorkerQueueSize = 100
	CloudEventsIdHeader   = "Ce-Id"
)

// A buffered channel that we can send work requests on.
var WorkQueue = make(chan LogRequest, LoggerWorkerQueueSize)

var PayloadSchema *Schema

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
	}
}

type Worker struct {
	Log         *zap.SugaredLogger
	ID          int
	Work        chan LogRequest
	WorkerQueue chan chan LogRequest
	QuitChan    chan bool
}

// The sendEvent functionality has been extended to support custom user provided JSON schemas.  In the future, it is aimed to
// be expanded to support formats such as avro, parquet, etc.
func (w *Worker) sendEvent(logReq LogRequest) error {
	if logReq.CustomSchema != "" {
		if PayloadSchema == nil {
			schemaPath := filepath.Join(constants.LoggerCustomSchemaMountPath, logReq.CustomSchema)
			rawSchema, err := os.ReadFile(schemaPath)
			if err != nil {
				w.Log.Errorf("Failed to find user-supplied schema: %s", err.Error())
				return err
			}
			schema, err := LoadSchema(rawSchema)
			if err != nil {
				w.Log.Errorf("Failed to load schema in-memory: %s", err.Error())
				return err
			}
			PayloadSchema = schema
		}
		w.sendCustomEvent(logReq)

		return nil
	} else {
		return w.sendCloudEvent(logReq)
	}
}

func (w *Worker) sendCustomEvent(logReq LogRequest) error {
	client := http.Client{}
	if logReq.Url.Scheme == "https" {
		caCertFilePath := filepath.Join(constants.LoggerCaCertMountPath, logReq.CertName)
		caCertFile, err := os.ReadFile(caCertFilePath)
		// Do not fail if certificates not found, for backwards compatibility
		if err == nil {
			clientCertPool := x509.NewCertPool()
			if !clientCertPool.AppendCertsFromPEM(caCertFile) {
				return errors.New("while parsing CA certificate")
			}

			tlsTransport := &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:            clientCertPool,
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: logReq.TlsSkipVerify, // #nosec G402
				},
			}
			client.Transport = tlsTransport
		} else {
			w.Log.Warnf("using https endpoint but could not find CA cert file %s", caCertFilePath)
		}
	}
	payload, err := GeneratePayload(logReq)
	if err != nil {
		return fmt.Errorf("while generating payload from custom schema: %w", err)
	}
	resp, err := client.Post(logReq.Url.String(), logReq.ContentType, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("while sending HTTP POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("non-200 response: %d - %s", resp.StatusCode, string(body))
	}
	return nil
}

func (w *Worker) sendCloudEvent(logReq LogRequest) error {
	t, err := cloudevents.NewHTTP(
		cloudevents.WithTarget(logReq.Url.String()),
	)
	if err != nil {
		return fmt.Errorf("while creating http transport: %w", err)
	}

	if logReq.Url.Scheme == "https" {
		caCertFilePath := filepath.Join(constants.LoggerCaCertMountPath, logReq.CertName)
		caCertFile, err := os.ReadFile(caCertFilePath)
		// Do not fail if certificates not found, for backwards compatibility
		if err == nil {
			clientCertPool := x509.NewCertPool()
			if !clientCertPool.AppendCertsFromPEM(caCertFile) {
				return errors.New("while parsing CA certificate")
			}

			tlsTransport := &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:            clientCertPool,
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: logReq.TlsSkipVerify, // #nosec G402
				},
			}
			t.Client.Transport = tlsTransport
		} else {
			w.Log.Warnf("using https endpoint but could not find CA cert file %s", caCertFilePath)
		}
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

	encodedMetadata, err := json.Marshal(logReq.Metadata)
	if err != nil {
		return fmt.Errorf("could not encode metadata as json: %w", err)
	}
	event.SetExtension(MetadataAttr, string(encodedMetadata))

	event.SetSource(logReq.SourceUri.String())
	if err := event.SetData(logReq.ContentType, *logReq.Bytes); err != nil {
		return fmt.Errorf("while setting cloudevents data: %w", err)
	}
	ceCtx := cloudevents.WithEncodingBinary(context.Background())
	res := c.Send(ceCtx, event)
	if cloudevents.IsUndelivered(res) {
		return fmt.Errorf("while sending event: %w", res)
	} else {
		var httpResult *cehttp.Result
		if cloudevents.ResultAs(res, &httpResult) {
			var err error
			if httpResult.StatusCode != http.StatusOK {
				err = fmt.Errorf(httpResult.Format, httpResult.Args...)
			}
			w.Log.Infof("Sent with status code %d, error: %v", httpResult.StatusCode, err)
		} else {
			w.Log.Infof("Send did not return an HTTP response: %s", res)
		}
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
				w.Log.Infof("work received %v", work)
				if err := w.sendEvent(work); err != nil {
					w.Log.Error(err, "Failed to send cloud event, url: %s", work.Url.String())
				}

			case <-w.QuitChan:
				// We have been asked to stop.
				w.Log.Infof("worker %d stopping\n", w.ID)
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
