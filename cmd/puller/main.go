package main

import (
	"flag"
	"github.com/kubeflow/kfserving/pkg/puller"
)

var (
	configDir  = flag.String("config-dir", "/mnt/configs", "directory for multi-model config files")
	numWorkers = flag.Int("num-workers", 1, "number of workers, per model")
	numRetries = flag.Int("num-retries", 3, "number of retries for downloading a model")
)

func main() {
	flag.Parse()
	puller.WatchConfig(*configDir, *numWorkers)
}
