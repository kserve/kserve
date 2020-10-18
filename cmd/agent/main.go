package main

import (
	"flag"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/kubeflow/kfserving/pkg/agent"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	s3credential "github.com/kubeflow/kfserving/pkg/credentials/s3"
	"os"
)

var (
	configDir = flag.String("config-dir", "/mnt/configs", "directory for model config files")
	modelDir  = flag.String("model-dir", "/mnt/models", "directory for model files")
)

func main() {
	flag.Parse()
	downloader := agent.Downloader{
		ModelDir:  *modelDir,
		Providers: map[storage.Protocol]storage.Provider{},
	}
	if endpoint, ok := os.LookupEnv(s3credential.AWSEndpointUrl); ok {
		region, _ := os.LookupEnv(s3credential.AWSRegion)
		sess, err := session.NewSession(&aws.Config{
			Endpoint: aws.String(endpoint),
			Region:   aws.String(region)},
		)
		if err != nil {
			panic(err)
		}
		sessionClient := s3.New(sess)
		downloader.Providers[storage.S3] = &storage.S3Provider{
			Client: sessionClient,
			Downloader: s3manager.NewDownloaderWithClient(sessionClient, func(d *s3manager.Downloader) {
			}),
		}
	}

	watcher := agent.NewWatcher(*configDir, *modelDir)
	agent.StartPuller(downloader, watcher.ModelEvents)
	watcher.Start()
}
