package main

import (
	"cloud.google.com/go/storage"
	"context"
	"flag"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/kubeflow/kfserving/pkg/agent"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	s3credential "github.com/kubeflow/kfserving/pkg/credentials/s3"
	"os"
	"github.com/kubeflow/kfserving/pkg/agent/test/mockapi"
	"github.com/kubeflow/kfserving/pkg/agent/utils"
)

var (
	configDir = flag.String("config-dir", "/mnt/configs", "directory for model config files")
	modelDir  = flag.String("model-dir", "/mnt/models", "directory for model files")
	gcsEndpoint = flag.String("gcs-endpoint", "", "endpoint for GCS bucket")
)

func main() {
	flag.Parse()
	downloader := agent.Downloader{
		ModelDir:  *modelDir,
		Providers: map[utils.Protocol]utils.Provider{},
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

	if *gcsEndpoint != "" {
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		if err != nil {
			panic(err)
		}
		downloader.Providers[utils.GCS] = &utils.GCSProvider{
			Client: mockapi.AdaptClient(client),
		}
	}

	watcher := agent.NewWatcher(*configDir, *modelDir)
	agent.StartPuller(downloader, watcher.ModelEvents)
	watcher.Start()
}
