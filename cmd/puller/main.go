package main

import (
	"flag"
	"github.com/kubeflow/kfserving/pkg/puller"
)

var (
	configDir  = flag.String("config-dir", "/mnt/configs", "directory for multi-model config files")
	modelDir   = flag.String("model-dir", "/mnt/models", "directory for multi-model models")
	numWorkers = flag.Int("num-workers", 1, "number of workers, per model")
	numRetries = flag.Int("num-retries", 3, "number of retries for downloading a model")
)

func main() {
	flag.Parse()
	puller.SetEnvs(*numRetries, *modelDir)
	puller.WatchConfig(*configDir, *numWorkers)
}
