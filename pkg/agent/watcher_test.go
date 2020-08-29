package agent

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/s3/s3manager/s3manageriface"
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	. "github.com/onsi/ginkgo"
	"io"
)

type mockS3Client struct {
	s3iface.S3API
}

func (m *mockS3Client) ListObjects(*s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
	return &s3.ListObjectsOutput{
		Contents: []*s3.Object{
			{
				Key: proto.String("model.pt"),
			},
		},
	}, nil
}

type mockS3Downloder struct {
	s3manageriface.DownloaderAPI
}

func (m *mockS3Downloder) DownloadWithContext(aws.Context, io.WriterAt, *s3.GetObjectInput, ...func(*s3manager.Downloader)) (int64, error) {
	return 0, nil
}

var _ = Describe("Watcher", func() {
	var watcher Watcher
	BeforeEach(func() {
	})
	Describe("Sync model config", func() {
		Context("Sync new models", func() {
			It("should download the new models", func() {
				defer GinkgoRecover()
				done := make(chan EventWrapper)
				watcher = Watcher{
					ConfigDir:    "/tmp/configs",
					ModelTracker: map[string]ModelWrapper{},
					Puller: Puller{
						ChannelMap: map[string]Channel{},
						Downloader: Downloader{
							ModelDir: "/tmp/models",
							Providers: map[storage.Protocol]storage.Provider{
								storage.S3: &storage.S3Provider{
									Client:     &mockS3Client{},
									Downloader: &mockS3Downloder{},
								},
							},
						},
					},
					EventDoneChannel: done,
				}
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1beta1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1beta1.ModelSpec{
							StorageURI: "s3://models/model2",
							Framework:  "sklearn",
						},
					},
				}
				watcher.ParseConfig(modelConfigs)
				event1 := <- done
				fmt.Printf("event done %v\n", event1)
				event2 := <-done
				fmt.Printf("event done %v\n", event2)
			})
		})
	})
})
