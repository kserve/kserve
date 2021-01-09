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

package main

import (
	"errors"
	"flag"
	"github.com/kubeflow/kfserving/pkg/batcher"
	"os"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strconv"
)

var (
	port          = flag.String("port", "9082", "Batcher port")
	componentHost = flag.String("component-host", "127.0.0.1", "Component host")
	componentPort = flag.String("component-port", "8080", "Component port")
	maxBatchSize  = flag.String("max-batchsize", "32", "Max Batch Size")
	maxLatency    = flag.String("max-latency", "5000", "Max Latency in milliseconds")
	timeout       = flag.String("timeout", "60", "Timeout of calling predictor service in seconds")
)

func main() {
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	maxBatchSizeInt, err := strconv.Atoi(*maxBatchSize)
	if err != nil || maxBatchSizeInt <= 0 {
		log.Error(errors.New("Invalid max batch size"), *maxBatchSize)
		os.Exit(1)
	}

	maxLatencyInt, err := strconv.Atoi(*maxLatency)
	if err != nil || maxLatencyInt <= 0 {
		log.Error(errors.New("Invalid max latency"), *maxLatency)
		os.Exit(1)
	}

	timeoutInt, err := strconv.Atoi(*timeout)
	if err != nil || timeoutInt <= 0 {
		log.Error(errors.New("Invalid timeout"), *timeout)
		os.Exit(1)
	}

	batcher.Config(*port, *componentHost, *componentPort, maxBatchSizeInt, maxLatencyInt, timeoutInt)

	log.Info("Starting", "Port", *port)
	//batcher.StartHttpServer()
}
