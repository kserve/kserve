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
	"log"
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
	var watcher Watcher
	var modelDir string
	BeforeEach(func() {
		dir, err := ioutil.TempDir("", "example")
		if err != nil {
			log.Fatal(err)
		}
		modelDir = dir
		log.Printf("Creating temp dir %v\n", modelDir)
	})
	AfterEach(func() {
		os.RemoveAll(modelDir)
		log.Printf("Deleted temp dir %v\n", modelDir)
	})
	Describe("Sync model config", func() {
		Context("Sync new models", func() {
			It("should download the new models", func() {
				defer GinkgoRecover()
				log.Printf("Using temp dir %v\n", modelDir)
				done := make(chan EventWrapper)
				watcher = Watcher{
					ConfigDir:    "/tmp/configs",
					ModelTracker: map[string]ModelWrapper{},
					Puller: Puller{
						ChannelMap: map[string]Channel{},
						Downloader: Downloader{
							ModelDir: modelDir + "/test1",
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
				doneEventMap := map[string]EventWrapper{}
				event1 := <-done
				doneEventMap[event1.ModelName] = event1
				event2 := <-done
				doneEventMap[event2.ModelName] = event2
				Expect(doneEventMap["model1"]).To(Equal(EventWrapper{
					ModelName:      "model1",
					ModelSpec:      &modelConfigs[0].Spec,
					Error:          nil,
					LoadState:      ShouldLoad,
					ShouldDownload: true,
				}))
				Expect(doneEventMap["model2"]).To(Equal(EventWrapper{
					ModelName:      "model2",
					ModelSpec:      &modelConfigs[1].Spec,
					Error:          nil,
					LoadState:      ShouldLoad,
					ShouldDownload: true,
				}))
			})
		})

		Context("Sync delete models", func() {
			It("should download the deleted models", func() {
				defer GinkgoRecover()
				log.Printf("Using temp dir %v\n", modelDir)
				done := make(chan EventWrapper)
				watcher = Watcher{
					ConfigDir:    "/tmp/configs",
					ModelTracker: map[string]ModelWrapper{},
					Puller: Puller{
						ChannelMap: map[string]Channel{},
						Downloader: Downloader{
							ModelDir: modelDir + "/test2",
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
				<-done
				<-done
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
				watcher.ParseConfig(modelConfigs)
				event1 := <-done
				Expect(event1).To(Equal(EventWrapper{
					ModelName: "model2",
					ModelSpec: &v1beta1.ModelSpec{
						StorageURI: "s3://models/model2",
						Framework:  "sklearn",
					},
					ShouldDownload: false,
					LoadState:      ShouldUnload,
					Error:          nil,
				}))
			})
		})

		Context("Sync update models", func() {
			It("should update models", func() {
				defer GinkgoRecover()
				log.Printf("Using temp dir %v\n", modelDir)
				done := make(chan EventWrapper)
				watcher = Watcher{
					ConfigDir:    "/tmp/configs",
					ModelTracker: map[string]ModelWrapper{},
					Puller: Puller{
						ChannelMap: map[string]Channel{},
						Downloader: Downloader{
							ModelDir: modelDir + "/test3",
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
				<-done
				<-done
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
				watcher.ParseConfig(modelConfigs)
				event1 := <-done
				Expect(event1).To(Equal(EventWrapper{
					ModelName: "model2",
					ModelSpec: &v1beta1.ModelSpec{
						StorageURI: "s3://models/model2v2",
						Framework:  "sklearn",
					},
					ShouldDownload: true,
					LoadState:      ShouldLoad,
					Error:          nil,
				}))
			})
		})

		Context("Model download failure", func() {
			It("should not create success file", func() {
				defer GinkgoRecover()
				log.Printf("Using temp dir %v\n", modelDir)
				done := make(chan EventWrapper)
				var errs []s3manager.Error
				errs = append(errs, s3manager.Error{
					OrigErr: fmt.Errorf("failed to download"),
					Bucket:  aws.String("modelRepo"),
					Key:     aws.String("model1/model.pt"),
				})
				var err error
				err = s3manager.NewBatchError("BatchedDownloadIncomplete", "some objects have failed to download.", errs)
				watcher = Watcher{
					ConfigDir:    "/tmp/configs",
					ModelTracker: map[string]ModelWrapper{},
					Puller: Puller{
						ChannelMap: map[string]Channel{},
						Downloader: Downloader{
							ModelDir: modelDir + "/test4",
							Providers: map[storage.Protocol]storage.Provider{
								storage.S3: &storage.S3Provider{
									Client:     &mockS3Client{},
									Downloader: &mockS3FailDownloder{err: err},
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
				}
				watcher.ParseConfig(modelConfigs)
				event1 := <-done
				Expect(event1).To(Equal(EventWrapper{
					ModelName: "model1",
					ModelSpec: &v1beta1.ModelSpec{
						StorageURI: "s3://models/model1",
						Framework:  "sklearn",
					},
					ShouldDownload: true,
					LoadState:      ShouldLoad,
					Error:          err,
				}))
			})
		})
	})
})
