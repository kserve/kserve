package v1beta1

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/constants"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MLServer predictor", func() {
	var mlserverSpec *MLServerSpec
	var config *InferenceServicesConfig

	BeforeEach(func() {
		mlserverSpec = &MLServerSpec{
			PredictorExtensionSpec: PredictorExtensionSpec{
				StorageURI: proto.String("gs://my-bucket/my-models"),
			},
		}

		config = &InferenceServicesConfig{
			Predictors: PredictorsConfig{
				MLServer: PredictorConfig{
					ContainerImage:      "seldonio/mlserver",
					DefaultImageVersion: "0.1.1",
				},
			},
		}
	})

	Context("When validating", func() {
		It("Shouldn't return an error on valid resources", func() {
			err := mlserverSpec.Validate()

			Expect(err).To(Succeed())
		})

		It("Should flag invalid storage URIs", func() {
			mlserverSpec.StorageURI = proto.String("foo://bar")
			err := mlserverSpec.Validate()

			Expect(err).To(HaveOccurred())
		})
	})

	Context("When defaulting", func() {
		It("Should set runtime version, resources and name", func() {
			mlserverSpec.Default(config)

			Expect(mlserverSpec.Name).To(Equal(constants.InferenceServiceContainerName))
			Expect(*mlserverSpec.RuntimeVersion).To(Equal(config.Predictors.MLServer.DefaultImageVersion))

			for key, value := range defaultResource {
				Expect(mlserverSpec.Resources.Requests).To(HaveKeyWithValue(key, value))
				Expect(mlserverSpec.Resources.Limits).To(HaveKeyWithValue(key, value))
			}
		})

		It("Shouldn't set runtime version if specified", func() {
			customVersion := "1.2.3"
			mlserverSpec.RuntimeVersion = &customVersion
			mlserverSpec.Default(config)

			Expect(*mlserverSpec.RuntimeVersion).To(Equal(customVersion))
		})
	})

	Context("When getting the container", func() {
		var metadata metav1.ObjectMeta
		var extensions *ComponentExtensionSpec

		BeforeEach(func() {
			metadata = metav1.ObjectMeta{}
			extensions = &ComponentExtensionSpec{}
			mlserverSpec.Default(config)
		})

		It("Should set env vars and image name", func() {
			c := mlserverSpec.GetContainer(metadata, extensions, config)

			Expect(c.Env).To(ContainElements(
				v1.EnvVar{
					Name:  MLServerHTTPPortEnv,
					Value: fmt.Sprint(MLServerISRestPort),
				},
				v1.EnvVar{
					Name:  MLServerGRPCPortEnv,
					Value: fmt.Sprint(MLServerISGRPCPort),
				},
				v1.EnvVar{
					Name:  MLServerModelsDirEnv,
					Value: constants.DefaultModelLocalMountPath,
				},
			))

			Expect(c.Image).To(Equal(fmt.Sprintf(
				"%s:%s",
				config.Predictors.MLServer.ContainerImage,
				*mlserverSpec.RuntimeVersion,
			)))
		})
	})
})
