/*
Copyright 2020 kubeflow.org.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package agent

import (
	"bytes"
	gstorage "cloud.google.com/go/storage"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/kubeflow/kfserving/pkg/agent/mocks"
	"github.com/kubeflow/kfserving/pkg/agent/storage"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/modelconfig"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/resource"
	logger "log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

var _ = Describe("Watcher", func() {
	var modelDir string
	var sugar *zap.SugaredLogger
	BeforeEach(func() {
		dir, err := ioutil.TempDir("", "example")
		if err != nil {
			logger.Fatal(err)
		}
		modelDir = dir
		zapLogger, _ := zap.NewProduction()
		sugar = zapLogger.Sugar()
		logger.Printf("Creating temp dir %v\n", modelDir)
	})
	AfterEach(func() {
		os.RemoveAll(modelDir)
		logger.Printf("Deleted temp dir %v\n", modelDir)
	})

	Describe("Sync models config on startup", func() {
		Context("Getting new model events", func() {
			It("should download and load the new models", func() {
				defer GinkgoRecover()
				logger.Printf("Sync model config using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
							Memory:     resource.MustParse("100Mi"),
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model2",
							Framework:  "sklearn",
							Memory:     resource.MustParse("100Mi"),
						},
					},
				}
				watcher.parseConfig(modelConfigs, false)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					waitGroup: WaitGroupWrapper{sync.WaitGroup{}},
					Downloader: Downloader{
						ModelDir: modelDir + "/test1",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mocks.MockS3Client{},
								Downloader: &mocks.MockS3Downloader{},
							},
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				go puller.processCommands(watcher.ModelEvents)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
				modelSpecMap, _ := SyncModelDir(modelDir+"/test1", watcher.logger)
				Expect(watcher.ModelTracker).Should(Equal(modelSpecMap))
			})
		})
	})

	Describe("Watch model config changes", func() {
		Context("When new models are added", func() {
			It("Should download and load the new models", func() {
				defer GinkgoRecover()
				logger.Printf("Sync model config using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					waitGroup:   WaitGroupWrapper{sync.WaitGroup{}},
					Downloader: Downloader{
						ModelDir: modelDir + "/test1",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mocks.MockS3Client{},
								Downloader: &mocks.MockS3Downloader{},
							},
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				go puller.processCommands(watcher.ModelEvents)
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model2",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs, false)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
			})
		})

		Context("When models are deleted from config", func() {
			It("Should remove the model dir and unload the models", func() {
				defer GinkgoRecover()
				logger.Printf("Sync delete models using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					waitGroup:   WaitGroupWrapper{sync.WaitGroup{}},
					Downloader: Downloader{
						ModelDir: modelDir + "/test2",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mocks.MockS3Client{},
								Downloader: &mocks.MockS3Downloader{},
							},
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				go puller.processCommands(watcher.ModelEvents)
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model2",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs, false)
				// remove model2
				modelConfigs = modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs, false)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Remove] }).Should(Equal(1))
			})
		})

		Context("When models uri are updated in config", func() {
			It("Should download and reload the model", func() {
				defer GinkgoRecover()
				logger.Printf("Sync update models using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					waitGroup:   WaitGroupWrapper{sync.WaitGroup{}},
					Downloader: Downloader{
						ModelDir: modelDir + "/test3",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mocks.MockS3Client{},
								Downloader: &mocks.MockS3Downloader{},
							},
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				puller.waitGroup.wg.Add(len(watcher.ModelEvents))
				go puller.processCommands(watcher.ModelEvents)
				puller.waitGroup.wg.Wait()
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model2",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs, false)
				// update model2 storageUri
				modelConfigs = modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model2v2",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs, false)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(2))
				Eventually(func() int { return puller.opStats["model2"][Remove] }).Should(Equal(1))
			})
		})

		Context("When model download fails", func() {
			It("Should not create the success file", func() {
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
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: Downloader{
						ModelDir: modelDir + "/test4",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mocks.MockS3Client{},
								Downloader: &mocks.MockS3FailDownloader{Err: err},
							},
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				go puller.processCommands(watcher.ModelEvents)
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
				}
				watcher.parseConfig(modelConfigs, false)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
			})
		})
	})

	Describe("Use GCS Downloader", func() {
		Context("Download Mocked Model", func() {
			It("should download test model and write contents", func() {
				defer GinkgoRecover()

				logger.Printf("Creating mock GCS Client")
				ctx := context.Background()
				client := mocks.NewMockClient()
				cl := storage.GCSProvider{
					Client: client,
				}

				logger.Printf("Populating mocked bucket with test model")
				bkt := client.Bucket("testBucket")
				if err := bkt.Create(ctx, "test", nil); err != nil {
					Fail("Error creating bucket.")
				}
				const modelContents = "Model Contents"
				w := bkt.Object("testModel1").NewWriter(ctx)
				if _, err := fmt.Fprint(w, modelContents); err != nil {
					Fail("Failed to write contents.")
				}
				modelName := "model1"
				modelStorageURI := "gs://testBucket/testModel1"
				err := cl.DownloadModel(modelDir, modelName, modelStorageURI)
				Expect(err).To(BeNil())

				testFile := filepath.Join(modelDir, "model1")
				dat, err := ioutil.ReadFile(testFile)
				Expect(err).To(BeNil())
				Expect(string(dat)).To(Equal(modelContents))
			})
		})

		Context("Model Download Failure", func() {
			It("should fail out if the model does not exist in the bucket", func() {
				defer GinkgoRecover()

				logger.Printf("Creating mock GCS Client")
				ctx := context.Background()
				client := mocks.NewMockClient()
				cl := storage.GCSProvider{
					Client: client,
				}

				logger.Printf("Populating mocked bucket with test model")
				bkt := client.Bucket("testBucket")
				if err := bkt.Create(ctx, "test", nil); err != nil {
					Fail("Error creating bucket.")
				}
				const modelContents = "Model Contents"
				w := bkt.Object("testModel1").NewWriter(ctx)
				if _, err := fmt.Fprint(w, modelContents); err != nil {
					Fail("Failed to write contents.")
				}
				modelName := "model1"
				modelStorageURI := "gs://testBucket/testModel2"
				expectedErr := fmt.Errorf("unable to download object/s because: %v", gstorage.ErrObjectNotExist)
				actualErr := cl.DownloadModel(modelDir, modelName, modelStorageURI)
				Expect(actualErr).To(Equal(expectedErr))
			})
		})

		Context("Download All Models", func() {
			It("should download all models if a model name is not specified for bucket", func() {
				logger.Printf("Creating mock GCS Client")
				ctx := context.Background()
				client := mocks.NewMockClient()
				cl := storage.GCSProvider{
					Client: client,
				}

				logger.Printf("Populating mocked bucket with test model")
				bkt := client.Bucket("testBucket")
				if err := bkt.Create(ctx, "test", nil); err != nil {
					Fail("Error creating bucket.")
				}
				const modelContents = "Model Contents"
				w := bkt.Object("testModel1").NewWriter(ctx)
				if _, err := fmt.Fprint(w, modelContents); err != nil {
					Fail("Failed to write contents.")
				}

				const secondaryContents = "Secondary Contents"
				w2 := bkt.Object("testModel2").NewWriter(ctx)
				if _, err := fmt.Fprint(w2, modelContents); err != nil {
					Fail("Failed to write contents.")
				}

				modelStorageURI := "gs://testBucket/"
				err := cl.DownloadModel(modelDir, "", modelStorageURI)
				Expect(err).To(BeNil())
			})
		})

		Context("Getting new model events", func() {
			It("should download and load the new models", func() {
				defer GinkgoRecover()
				logger.Printf("Sync model config using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "gs://testBucket/testModel1",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "gs://testBucket/testModel2",
							Framework:  "sklearn",
						},
					},
				}
				// Creating GCS mock client and populating buckets
				ctx := context.Background()
				client := mocks.NewMockClient()
				cl := storage.GCSProvider{
					Client: client,
				}
				bkt := client.Bucket("testBucket")
				if err := bkt.Create(ctx, "test", nil); err != nil {
					Fail("Error creating bucket.")
				}
				const modelContents = "Model Contents"
				w := bkt.Object("testModel1").NewWriter(ctx)
				if _, err := fmt.Fprint(w, modelContents); err != nil {
					Fail("Failed to write contents.")
				}

				const secondaryContents = "Secondary Contents"
				w2 := bkt.Object("testModel2").NewWriter(ctx)
				if _, err := fmt.Fprint(w2, modelContents); err != nil {
					Fail("Failed to write contents.")
				}

				watcher.parseConfig(modelConfigs, false)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: Downloader{
						ModelDir: modelDir + "/test1",
						Providers: map[storage.Protocol]storage.Provider{
							storage.GCS: &cl,
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				go puller.processCommands(watcher.ModelEvents)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
			})
		})

		Context("Puller Waits Before Initializing", func() {
			It("should download all models before allowing watcher to add new events", func() {
				defer GinkgoRecover()
				logger.Printf("Sync model config using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					waitGroup:   WaitGroupWrapper{sync.WaitGroup{}},
					Downloader: Downloader{
						ModelDir: modelDir + "/test2",
						Providers: map[storage.Protocol]storage.Provider{
							storage.S3: &storage.S3Provider{
								Client:     &mocks.MockS3Client{},
								Downloader: &mocks.MockS3Downloader{},
							},
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model1",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "s3://models/model2",
							Framework:  "sklearn",
						},
					},
				}
				puller.waitGroup.wg.Add(len(modelConfigs))
				watcher.parseConfig(modelConfigs, true)
				go puller.processCommands(watcher.ModelEvents)
				puller.waitGroup.wg.Wait()
				Expect(len(puller.channelMap)).To(Equal(0))
				Expect(puller.opStats["model1"][Add]).Should(Equal(1))
				Expect(puller.opStats["model2"][Add]).Should(Equal(1))
			})
		})
	})

	Describe("Use HTTPS Downloader", func() {
		Context("Download Uncompressed Mocked Model", func() {
			It("should download test model and write contents", func() {
				logger.Printf("Creating mock HTTPS Client")
				modelContents := "Test"
				body := ioutil.NopCloser(bytes.NewReader([]byte(modelContents)))
				modelName := "model1"
				modelStorageURI := "https://example.com/model.joblib"
				responses := map[string]*http.Response{
					modelStorageURI: {
						StatusCode:    200,
						Body:          body,
						ContentLength: -1,
						Uncompressed:  true,
					},
				}
				client := mocks.NewHTTPSMockClient(responses)
				cl := storage.HTTPSProvider{
					Client: client,
				}

				err := cl.DownloadModel(modelDir, modelName, modelStorageURI)
				Expect(err).To(BeNil())

				testFile := filepath.Join(modelDir, "model1", "model.joblib")
				dat, err := ioutil.ReadFile(testFile)
				Expect(err).To(BeNil())
				Expect(string(dat)).To(Equal(modelContents))
			})
		})

		Context("Model Download Failure", func() {
			It("should fail out if the uri does not exist", func() {
				logger.Printf("Creating mock HTTPS Client")
				modelName := "model1"
				modelStorageURI := "https://example.com/model.joblib"
				body := ioutil.NopCloser(bytes.NewReader([]byte("")))
				responses := map[string]*http.Response{
					modelStorageURI: {
						StatusCode: 404,
						Body:       body,
					},
				}
				client := mocks.NewHTTPSMockClient(responses)
				cl := storage.HTTPSProvider{
					Client: client,
				}

				expectedErr := fmt.Errorf("URI: %s returned a %d response code", modelStorageURI, 404)
				actualErr := cl.DownloadModel(modelDir, modelName, modelStorageURI)
				Expect(actualErr).To(Equal(expectedErr))
			})
		})

		Context("Download All Models", func() {
			It("should download and load zip and tar files", func() {
				logger.Printf("Setting up tar model")
				tarModel := "model1"
				tarContent := "1f8b0800bac550600003cbcd4f49cdd12b28c960a01d3030303033315100d1e666a660dac008c287010" +
					"54313a090a189919981998281a1b1b1a1118382010ddd0407a5c525894540a754656466e464e2560754" +
					"969686c71ca83fe0f4281805a360140c7200009f7e1bb400060000"
				tarByte, err := hex.DecodeString(tarContent)
				Expect(err).To(BeNil())
				tarBody := ioutil.NopCloser(bytes.NewReader(tarByte))
				tarStorageURI := "https://example.com/test.tar"
				var tarHead http.Header = map[string][]string{}
				tarHead.Add("Content-type", "application/x-tar; charset='UTF-8'")

				logger.Printf("Setting up zip model")
				zipModel := "model2"
				zipContents := "504b030414000800080035b67052000000000000000000000000090020006d6f64656c2e70746855540" +
					"d000786c5506086c5506086c5506075780b000104f501000004140000000300504b07080000000002000" +
					"00000000000504b0102140314000800080035b6705200000000020000000000000009002000000000000" +
					"0000000a481000000006d6f64656c2e70746855540d000786c5506086c5506086c5506075780b000104f" +
					"50100000414000000504b0506000000000100010057000000590000000000"
				zipByte, err := hex.DecodeString(zipContents)
				Expect(err).To(BeNil())
				zipBody := ioutil.NopCloser(bytes.NewReader(zipByte))
				zipStorageURI := "https://example.com/test.zip"
				var zipHead http.Header = map[string][]string{}
				zipHead.Add("Content-type", "application/zip; charset='UTF-8'")

				responses := map[string]*http.Response{
					tarStorageURI: {
						StatusCode:   200,
						Header:       tarHead,
						Body:         tarBody,
						Uncompressed: false,
					},
					zipStorageURI: {
						StatusCode:   200,
						Header:       zipHead,
						Body:         zipBody,
						Uncompressed: false,
					},
				}

				logger.Printf("Creating mock HTTPS Client")
				client := mocks.NewHTTPSMockClient(responses)
				cl := storage.HTTPSProvider{
					Client: client,
				}

				err = cl.DownloadModel(modelDir, zipModel, zipStorageURI)
				Expect(err).To(BeNil())
				err = cl.DownloadModel(modelDir, tarModel, tarStorageURI)
				Expect(err).To(BeNil())

			})
		})

		Context("Getting new model events", func() {
			It("should download and load the new models", func() {
				logger.Printf("Sync model config using temp dir %v\n", modelDir)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				modelConfigs := modelconfig.ModelConfigs{
					{
						Name: "model1",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "https://example.com/test.tar",
							Framework:  "sklearn",
						},
					},
					{
						Name: "model2",
						Spec: v1alpha1.ModelSpec{
							StorageURI: "https://example.com/test.zip",
							Framework:  "sklearn",
						},
					},
				}

				// Create HTTPS client
				client := mocks.NewHTTPSMockClient(nil)
				cl := storage.HTTPSProvider{
					Client: client,
				}

				watcher.parseConfig(modelConfigs)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: Downloader{
						ModelDir: modelDir + "/test1",
						Providers: map[storage.Protocol]storage.Provider{
							storage.HTTPS: &cl,
						},
						Logger: sugar,
					},
					logger: sugar,
				}
				go puller.processCommands(watcher.ModelEvents)
				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
			})
		})
	})
})
