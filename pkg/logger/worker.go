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
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	EndpointAttr     = "endpoint"
	AnnotationAttr   = "annotations"
	RecordedTimeAttr = "recordedtime"

	// LoggerWorkerQueueSize is the default WorkQueue capacity, used when
	// StartDispatcher is called with a non-positive queue size. Callers
	// (e.g. cmd/agent) can override it, such as via a flag.
	LoggerWorkerQueueSize = 100
	CloudEventsIdHeader   = "Ce-Id"
)

// A buffered channel that we can send work requests on.
var WorkQueue = make(chan LogRequest, LoggerWorkerQueueSize)

// QueueLogRequest enqueues req without blocking the caller (the inference
// request path). If WorkQueue is full - e.g. because the log endpoint is
// slow or unavailable and workers can't keep up - the request is dropped
// rather than piling up unbounded memory or stalling inference traffic.
func QueueLogRequest(req LogRequest) error {
	select {
	case WorkQueue <- req:
		return nil
	default:
		return fmt.Errorf("logger work queue is full (capacity %d), dropping log request %s", cap(WorkQueue), req.Id)
	}
}

// DefaultHTTPClientTimeout is used when NewWorker is given a non-positive
// timeout.
const DefaultHTTPClientTimeout = 10 * time.Second

// WorkerOption configures optional NewWorker behavior.
type WorkerOption func(*Worker)

// WithHTTPClientTimeout sets the timeout of the HTTP client the worker uses
// to deliver CloudEvents. A non-positive timeout keeps the default.
func WithHTTPClientTimeout(timeout time.Duration) WorkerOption {
	return func(w *Worker) {
		if timeout > 0 {
			w.httpClientTimeout = timeout
		}
	}
}

// NewWorker creates, and returns a new Worker object. Its only required
// arguments are a channel that the worker can add itself to whenever it is
// done its work, and a logger.
func NewWorker(id int, workerQueue chan chan LogRequest, logger *zap.SugaredLogger, opts ...WorkerOption) Worker {
	// Create, and return the worker.
	w := Worker{
		Log:               logger,
		ID:                id,
		Work:              make(chan LogRequest),
		WorkerQueue:       workerQueue,
		QuitChan:          make(chan bool),
		httpClientTimeout: DefaultHTTPClientTimeout,
		caCertDir:         constants.LoggerCaCertMountPath,
	}
	for _, opt := range opts {
		opt(&w)
	}
	return w
}

// clientCacheKey identifies the destination and TLS config a cached
// cloudevents.Client was built for.
type clientCacheKey struct {
	url           string
	certName      string
	tlsSkipVerify bool
}

// cachedClient is a cloudevents.Client together with what it was built from,
// so getClient can tell when it has gone stale.
type cachedClient struct {
	client    cloudevents.Client
	transport *http.Transport
	// caCert is the CA file content the transport trusts, nil when the file
	// was absent and the default roots are used.
	caCert []byte
}

type Worker struct {
	Log         *zap.SugaredLogger
	ID          int
	Work        chan LogRequest
	WorkerQueue chan chan LogRequest
	QuitChan    chan bool

	httpClientTimeout time.Duration
	// caCertDir is where the CA certificates for https log endpoints are
	// mounted. It is a field only so tests can point it elsewhere.
	caCertDir string

	// clients caches a cloudevents.Client per destination. A plain map is
	// safe here because Start's loop is the only goroutine that ever
	// touches a given Worker.
	clients map[clientCacheKey]*cachedClient
}

// readCACert returns the content of the CA file for logReq, or nil when the
// destination is not https or the file cannot be read.
func (w *Worker) readCACert(logReq LogRequest) []byte {
	if logReq.Url.Scheme != "https" {
		return nil
	}
	caCertFile, err := os.ReadFile(filepath.Clean(filepath.Join(w.caCertDir, logReq.CertName)))
	if err != nil {
		return nil
	}
	return caCertFile
}

// getClient returns the cached client for logReq's destination. The CA file
// is re-read for every event, as it was before clients were cached, so that a
// rotated or late-arriving CA is picked up: when its content differs from the
// one the cached client was built with, the client is rebuilt and the idle
// connections of the replaced one are closed.
func (w *Worker) getClient(logReq LogRequest) (cloudevents.Client, error) {
	key := clientCacheKey{
		url:           logReq.Url.String(),
		certName:      logReq.CertName,
		tlsSkipVerify: logReq.TlsSkipVerify,
	}
	caCert := w.readCACert(logReq)
	cached, found := w.clients[key]
	if found && bytes.Equal(cached.caCert, caCert) {
		return cached.client, nil
	}

	built, err := w.buildClient(logReq, caCert)
	if err != nil {
		return nil, err
	}
	if found {
		cached.transport.CloseIdleConnections()
	}
	if w.clients == nil {
		w.clients = make(map[clientCacheKey]*cachedClient)
	}
	w.clients[key] = built
	return built.client, nil
}

func (w *Worker) buildClient(logReq LogRequest, caCert []byte) (*cachedClient, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert
	transport.ForceAttemptHTTP2 = true

	if logReq.Url.Scheme == "https" {
		// Do not fail if certificates not found, for backwards compatibility
		if caCert != nil {
			clientCertPool := x509.NewCertPool()
			if !clientCertPool.AppendCertsFromPEM(caCert) {
				return nil, errors.New("while parsing CA certificate")
			}

			transport.TLSClientConfig = &tls.Config{
				RootCAs:            clientCertPool,
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: logReq.TlsSkipVerify, // #nosec G402
				NextProtos:         []string{"h2", "http/1.1"},
			}
		} else {
			w.Log.Warnf("using https endpoint but could not find CA cert file %s", filepath.Join(w.caCertDir, logReq.CertName))
		}
	}

	httpClient := http.Client{
		Transport: transport,
		Timeout:   w.httpClientTimeout,
	}
	t, err := cloudevents.NewHTTP(
		cloudevents.WithTarget(logReq.Url.String()),
		cehttp.WithClient(httpClient),
	)
	if err != nil {
		return nil, fmt.Errorf("while creating http transport: %w", err)
	}

	c, err := cloudevents.NewClient(t)
	if err != nil {
		return nil, fmt.Errorf("while creating new cloudevents client: %w", err)
	}
	return &cachedClient{client: c, transport: transport, caCert: caCert}, nil
}

func (w *Worker) sendHttpCloudEvent(logReq LogRequest) error {
	c, err := w.getClient(logReq)
	if err != nil {
		return err
	}

	event := cloudevents.NewEvent(cloudevents.VersionV1)
	event.SetID(logReq.Id)
	event.SetType(logReq.ReqType)
	event.SetTime(logReq.OccurrenceTime)
	event.SetExtension(RecordedTimeAttr, time.Now())

	event.SetExtension(InferenceServiceAttr, logReq.InferenceService)
	event.SetExtension(NamespaceAttr, logReq.Namespace)
	event.SetExtension(ComponentAttr, logReq.Component)
	event.SetExtension(EndpointAttr, logReq.Endpoint)

	encodedMetadata, err := json.Marshal(logReq.Metadata)
	if err != nil {
		return fmt.Errorf("could not encode metadata as json: %w", err)
	}
	event.SetExtension(MetadataAttr, string(encodedMetadata))

	if len(logReq.Annotations) > 0 {
		bits, err := json.Marshal(logReq.Annotations)
		if err != nil {
			w.Log.Errorf("failed to marshal annotations: %w", err)
		} else {
			event.SetExtension(AnnotationAttr, string(bits))
		}
	}

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

// Start begins the worker goroutine. Workers handle HTTP CloudEvents delivery.
// Blob storage is handled by the dispatcher's batch pipeline.
func (w *Worker) Start() {
	go func() {
		for {
			// Add ourselves into the worker queue.
			w.WorkerQueue <- w.Work

			select {
			case work := <-w.Work:
				w.Log.Infof("Received work request %d, url: %s, requestId: %s", w.ID, work.Url.String(), work.Id)

				if err := w.sendHttpCloudEvent(work); err != nil {
					w.Log.Error(err, "Failed to send cloud event, url: %s", work.Url.String())
				}

			case <-w.QuitChan:
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
