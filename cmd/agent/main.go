package main

import (
	"flag"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/kubeflow/kfserving/pkg/agent"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
)

var (
	configDir  = flag.String("config-dir", "/mnt/configs", "directory for model config files")
	modelDir   = flag.String("model-dir", "/mnt/models", "directory for model files")
	s3Endpoint = flag.String("s3-endpoint", "", "endpoint for s3 bucket")
	s3Region   = flag.String("s3-region", "us-west-2", "region for s3 bucket")
)

func main() {
	flag.Parse()
	downloader := agent.Downloader{
		ModelDir:  *modelDir,
		Providers: map[storage.Protocol]storage.Provider{},
	}
	if *s3Endpoint != "" {
		sess, err := session.NewSession(&aws.Config{
			Endpoint: aws.String(*s3Endpoint),
			Region:   aws.String(*s3Region)},
		)
		if err != nil {
			panic(err)
		}
		downloader.Providers[storage.S3] = &storage.S3Provider{
			Client: s3.New(sess),
		}
	}

	watcher := agent.NewWatcher(*configDir)
	agent.StartPuller(downloader, watcher.ModelEvents)
	watcher.Start()
}
