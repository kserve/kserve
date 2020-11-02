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
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"strings"
)

var (
	log       = logf.Log.WithName("modelAgent")
	configDir = flag.String("config-dir", "/mnt/configs", "directory for model config files")
	modelDir  = flag.String("model-dir", "/mnt/models", "directory for model files")
)

func main() {
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(false))
	log.Info("Initializing model agent with", "config-dir", configDir, "model-dir", modelDir)
	downloader := agent.Downloader{
		ModelDir:  *modelDir,
		Providers: map[storage.Protocol]storage.Provider{},
	}

	if endpoint, ok := os.LookupEnv(s3credential.AWSEndpointUrl); ok {
		region, _ := os.LookupEnv(s3credential.AWSRegion)
		useVirtualBucketString, ok := os.LookupEnv(s3credential.S3UseVirtualBucket)
		useVirtualBucket := true
		if ok && strings.ToLower(useVirtualBucketString) == "false" {
			useVirtualBucket = false
		}
		sess, err := session.NewSession(&aws.Config{
			Endpoint:         aws.String(endpoint),
			Region:           aws.String(region),
			S3ForcePathStyle: aws.Bool(!useVirtualBucket)},
		)
		log.Info("Initializing s3 client with ", "endpoint", endpoint, "region", region)
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
