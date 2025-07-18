/*
Copyright 2021 The KServe Authors.

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
	"context"
	"encoding/json"
	"fmt"
	logger "log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"time"

	gstorage "cloud.google.com/go/storage"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kserve/kserve/pkg/agent/mocks"
	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/modelconfig"
)

var _ = Describe("Watcher", func() {
	var modelDir string
	var sugar *zap.SugaredLogger
	BeforeEach(func() {
		dir, err := os.MkdirTemp("", "example")
		if err != nil {
			logger.Fatal(err)
		}
		modelDir = dir
		zapLogger, _ := zap.NewProduction()
		sugar = zapLogger.Sugar()
		logger.Printf("Creating temp dir %v\n", modelDir)
		SetDefaultEventuallyTimeout(2 * time.Minute)
		log.SetLogger(zapr.NewLogger(zapLogger))
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
				_, err := os.Stat("/tmp/configs")
				if os.IsNotExist(err) {
					if err := os.MkdirAll("/tmp/configs", os.ModePerm); err != nil {
						logger.Fatal(err, " Failed to create configs directory")
					}
				}

				file, _ := json.MarshalIndent(modelConfigs, "", " ")
				tmpFile, err := os.Create("/tmp/configs/" + constants.ModelConfigFileName) // #nosec G303
				if err != nil {
					logger.Fatal(err, "failed to create model config file")
				}

				if _, err := tmpFile.Write(file); err != nil {
					logger.Fatal(err, "tmpFile.Write failed")
				}
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)

				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					waitGroup:   WaitGroupWrapper{sync.WaitGroup{}},
					Downloader: &Downloader{
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
				puller.waitGroup.wg.Add(len(watcher.ModelEvents))
				go puller.processCommands(watcher.ModelEvents)

				Eventually(func() int { return len(puller.channelMap) }).Should(Equal(0))
				Eventually(func() int { return puller.opStats["model1"][Add] }).Should(Equal(1))
				Eventually(func() int { return puller.opStats["model2"][Add] }).Should(Equal(1))
				modelSpecMap, _ := SyncModelDir(modelDir+"/test1", watcher.logger)
				Expect(watcher.ModelTracker).Should(Equal(modelSpecMap))

				DeferCleanup(func() {
					tmpFile.Close()
					os.RemoveAll("/tmp/configs")
				})
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
					Downloader: &Downloader{
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
					Downloader: &Downloader{
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
					Downloader: &Downloader{
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
					OrigErr: errors.New("failed to download"),
					Bucket:  aws.String("modelRepo"),
					Key:     aws.String("model1/model.pt"),
				})
				err := s3manager.NewBatchError("BatchedDownloadIncomplete", "some objects have failed to download.", errs)
				watcher := NewWatcher("/tmp/configs", modelDir, sugar)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: &Downloader{
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
				modelStorageURI := "gs://testBucket/"
				err := cl.DownloadModel(modelDir, modelName, modelStorageURI)
				Expect(err).ToNot(HaveOccurred())

				testFile := filepath.Join(modelDir, modelName, "testModel1")
				dat, err := os.ReadFile(testFile)
				Expect(err).ToNot(HaveOccurred())
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
				expectedErr := fmt.Errorf("unable to download object/s because: %w", gstorage.ErrObjectNotExist)
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

				w2 := bkt.Object("testModel2").NewWriter(ctx)
				if _, err := fmt.Fprint(w2, modelContents); err != nil {
					Fail("Failed to write contents.")
				}

				modelStorageURI := "gs://testBucket/"
				err := cl.DownloadModel(modelDir, "", modelStorageURI)
				Expect(err).ToNot(HaveOccurred())
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

				w2 := bkt.Object("testModel2").NewWriter(ctx)
				if _, err := fmt.Fprint(w2, modelContents); err != nil {
					Fail("Failed to write contents.")
				}

				watcher.parseConfig(modelConfigs, false)
				puller := Puller{
					channelMap:  make(map[string]*ModelChannel),
					completions: make(chan *ModelOp, 4),
					opStats:     make(map[string]map[OpType]int),
					Downloader: &Downloader{
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
					Downloader: &Downloader{
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
				Expect(puller.channelMap).To(BeEmpty())
				Expect(puller.opStats["model1"][Add]).Should(Equal(1))
				Expect(puller.opStats["model2"][Add]).Should(Equal(1))
			})
		})
	})

	Describe("Use HTTP(S) Downloader", func() {
		Context("Download Uncompressed Model", func() {
			It("should download test model and write contents", func() {
				modelContents := "Temporary content"
				scenarios := map[string]struct {
					server *httptest.Server
				}{
					"HTTP": {
						httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, modelContents)
						})),
					},
					"HTTPS": {
						httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, modelContents)
						})),
					},
				}

				for protocol, scenario := range scenarios {
					logger.Printf("Setting up %s Server", protocol)
					ts := scenario.server
					defer ts.Close()

					modelName := "model1"
					modelFile := "model.joblib"
					modelStorageURI := ts.URL + "/" + modelFile
					cl := storage.HTTPSProvider{
						Client: ts.Client(),
					}

					err := cl.DownloadModel(modelDir, modelName, modelStorageURI)
					Expect(err).ToNot(HaveOccurred())

					testFile := filepath.Join(modelDir, modelName, modelFile)
					dat, err := os.ReadFile(testFile)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(dat)).To(Equal(modelContents + "\n"))
				}
			})
		})

		Context("Model Download Failure", func() {
			It("should fail out if the uri does not exist", func() {
				logger.Printf("Creating Client")
				modelName := "model1"
				invalidModelStorageURI := "https://example.com/model.joblib"
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				defer ts.Close()
				cl := storage.HTTPSProvider{
					Client: ts.Client(),
				}

				actualErr := cl.DownloadModel(modelDir, modelName, invalidModelStorageURI)
				Expect(actualErr).To(HaveOccurred())
			})
		})

		Context("Download All Models", func() {
			It("should download and load zip and tar files", func() {
				tarContent := "1f8b0800bac550600003cbcd4f49cdd12b28c960a01d3030303033315100d1e666a660dac008c287010" +
					"54313a090a189919981998281a1b1b1a1118382010ddd0407a5c525894540a754656466e464e2560754" +
					"969686c71ca83fe0f4281805a360140c7200009f7e1bb400060000"

				zipContents := "504b030414000800080035b67052000000000000000000000000090020006d6f64656c2e70746855540" +
					"d000786c5506086c5506086c5506075780b000104f501000004140000000300504b07080000000002000" +
					"00000000000504b0102140314000800080035b6705200000000020000000000000009002000000000000" +
					"0000000a481000000006d6f64656c2e70746855540d000786c5506086c5506086c5506075780b000104f" +
					"50100000414000000504b0506000000000100010057000000590000000000"

				scenarios := map[string]struct {
					tarServer *httptest.Server
					zipServer *httptest.Server
				}{
					"HTTP": {
						httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, tarContent)
						})),
						httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, zipContents)
						})),
					},
					"HTTPS": {
						httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, tarContent)
						})),
						httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, zipContents)
						})),
					},
				}
				for protocol, scenario := range scenarios {
					logger.Printf("Using %s Server", protocol)
					logger.Printf("Setting up tar model")
					tarServer := scenario.tarServer
					defer tarServer.Close()

					tarModel := "model1"
					tarStorageURI := tarServer.URL + "/test.tar"
					tarcl := storage.HTTPSProvider{
						Client: tarServer.Client(),
					}

					logger.Printf("Setting up zip model")
					zipServer := scenario.zipServer
					defer zipServer.Close()

					zipModel := "model2"
					zipStorageURI := zipServer.URL + "/test.zip"
					zipcl := storage.HTTPSProvider{
						Client: tarServer.Client(),
					}

					err := zipcl.DownloadModel(modelDir, zipModel, zipStorageURI)
					Expect(err).ToNot(HaveOccurred())
					err = tarcl.DownloadModel(modelDir, tarModel, tarStorageURI)
					Expect(err).ToNot(HaveOccurred())
				}
			})
		})

		Context("Getting new model events", func() {
			It("should download and load the new models", func() {
				modelContents := "Temporary content"
				scenarios := map[string]struct {
					server *httptest.Server
				}{
					"HTTP": {
						httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, modelContents)
						})),
					},
					"HTTPS": {
						httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							fmt.Fprintln(w, modelContents)
						})),
					},
				}
				for protocol, scenario := range scenarios {
					ts := scenario.server
					defer ts.Close()
					cl := storage.HTTPSProvider{
						Client: ts.Client(),
					}

					logger.Printf("Setting up %s Server", protocol)
					logger.Printf("Sync model config using temp dir %v\n", modelDir)
					watcher := NewWatcher("/tmp/configs", modelDir, sugar)
					modelConfigs := modelconfig.ModelConfigs{
						{
							Name: "model1",
							Spec: v1alpha1.ModelSpec{
								StorageURI: ts.URL + "/test.tar",
								Framework:  "sklearn",
							},
						},
						{
							Name: "model2",
							Spec: v1alpha1.ModelSpec{
								StorageURI: ts.URL + "/test.zip",
								Framework:  "sklearn",
							},
						},
					}

					watcher.parseConfig(modelConfigs, false)
					puller := Puller{
						channelMap:  make(map[string]*ModelChannel),
						completions: make(chan *ModelOp, 4),
						opStats:     make(map[string]map[OpType]int),
						Downloader: &Downloader{
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
				}
			})
		})
	})
})
