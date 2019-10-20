package inferencelogger

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
)

// NewWorker creates, and returns a new Worker object. Its only argument
// is a channel that the worker can add itself to whenever it is done its
// work.
func NewWorker(id int, workerQueue chan chan LogRequest, log logr.Logger) Worker {
	// Create, and return the worker.
	worker := Worker{
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

	return worker
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
		cloudevents.WithTarget(logReq.url.String()),
		//cloudevents.WithEncoding(cloudevents.HTTPBinaryV02),
		cloudevents.WithContextBasedEncoding(), // toggle this or WithEncoding to see context based encoding work.
	)
	if err != nil {
		return err
	}
	c, err := cloudevents.NewClient(t,
		cloudevents.WithTimeNow(),
	)
	if err != nil {
		return err
	}
	event := cloudevents.NewEvent()
	event.SetID(logReq.id)
	if logReq.reqType == InferenceRequest {
		event.SetType(CEInferenceRequest)
	} else {
		event.SetType(CEInferenceResponse)
	}

	event.SetSource(logReq.sourceUri.String())
	event.SetDataContentType(logReq.contentType)
	err = event.SetData(*logReq.b)
	if err != nil {
		return err
	}

	if _, _, err := c.Send(W.CeCtx, event); err != nil {
		return nil
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
				fmt.Printf("worker%d: Received work request for %s\n", w.ID, work.url.String())

				err := w.sendCloudEvent(work)
				if err != nil {
					w.Log.Error(err, "Failed to send log", "URL", work.url.String())
				}

			case <-w.QuitChan:
				// We have been asked to stop.
				fmt.Printf("worker%d stopping\n", w.ID)
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
