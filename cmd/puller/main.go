package main

import (
	"flag"
	"github.com/kubeflow/kfserving/pkg/puller"
	"log"
)

var (
	modelDir = flag.String("model_dir", "/tmp","directory for multi-model config files")
	numWorkers = flag.Int("num_workers", 1,"number of workers for parallel downloads")
	numRetries = flag.Int("num_retries", 3, "number of retries for downloading a model")
)

func main() {
	flag.Parse()
	puller.InitiatePullers(*numWorkers, *numRetries)
	puller.OnConfigChange(func(e puller.EventWrapper) {
		log.Println("Send a request to:", e.LoadState, "model:", e.ModelName)
		puller.AddModelToChannel(e)
	})
	puller.WatchConfig(*modelDir)
}
