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
	"go.uber.org/zap"
)

var WorkerQueue chan chan LogRequest

func StartDispatcher(nworkers int, logger *zap.SugaredLogger) {
	// First, initialize the channel we are going to but the workers' work channels into.
	WorkerQueue = make(chan chan LogRequest, nworkers)

	// Now, create all of our workers.
	for i := range nworkers {
		logger.Info("Starting worker ", i+1)
		worker := NewWorker(i+1, WorkerQueue, logger)
		worker.Start()
	}

	go func() {
		for {
			work := <-WorkQueue
			go func() {
				worker := <-WorkerQueue
				worker <- work
			}()
		}
	}()
}
