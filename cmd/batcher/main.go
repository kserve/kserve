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
	"flag"
	"os"
	"strconv"
	"github.com/kubeflow/kfserving/pkg/batcher"
	"github.com/kubeflow/kfserving/pkg/batcher/controllers"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	port          = flag.String("port", "9082", "Batcher port")
	componentHost = flag.String("component-host", "127.0.0.1", "Component host")
	componentPort = flag.String("component-port", "8080", "Component port")
	maxBatchSize  = flag.String("max-batchsize", "32", "Max Batch Size")
	maxLatency    = flag.String("max-latency", "5000.0", "Max Latency in milliseconds")
	timeout       = flag.String("timeout", "60", "Timeout of calling predictor service in seconds")
)

func main() {
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	maxBatchSizeInt, err := strconv.Atoi(*maxBatchSize)
	if err != nil || maxBatchSizeInt <= 0 {
		log.Info("Invalid max batch size", "max-batchsize", *maxBatchSize)
		os.Exit(1)
	}

	maxLatencyFloat64, err := strconv.ParseFloat(*maxLatency, 64)
	if err != nil || maxLatencyFloat64 <= 0.0 {
		log.Info("Invalid max latency", "max-latency", *maxLatency)
		os.Exit(1)
	}

	timeoutInt, err := strconv.Atoi(*timeout)
	if err != nil || timeoutInt <= 0 {
		log.Info("Invalid timeout", "timeout", *timeout)
		os.Exit(1)
	}

	controllers.Config(*port, *componentHost, *componentPort, maxBatchSizeInt, maxLatencyFloat64, timeoutInt)

	log.Info("Starting", "Port", *port)
	batcher.StartHttpServer()
}
