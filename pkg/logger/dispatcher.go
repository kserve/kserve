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

	"go.uber.org/zap"
)

var WorkerQueue chan chan LogRequest

func StartDispatcher(nworkers int, store Store, batchStrategy BatchStrategy, logger *zap.SugaredLogger) {
	// Reinitialize WorkQueue so that any previous dispatcher goroutines
	// (from prior calls, e.g. in tests) lose their channel reference and
	// cannot compete for work items.
	WorkQueue = make(chan LogRequest, LoggerWorkerQueueSize)

	// Initialize the channel for workers to register their work channels.
	WorkerQueue = make(chan chan LogRequest, nworkers)

	// Create workers for HTTP CloudEvents processing.
	for i := range nworkers {
		logger.Info("Starting worker ", i+1)
		worker := NewWorker(i+1, WorkerQueue, logger)
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

	// Dispatcher goroutine: read from WorkQueue, split HTTP vs blob.
	go func() {
		for work := range WorkQueue {
			strategy := GetStorageStrategy(work.Url.String())

			if strategy == HttpStorage {
				// Dispatch to a worker for CloudEvents delivery.
				w := work
				go func() {
					worker := <-WorkerQueue
					worker <- w
				}()
			} else {
				// Send to batch pipeline for blob storage.
				batchIn <- work
			}
		}
	}()
}
