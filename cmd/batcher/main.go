/*
Copyright 2019 kubeflow.org.

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
	port          = flag.String("port", "8082", "Logger port")
	componentHost = flag.String("component-host", "127.0.0.1", "Component host")
	componentPort = flag.String("component-port", "8080", "Component port")
	maxBatchsize  = flag.String("max-batchsize", "32", "Max Batchsize")
	maxLatency    = flag.String("max-latency", "1.0", "Max Latency")
)

func main() {
	flag.Parse()

	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	maxBatchsizeInt, err := strconv.Atoi(*maxBatchsize)
	if err != nil || maxBatchsizeInt <= 0 {
		log.Info("Invalid max batchsize", "max-batchsize", *maxBatchsize)
		os.Exit(-1)
	}

	maxLatencyFloat64, err := strconv.ParseFloat(*maxLatency, 64)
	if err != nil || maxLatencyFloat64 <= 0.0 {
		log.Info("Invalid max latency", "max-latency", *maxLatency)
		os.Exit(-1)
	}

	controllers.New(log, *port, *componentHost, *componentPort, maxBatchsizeInt, maxLatencyFloat64)

	log.Info("Starting", "Port", *port)
	batcher.StartHttpServer()
}
