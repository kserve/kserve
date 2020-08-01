package main

import (
	"flag"
	"github.com/kubeflow/kfserving/pkg/puller"
	"log"
)

var (
	modelDir = flag.String("model_dir", "/tmp","directory for multi-model config files")
)

func main() {
	flag.Parse()
	puller.OnConfigChange(func(e puller.EventWrapper) {
		log.Println("Send a request to:", e.LoadState)
		log.Println("for model", e.ModelName)
	})
	puller.WatchConfig(*modelDir)
}
