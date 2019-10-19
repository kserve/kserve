package inferencelogger

import (
	"context"
	"fmt"
	"github.com/cloudevents/sdk-go"
	"github.com/cloudevents/sdk-go/pkg/cloudevents/transport"
	"github.com/go-logr/logr"
	"net/http"
	"net/url"
	"time"
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
			Timeout: 10 * time.Second,
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

func (W *Worker) sendCloudEvent(url *url.URL, body *[]byte, contentType string) error {

	t, err := cloudevents.NewHTTPTransport(
		cloudevents.WithTarget(url.String()),
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
	event.SetID("ABC-123")
	event.SetType("org.kubeflow.serving.inference")
	event.SetSource("http://localhost:8081/")
	event.SetDataContentType(contentType)
	err = event.SetData(body)
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

				//err := w.sendLog(work.url, work.b, work.contentType)
				err := w.sendCloudEvent(work.url, work.b, work.contentType)
				if err != nil {
					w.Log.Error(err, "Failed to send log", "URL", work.url.String())
				}

				//FIXME remove!
				time.Sleep(time.Hour)
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
