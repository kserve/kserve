package v1beta1

import (
	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MLServer predictor", func() {
	var mlserverSpec *MLServerSpec

	BeforeEach(func() {
		mlserverSpec = &MLServerSpec{
			PredictorExtensionSpec: PredictorExtensionSpec{
				StorageURI: proto.String("gs://my-bucket/my-models"),
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
})
