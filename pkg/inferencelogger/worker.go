package inferencelogger

import (
	"bytes"
	"fmt"
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
}

func (w *Worker) sendLog(url *url.URL, body *[]byte, contentType string) error {
	w.Log.Info("Calling server", "url", url.String(), "contentType", contentType)
	_, err := w.Client.Post(url.String(), contentType, bytes.NewReader(*body))
	return err
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

				err := w.sendLog(work.url, work.b, work.contentType)
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
