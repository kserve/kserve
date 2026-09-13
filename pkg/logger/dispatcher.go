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
	"time"

	"go.uber.org/zap"
)

var WorkerQueue chan chan LogRequest

// DispatcherOption configures optional StartDispatcher behavior.
type DispatcherOption func(*dispatcherConfig)

type dispatcherConfig struct {
	httpClientTimeout time.Duration
}

// WithLogClientTimeout sets the timeout each worker's HTTP client uses when
// delivering CloudEvents to the logger URL.
func WithLogClientTimeout(timeout time.Duration) DispatcherOption {
	return func(c *dispatcherConfig) {
		c.httpClientTimeout = timeout
	}
}

func StartDispatcher(nworkers int, store Store, batchStrategy BatchStrategy, logger *zap.SugaredLogger, opts ...DispatcherOption) {
	cfg := dispatcherConfig{httpClientTimeout: DefaultHTTPClientTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Reinitialize WorkQueue so that any previous dispatcher goroutines
	// (from prior calls, e.g. in tests) lose their channel reference and
	// cannot compete for work items. workQueue/workerQueue are also
	// captured by the goroutines below instead of read back from the
	// package vars, since a previous call's goroutines aren't guaranteed
	// to have exited yet and would otherwise race this reassignment.
	workQueue := make(chan LogRequest, LoggerWorkerQueueSize)
	WorkQueue = workQueue

	// Initialize the channel for workers to register their work channels.
	workerQueue := make(chan chan LogRequest, nworkers)
	WorkerQueue = workerQueue

	// Create workers for HTTP CloudEvents processing.
	for i := range nworkers {
		logger.Info("Starting worker ", i+1)
		worker := NewWorker(i+1, workerQueue, logger, cfg.httpClientTimeout)
		worker.Start()
	}

	// Set up the batch pipeline for blob storage.
	batchIn := make(chan LogRequest)
	batchOut := make(chan []LogRequest)
	go batchStrategy.Run(context.Background(), batchIn, batchOut)

	// Process batches from BatchStrategy output and write to Store.
	go func() {
		for batch := range batchOut {
			if len(batch) == 0 {
				continue
			}
			if store == nil {
				logger.Error("Logger store not configured, cannot store batch")
				continue
			}
			if err := store.Store(batch[0].Url, batch); err != nil {
				logger.Errorf("Failed to store batch: %v", err)
			}
		}
	}()

	// Dispatcher goroutine: read from workQueue, split HTTP vs blob.
	go func() {
		for work := range workQueue {
			strategy := GetStorageStrategy(work.Url.String())

			if strategy == HttpStorage {
				// Dispatch to a worker for CloudEvents delivery.
				w := work
				go func() {
					worker := <-workerQueue
					worker <- w
				}()
			} else {
				// Send to batch pipeline for blob storage.
				batchIn <- work
			}
		}
	}()
}
