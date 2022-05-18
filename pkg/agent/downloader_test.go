package agent

import (
	"io/ioutil"
	logger "log"
	"os"

	"github.com/kserve/kserve/pkg/agent/mocks"
	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/modelconfig"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
)

var _ = Describe("Downloader", func() {
	var modelDir string
	var downloader *Downloader
	BeforeEach(func() {
		dir, err := ioutil.TempDir("", "example")
		if err != nil {
			logger.Fatal(err)
		}
		modelDir = dir
		logger.Printf("Creating temp dir %v\n", modelDir)
		zapLogger, _ := zap.NewProduction()
		sugar := zapLogger.Sugar()
		downloader = &Downloader{
			ModelDir: modelDir + "/test",
			Providers: map[storage.Protocol]storage.Provider{
				storage.S3: &storage.S3Provider{
					Client:     &mocks.MockS3Client{},
					Downloader: &mocks.MockS3Downloader{},
				},
			},
			Logger: sugar,
		}
	})
	AfterEach(func() {
		os.RemoveAll(modelDir)
		logger.Printf("Deleted temp dir %v\n", modelDir)
	})

	Context("When protocol is invalid", func() {
		It("Should fail out and return error", func() {
			modelConfig := modelconfig.ModelConfig{
				Name: "model1",
				Spec: v1alpha1.ModelSpec{
					StorageURI: "sss://models/model1",
					Framework:  "sklearn",
				},
			}
			err := downloader.DownloadModel(modelConfig.Name, &modelConfig.Spec)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("When storage uri is empty", func() {
		It("Should fail out and return error", func() {
			modelConfig := modelconfig.ModelConfig{
				Name: "model1",
				Spec: v1alpha1.ModelSpec{
					StorageURI: "",
					Framework:  "sklearn",
				},
			}
			err := downloader.DownloadModel(modelConfig.Name, &modelConfig.Spec)
			Expect(err).ShouldNot(BeNil())
		})
	})

	Context("When storage uri is invalid", func() {
		It("Should fail out and return error", func() {
			modelConfig := modelconfig.ModelConfig{
				Name: "model1",
				Spec: v1alpha1.ModelSpec{
					StorageURI: "s3:://models/model1",
					Framework:  "sklearn",
				},
			}
			err := downloader.DownloadModel(modelConfig.Name, &modelConfig.Spec)
			Expect(err).ShouldNot(BeNil())
		})
	})
})
