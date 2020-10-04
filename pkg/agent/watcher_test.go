package agent

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	logger "log"
	"os"
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
}

func (m *mockS3Downloder) DownloadWithIterator(aws.Context, s3manager.BatchDownloadIterator, ...func(*s3manager.Downloader)) error {
	return nil
}

type mockS3FailDownloder struct {
	err error
}

func (m *mockS3FailDownloder) DownloadWithIterator(aws.Context, s3manager.BatchDownloadIterator, ...func(*s3manager.Downloader)) error {
	var errs []s3manager.Error
	errs = append(errs, s3manager.Error{
		OrigErr: fmt.Errorf("failed to download"),
		Bucket:  aws.String("modelRepo"),
		Key:     aws.String("model1/model.pt"),
	})
	return s3manager.NewBatchError("BatchedDownloadIncomplete", "some objects have failed to download.", errs)
}

var _ = Describe("Watcher", func() {
	var modelDir string
	BeforeEach(func() {
		dir, err := ioutil.TempDir("", "example")
		if err != nil {
			logger.Fatal(err)
		}
		modelDir = dir
		logger.Printf("Creating temp dir %v\n", modelDir)
	})
	AfterEach(func() {
		os.RemoveAll(modelDir)
		logger.Printf("Deleted temp dir %v\n", modelDir)
	})
	Describe("Sync model config", func() {
		Context("Sync new models", func() {
			It("should download the new models", func() {
				defer GinkgoRecover()
				logger.Printf("Sync model config using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: Downloader{
						ModelDir: modelDir + "/test1",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mockS3Client{},
								Downloader: &mockS3Downloder{},
							},
						},
					},
				}
				go puller.processCommands(watcher.ModelEvents)
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
				watcher.parseConfig(modelConfigs)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
			})
		})

		Context("Sync delete models", func() {
			It("should download the deleted models", func() {
				defer GinkgoRecover()
				logger.Printf("Sync delete models using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: Downloader{
						ModelDir: modelDir + "/test2",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mockS3Client{},
								Downloader: &mockS3Downloder{},
							},
						},
					},
				}
				go puller.processCommands(watcher.ModelEvents)
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
				watcher.parseConfig(modelConfigs)
				// remove model2
				modelConfigs = modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1beta1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Remove] }).Should(Equal(1))
			})
		})

		Context("Sync update models", func() {
			It("should update models", func() {
				defer GinkgoRecover()
				logger.Printf("Sync update models using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: Downloader{
						ModelDir: modelDir + "/test3",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mockS3Client{},
								Downloader: &mockS3Downloder{},
							},
						},
					},
				}
				go puller.processCommands(watcher.ModelEvents)
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
				watcher.parseConfig(modelConfigs)
				// remove model2
				modelConfigs = modelconfig.ModelConfigs{
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
							StorageURI: "s3://models/model2v2",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(2))
				Eventually(func() int { return puller.opStats["model2"][Remove] }).Should(Equal(1))
			})
		})

		Context("Model download failure", func() {
			It("should not create success file", func() {
				defer GinkgoRecover()
				logger.Printf("Using temp dir %v\n", modelDir)
				var errs []s3manager.Error
				errs = append(errs, s3manager.Error{
					OrigErr: fmt.Errorf("failed to download"),
					Bucket:  aws.String("modelRepo"),
					Key:     aws.String("model1/model.pt"),
				})
				var err error
				err = s3manager.NewBatchError("BatchedDownloadIncomplete", "some objects have failed to download.", errs)
				watcher := NewWatcher("/tmp/configs", modelDir)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: Downloader{
						ModelDir: modelDir + "/test4",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mockS3Client{},
								Downloader: &mockS3FailDownloder{err: err},
							},
						},
					},
				}
				go puller.processCommands(watcher.ModelEvents)
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1beta1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
			})
		})
	})
})
