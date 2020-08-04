package main

import (
	"flag"
	"github.com/kubeflow/kfserving/pkg/puller"
	"log"
)

var (
	modelDir = flag.String("model_dir", "/tmp","directory for multi-model config files")
	numWorkers = flag.Int("num_workers", 1,"number of workers for parallel downloads")
)

func main() {
	flag.Parse()
	puller.InitiatePullers(*numWorkers)
	puller.OnConfigChange(func(e puller.EventWrapper) {
		log.Println("Send a request to:", e.LoadState, "for model", e.ModelName)
		puller.AddModelToChannel(e)
	})
	puller.WatchConfig(*modelDir)
}
